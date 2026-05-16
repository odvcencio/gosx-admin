package calendar

import (
	"testing"
	"time"
)

func TestNormalizeEvent(t *testing.T) {
	start := time.Date(2026, 6, 1, 9, 0, 0, 0, time.UTC)
	event, ok := NormalizeEvent(Event{Title: " Forest day ", Start: start, Registered: -1, Capacity: -2})
	if !ok {
		t.Fatal("expected valid event")
	}
	if event.Title != "Forest day" || event.End != start || event.Status != StatusScheduled || event.Registered != 0 || event.Capacity != 0 {
		t.Fatalf("unexpected normalized event: %#v", event)
	}
	if _, ok := NormalizeEvent(Event{Start: start}); ok {
		t.Fatal("expected missing title to be invalid")
	}
}

func TestFilterEvents(t *testing.T) {
	from := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	to := from.AddDate(0, 0, 7)
	events := []Event{
		{Title: "Forest day", Start: from.Add(9 * time.Hour), ResourceKind: "program", ResourceID: "forest", Status: StatusOpen},
		{Title: "Garden day", Start: from.AddDate(0, 0, 2), ResourceKind: "program", ResourceID: "garden", Status: StatusOpen},
		{Title: "Cancelled", Start: from.AddDate(0, 0, 3), ResourceKind: "program", ResourceID: "forest", Status: StatusCancelled},
		{Title: "Later", Start: to.AddDate(0, 0, 1), ResourceKind: "program", ResourceID: "forest", Status: StatusOpen},
	}
	filtered := FilterEvents(events, Filter{
		From:         from,
		To:           to,
		ResourceKind: "program",
		ResourceID:   "forest",
		Statuses:     []Status{StatusOpen},
	})
	if len(filtered) != 1 || filtered[0].Title != "Forest day" {
		t.Fatalf("unexpected filtered events: %#v", filtered)
	}
}

func TestMonthBuildsSixWeekGridWithEvents(t *testing.T) {
	month := time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC)
	eventDay := time.Date(2026, 6, 3, 9, 0, 0, 0, time.UTC)
	view := Month([]Event{
		{Title: "Forest day", Start: eventDay, Status: StatusOpen},
		{Title: "Garden day", Start: eventDay.Add(time.Hour), Status: StatusOpen},
	}, Options{Month: month, Today: eventDay, WeekStart: time.Monday, Limit: 1})
	if view.Label != "June 2026" || len(view.Weeks) != 6 || len(view.Weeks[0].Days) != 7 || !view.HasEvents {
		t.Fatalf("unexpected month view: %#v", view)
	}
	if !view.Start.Equal(time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("expected grid to start on Monday June 1, got %s", view.Start)
	}
	day := view.Weeks[0].Days[2]
	if !day.Today || !day.InMonth || day.EventCount != 2 || day.OverflowCount != 1 || len(day.Events) != 1 {
		t.Fatalf("unexpected event day: %#v", day)
	}
}

func TestMonthIncludesPriorMonthLeadDays(t *testing.T) {
	month := time.Date(2026, 8, 10, 0, 0, 0, 0, time.UTC)
	view := Month(nil, Options{Month: month, WeekStart: time.Monday})
	if !view.Start.Equal(time.Date(2026, 7, 27, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("expected July 27 grid start, got %s", view.Start)
	}
	if view.Weeks[0].Days[0].InMonth {
		t.Fatalf("expected leading day outside current month: %#v", view.Weeks[0].Days[0])
	}
}
