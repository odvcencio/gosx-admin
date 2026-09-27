package schedule

import (
	"strings"
	"time"
)

// Revision is a record's optimistic-concurrency counter. A store sets it to 1
// on create and adds 1 on every write.
type Revision int64

// EventStatus is an event's publication state.
type EventStatus string

const (
	// EventDraft is not in feeds or public views.
	EventDraft EventStatus = "draft"
	// EventPublished is in feeds and open for booking inside its window.
	EventPublished EventStatus = "published"
	// EventCancelled stays in feeds with STATUS:CANCELLED; booking is closed.
	EventCancelled EventStatus = "cancelled"
)

// Event is one scheduled occurrence.
type Event struct {
	ID string
	// ResourceID optionally names the Resource the event uses.
	ResourceID string
	Title      string
	// Summary is plain text.
	Summary  string
	Location string
	// Category and Visibility are consumer-defined labels.
	Category   string
	Visibility string
	Status     EventStatus
	// Zone is the IANA zone the event is planned in, for example
	// "America/Los_Angeles". Views show the event's times in this zone.
	Zone   string
	AllDay bool
	// StartDate is the first day of an all-day event. EndDate is the day
	// after its last day (exclusive). Both are zero for timed events.
	StartDate Date
	EndDate   Date
	// Start and End bound the event: [Start, End). For an all-day event,
	// NormalizeEvent sets them to StartDate.Start(zone) and
	// EndDate.Start(zone).
	Start time.Time
	End   time.Time
	// Capacity is the seat limit; 0 means no limit.
	Capacity int
	// Waitlist lets a full event take waitlisted registrations.
	Waitlist bool
	// BookingOpens and BookingCloses bound the booking window. Zero means no
	// bound on that side. Booking also closes at Start.
	BookingOpens  time.Time
	BookingCloses time.Time
	// Sequence is the RFC 5545 SEQUENCE. It starts at 0; SaveEvent adds 1
	// for each material change to a published or cancelled event.
	Sequence int
	// PublishedAt is the UTC time of the last published change, used as
	// DTSTAMP.
	PublishedAt time.Time
	// Revision counts event writes. RosterRevision counts registration
	// writes (Apply). Stores maintain both.
	Revision       Revision
	RosterRevision Revision
	Created        time.Time
	Updated        time.Time
	// Meta is consumer data the store round-trips. This package never reads it.
	Meta map[string]string
}

// TimeZone loads e.Zone.
func (e Event) TimeZone() (*time.Location, error) { return time.LoadLocation(e.Zone) }

// LastDay returns the last included day of an all-day event (EndDate - 1).
// It returns the zero Date for a timed event.
func (e Event) LastDay() Date {
	if !e.AllDay || e.EndDate.IsZero() {
		return Date{}
	}
	return e.EndDate.AddDays(-1)
}

// NormalizeEvent trims text fields, applies defaults (Status draft), derives
// Start and End for all-day events, and validates. It returns a
// *ValidationError (errors.Is ErrInvalid) keyed by field: "id", "title",
// "zone", "start", "end", "start_date", "end_date", "capacity", "booking".
// Rules: ID, Title, and a loadable Zone are required; End is after Start;
// an all-day EndDate is after StartDate; Capacity >= 0; BookingOpens is
// before BookingCloses when both are set.
func NormalizeEvent(e Event) (Event, error) {
	e.ID = strings.TrimSpace(e.ID)
	e.ResourceID = strings.TrimSpace(e.ResourceID)
	e.Title = strings.TrimSpace(e.Title)
	e.Summary = strings.TrimSpace(e.Summary)
	e.Location = strings.TrimSpace(e.Location)
	e.Category = strings.TrimSpace(e.Category)
	e.Visibility = strings.TrimSpace(e.Visibility)
	e.Zone = strings.TrimSpace(e.Zone)
	if e.Status == "" {
		e.Status = EventDraft
	}
	fields := make(map[string]string)
	if e.ID == "" {
		fields["id"] = "Enter an ID."
	}
	if e.Title == "" {
		fields["title"] = "Enter a title."
	}
	loc, err := time.LoadLocation(e.Zone)
	if e.Zone == "" || err != nil {
		fields["zone"] = "Choose a loadable time zone, such as America/Los_Angeles."
		loc = time.UTC
	}
	if e.AllDay {
		if !validDate(e.StartDate) {
			fields["start_date"] = "Choose a valid start date."
		}
		if !validDate(e.EndDate) || !e.EndDate.After(e.StartDate) {
			fields["end_date"] = "Choose an end date after the start date."
		}
		if _, ok := fields["start_date"]; !ok && !e.EndDate.IsZero() {
			e.Start = e.StartDate.Start(loc)
		}
		if _, ok := fields["end_date"]; !ok {
			e.End = e.EndDate.Start(loc)
		}
	} else {
		if e.Start.IsZero() {
			fields["start"] = "Choose a start time."
		}
		if e.End.IsZero() || !e.End.After(e.Start) {
			fields["end"] = "Choose an end time after the start time."
		}
	}
	if e.Capacity < 0 {
		fields["capacity"] = "Capacity cannot be negative."
	}
	if !e.BookingOpens.IsZero() && !e.BookingCloses.IsZero() && !e.BookingOpens.Before(e.BookingCloses) {
		fields["booking"] = "The booking window must open before it closes."
	}
	if len(fields) != 0 {
		return Event{}, &ValidationError{Fields: fields}
	}
	return e, nil
}

// MaterialChange reports whether next differs from prev in a field that
// changes what a calendar client shows.
func MaterialChange(prev, next Event) bool {
	return prev.Title != next.Title || prev.Summary != next.Summary || prev.Location != next.Location ||
		prev.Zone != next.Zone || prev.AllDay != next.AllDay || prev.StartDate != next.StartDate ||
		prev.EndDate != next.EndDate || !prev.Start.Equal(next.Start) || !prev.End.Equal(next.End) || prev.Status != next.Status
}
