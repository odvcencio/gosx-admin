package ics_test

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"m31labs.dev/gosx-admin/schedule"
	"m31labs.dev/gosx-admin/schedule/ics"
)

var stamp = time.Date(2027, 2, 1, 12, 30, 45, 0, time.UTC)

func event(id string) schedule.Event {
	return schedule.Event{ID: id, Title: "A title", Summary: "A summary", Location: "Los Angeles", Category: "program", Visibility: "public", Status: schedule.EventPublished, Zone: "America/Los_Angeles",
		Start: time.Date(2027, 3, 1, 10, 0, 0, 0, time.UTC), End: time.Date(2027, 3, 1, 11, 0, 0, 0, time.UTC), Sequence: 2, PublishedAt: stamp}
}

func TestICSVectors(t *testing.T) {
	allDay := event("all-day")
	allDay.AllDay = true
	allDay.StartDate = schedule.Date{Year: 2027, Month: time.February, Day: 15}
	allDay.EndDate = allDay.StartDate.AddDays(1)
	twoDay := event("two-day")
	twoDay.AllDay = true
	twoDay.StartDate = schedule.Date{Year: 2027, Month: time.June, Day: 18}
	twoDay.EndDate = schedule.Date{Year: 2027, Month: time.June, Day: 20}
	body, _, err := ics.Feed([]schedule.Event{twoDay, allDay}, ics.Options{ProdID: "-//Example//Admin Calendar//EN", UIDDomain: "example.test", Visibility: []string{"public"}, At: stamp})
	if err != nil {
		t.Fatal(err)
	}
	text := string(body)
	for _, want := range []string{"DTSTART;VALUE=DATE:20270215\r\n", "DTEND;VALUE=DATE:20270216\r\n", "DTSTART;VALUE=DATE:20270618\r\n", "DTEND;VALUE=DATE:20270620\r\n", "UID:all-day@example.test\r\n", "DTSTAMP:20270201T123045Z\r\n"} {
		if !strings.Contains(text, want) {
			t.Errorf("feed missing %q", want)
		}
	}
	timed := event("timed-zone")
	timed.Start = time.Date(2027, 3, 1, 18, 0, 0, 0, time.FixedZone("UTC+08", 8*60*60))
	timed.End = timed.Start.Add(time.Hour)
	timedBody, _, err := ics.Feed([]schedule.Event{timed}, ics.Options{UIDDomain: "example.test", Visibility: []string{"public"}, At: stamp})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(timedBody), "DTSTART:20270301T100000Z\r\n") || !strings.Contains(string(timedBody), "DTEND:20270301T110000Z\r\n") {
		t.Fatalf("timed event did not use UTC instants:\n%s", timedBody)
	}
	updated := event("stable")
	updated.Sequence = 3
	updated.Start = updated.Start.Add(time.Hour)
	first, _, err := ics.Feed([]schedule.Event{updated}, ics.Options{UIDDomain: "example.test", Visibility: []string{"public"}, At: stamp})
	if err != nil {
		t.Fatal(err)
	}
	cancelled := updated
	cancelled.Sequence = 4
	cancelled.Status = schedule.EventCancelled
	cancelled.PublishedAt = stamp.Add(time.Hour)
	second, _, err := ics.Feed([]schedule.Event{cancelled}, ics.Options{UIDDomain: "example.test", Visibility: []string{"public"}, At: stamp.Add(2 * time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(first), "UID:stable@example.test") || !strings.Contains(string(second), "UID:stable@example.test") || !strings.Contains(string(second), "SEQUENCE:4\r\n") || !strings.Contains(string(second), "STATUS:CANCELLED\r\n") {
		t.Fatalf("sequence vector invalid:\n%s\n%s", first, second)
	}
}

func TestICSEscapingAndFolding(t *testing.T) {
	e := event("fold")
	e.Title = strings.Repeat("é", 200)
	e.Summary = "backslash\\ semicolon; comma, CR\r LF\n CRLF\r\n"
	body, _, err := ics.Feed([]schedule.Event{e}, ics.Options{Visibility: []string{"public"}, UIDDomain: "example.test", At: stamp})
	if err != nil {
		t.Fatal(err)
	}
	if !utf8.Valid(body) {
		t.Fatal("feed is not valid UTF-8")
	}
	if !strings.HasSuffix(string(body), "\r\n") {
		t.Fatal("feed does not end with CRLF")
	}
	physical := strings.Split(strings.TrimSuffix(string(body), "\r\n"), "\r\n")
	for i, line := range physical {
		if len([]byte(line)) > 75 {
			t.Errorf("physical line %d is %d octets", i, len([]byte(line)))
		}
		if i > 0 && strings.HasPrefix(line, " ") && len(line) == 1 {
			t.Errorf("empty folded continuation at line %d", i)
		}
	}
	unfolded := strings.ReplaceAll(string(body), "\r\n ", "")
	for _, want := range []string{`SUMMARY:` + strings.Repeat("é", 200), `backslash\\ semicolon\; comma\, CR\n LF\n CRLF\n`} {
		if !strings.Contains(unfolded, want) {
			t.Errorf("unfolded feed missing %q", want)
		}
	}
	for _, line := range strings.Split(strings.TrimSuffix(unfolded, "\r\n"), "\r\n") {
		if !utf8.ValidString(line) {
			t.Fatal("fold split a UTF-8 rune")
		}
	}
}

func TestICSVisibilityRequired(t *testing.T) {
	e := event("event")
	if _, _, err := ics.Feed([]schedule.Event{e}, ics.Options{}); !errors.Is(err, ics.ErrNoVisibility) {
		t.Fatalf("Feed error = %v", err)
	}
	body, _, err := ics.Feed([]schedule.Event{e}, ics.Options{Visibility: []string{"private"}, At: stamp})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), "BEGIN:VEVENT") {
		t.Fatalf("unlisted event included:\n%s", body)
	}
}

