// Package ics writes RFC 5545 calendar feeds for schedule events.
package ics

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"m31labs.dev/gosx-admin/schedule"
)

// ErrNoVisibility is returned when Options.Visibility is empty, so a feed
// never includes events by accident.
var ErrNoVisibility = errors.New("ics: Options.Visibility is empty")

// Options controls a feed.
type Options struct {
	// ProdID is the PRODID value, for example "-//Example//Admin Calendar//EN".
	ProdID string
	// UIDDomain makes UID = event ID + "@" + UIDDomain. The UID never
	// changes for an event.
	UIDDomain string
	// Name sets X-WR-CALNAME when not empty.
	Name string
	// Visibility lists the Event.Visibility labels the feed includes.
	Visibility []string
	// At is the feed build time. Default time.Now().
	At time.Time
	// KeepCancelled keeps a cancelled event while At - PublishedAt is at
	// most this. Default 90 days.
	KeepCancelled time.Duration
}

// Write writes the feed for events to w. The calendar has VERSION:2.0,
// PRODID, CALSCALE:GREGORIAN, X-WR-CALNAME when Name is set, and no METHOD
// property, so DTSTAMP means the last revision time (RFC 5545 section
// 3.8.7.2) and the body stays stable between requests.
//
// It includes published events, and cancelled events inside KeepCancelled,
// whose Visibility is listed; it skips drafts. Events are ordered by ID. Each
// VEVENT has UID, SEQUENCE, DTSTAMP (PublishedAt in UTC), DTSTART and DTEND,
// SUMMARY, DESCRIPTION when Summary is set, LOCATION when set, CATEGORIES
// when Category is set, and STATUS:CONFIRMED or STATUS:CANCELLED. An all-day
// event writes DTSTART;VALUE=DATE:{StartDate} and DTEND;VALUE=DATE:{EndDate};
// a timed event writes UTC times with a Z suffix.
//
// Text escapes backslash, semicolon, comma, and line breaks. Lines end with
// CRLF and fold at 75 octets, never inside a UTF-8 sequence.
func Write(w io.Writer, events []schedule.Event, opts Options) error {
	if len(opts.Visibility) == 0 {
		return ErrNoVisibility
	}
	if opts.At.IsZero() {
		opts.At = time.Now()
	}
	if opts.KeepCancelled == 0 {
		opts.KeepCancelled = 90 * 24 * time.Hour
	}
	prodID := opts.ProdID
	if prodID == "" {
		prodID = "-//gosx-admin//schedule//EN"
	}
	domain := opts.UIDDomain
	if domain == "" {
		domain = "gosx-admin"
	}
	visible := make(map[string]bool, len(opts.Visibility))
	for _, v := range opts.Visibility {
		if v != "" {
			visible[v] = true
		}
	}
	selected := make([]schedule.Event, 0, len(events))
	for _, event := range events {
		if !visible[event.Visibility] {
			continue
		}
		if event.Status == schedule.EventPublished {
			selected = append(selected, event)
			continue
		}
		if event.Status == schedule.EventCancelled && !opts.At.After(event.PublishedAt.Add(opts.KeepCancelled)) {
			selected = append(selected, event)
		}
	}
	sort.Slice(selected, func(i, j int) bool { return selected[i].ID < selected[j].ID })
	var b bytes.Buffer
	writeLine(&b, "BEGIN:VCALENDAR")
	writeProperty(&b, "VERSION", "2.0")
	writeProperty(&b, "PRODID", escapeText(prodID))
	writeProperty(&b, "CALSCALE", "GREGORIAN")
	if opts.Name != "" {
		writeProperty(&b, "X-WR-CALNAME", escapeText(opts.Name))
	}
	for _, event := range selected {
		writeEvent(&b, event, domain)
	}
	writeLine(&b, "END:VCALENDAR")
	_, err := w.Write(b.Bytes())
	return err
}