func TestICSCancelledRetention(t *testing.T) {
	e := event("cancel")
	e.Status = schedule.EventCancelled
	e.PublishedAt = stamp
	for _, tc := range []struct {
		name string
		at   time.Time
		want bool
	}{{"90 days inclusive", stamp.Add(90 * 24 * time.Hour), true}, {"90 days and one second", stamp.Add(90*24*time.Hour + time.Second), false}} {
		t.Run(tc.name, func(t *testing.T) {
			body, _, err := ics.Feed([]schedule.Event{e}, ics.Options{Visibility: []string{"public"}, At: tc.at})
			if err != nil {
				t.Fatal(err)
			}
			got := strings.Contains(string(body), "BEGIN:VEVENT")
			if got != tc.want {
				t.Fatalf("included = %t, want %t", got, tc.want)
			}
		})
	}
}

func TestICSNoMethodStableBody(t *testing.T) {
	e := event("stable")
	opts := ics.Options{ProdID: "-//Example//Calendar//EN", UIDDomain: "example.test", Name: "Example", Visibility: []string{"public"}, At: stamp}
	first, etag1, err := ics.Feed([]schedule.Event{e}, opts)
	if err != nil {
		t.Fatal(err)
	}
	second, etag2, err := ics.Feed([]schedule.Event{e}, opts)
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != string(second) || etag1 != etag2 {
		t.Fatal("same feed input did not produce stable body and ETag")
	}
	if strings.Contains(string(first), "METHOD:") {
		t.Fatalf("feed has METHOD:\n%s", first)
	}
	if !strings.HasPrefix(etag1, "\"") || !strings.HasSuffix(etag1, "\"") || len(etag1) != 34 {
		t.Fatalf("ETag = %q", etag1)
	}
}

func TestICSHandler(t *testing.T) {
	e := event("event")
	handler := ics.Handler(func(r *http.Request) ([]schedule.Event, error) {
		if r.URL.Query().Get("fail") == "1" {
			return nil, fmt.Errorf("store error")
		}
		return []schedule.Event{e}, nil
	}, ics.Options{Visibility: []string{"public"}, At: stamp})
	request := httptest.NewRequest(http.MethodGet, "/feed", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("GET status = %d", response.Code)
	}
	if response.Header().Get("Content-Type") != "text/calendar; charset=utf-8" || response.Header().Get("ETag") == "" || response.Header().Get("Cache-Control") != "no-cache" {
		t.Fatalf("headers = %#v", response.Header())
	}
	request = httptest.NewRequest(http.MethodGet, "/feed", nil)
	request.Header.Set("If-None-Match", response.Header().Get("ETag"))
	cached := httptest.NewRecorder()
	handler.ServeHTTP(cached, request)
	if cached.Code != http.StatusNotModified {
		t.Fatalf("conditional GET status = %d", cached.Code)
	}
	head := httptest.NewRequest(http.MethodHead, "/feed", nil)
	headResponse := httptest.NewRecorder()
	handler.ServeHTTP(headResponse, head)
	if headResponse.Code != http.StatusOK || headResponse.Body.Len() != 0 {
		t.Fatalf("HEAD = %d, body %d", headResponse.Code, headResponse.Body.Len())
	}
	post := httptest.NewRequest(http.MethodPost, "/feed", nil)
	postResponse := httptest.NewRecorder()
	handler.ServeHTTP(postResponse, post)
	if postResponse.Code != http.StatusMethodNotAllowed || postResponse.Header().Get("Allow") != "GET, HEAD" {
		t.Fatalf("POST = %d, Allow %q", postResponse.Code, postResponse.Header().Get("Allow"))
	}
	fail := httptest.NewRequest(http.MethodGet, "/feed?fail=1", nil)
	failResponse := httptest.NewRecorder()
	handler.ServeHTTP(failResponse, fail)
	if failResponse.Code != http.StatusServiceUnavailable {
		t.Fatalf("load error status = %d", failResponse.Code)
	}
}