// Feed returns the feed bytes and a strong ETag: the quoted first 32 hex
// characters of the SHA-256 of the bytes.
func Feed(events []schedule.Event, opts Options) (body []byte, etag string, err error) {
	var b bytes.Buffer
	if err := Write(&b, events, opts); err != nil {
		return nil, "", err
	}
	body = b.Bytes()
	sum := sha256.Sum256(body)
	return body, `"` + fmt.Sprintf("%x", sum[:16]) + `"`, nil
}

// Handler serves a feed. load returns the events for the request; the
// handler still applies Options.Visibility. It answers GET and HEAD with
// "Content-Type: text/calendar; charset=utf-8", the ETag,
// "Cache-Control: no-cache", and 304 for a matching If-None-Match; other
// methods get 405; a load error gets 503.
func Handler(load func(r *http.Request) ([]schedule.Event, error), opts Options) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if load == nil {
			http.Error(w, "feed unavailable", http.StatusServiceUnavailable)
			return
		}
		events, err := load(r)
		if err != nil {
			http.Error(w, "feed unavailable", http.StatusServiceUnavailable)
			return
		}
		body, etag, err := Feed(events, opts)
		if err != nil {
			http.Error(w, "feed unavailable", http.StatusServiceUnavailable)
			return
		}
		h := w.Header()
		h.Set("Content-Type", "text/calendar; charset=utf-8")
		h.Set("ETag", etag)
		h.Set("Cache-Control", "no-cache")
		if etagMatches(r.Header.Get("If-None-Match"), etag) {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		if r.Method == http.MethodHead {
			w.WriteHeader(http.StatusOK)
			return
		}
		_, _ = w.Write(body)
	})
}

func writeEvent(b *bytes.Buffer, event schedule.Event, domain string) {
	writeLine(b, "BEGIN:VEVENT")
	writeProperty(b, "UID", escapeText(event.ID+"@"+domain))
	writeProperty(b, "SEQUENCE", fmt.Sprint(event.Sequence))
	writeProperty(b, "DTSTAMP", event.PublishedAt.UTC().Format("20060102T150405Z"))
	if event.AllDay {
		writeLine(b, "DTSTART;VALUE=DATE:"+strings.ReplaceAll(event.StartDate.String(), "-", ""))
		writeLine(b, "DTEND;VALUE=DATE:"+strings.ReplaceAll(event.EndDate.String(), "-", ""))
	} else {
		writeProperty(b, "DTSTART", event.Start.UTC().Format("20060102T150405Z"))
		writeProperty(b, "DTEND", event.End.UTC().Format("20060102T150405Z"))
	}
	writeProperty(b, "SUMMARY", escapeText(event.Title))
	if event.Summary != "" {
		writeProperty(b, "DESCRIPTION", escapeText(event.Summary))
	}
	if event.Location != "" {
		writeProperty(b, "LOCATION", escapeText(event.Location))
	}
	if event.Category != "" {
		writeProperty(b, "CATEGORIES", escapeText(event.Category))
	}
	status := "CONFIRMED"
	if event.Status == schedule.EventCancelled {
		status = "CANCELLED"
	}
	writeProperty(b, "STATUS", status)
	writeLine(b, "END:VEVENT")
}

func writeProperty(b *bytes.Buffer, name, value string) { writeLine(b, name+":"+value) }

func writeLine(b *bytes.Buffer, line string) {
	b.WriteString(foldLine(line))
	b.WriteString("\r\n")
}

func foldLine(line string) string {
	var b strings.Builder
	bytesOnLine := 0
	for _, r := range line {
		piece := string(r)
		if bytesOnLine+len(piece) > 75 {
			b.WriteString("\r\n ")
			bytesOnLine = 1
		}
		b.WriteString(piece)
		bytesOnLine += len(piece)
	}
	return b.String()
}

func escapeText(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	s = strings.ReplaceAll(s, "\n", `\n`)
	s = strings.ReplaceAll(s, ";", `\;`)
	s = strings.ReplaceAll(s, ",", `\,`)
	return s
}

func etagMatches(header, etag string) bool {
	for _, candidate := range strings.Split(header, ",") {
		candidate = strings.TrimSpace(candidate)
		if candidate == "*" || candidate == etag {
			return true
		}
	}
	return false
}
