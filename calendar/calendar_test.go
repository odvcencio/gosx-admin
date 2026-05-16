package calendar

import (
	"strings"
	"testing"
	"time"

	"github.com/odvcencio/gosx"
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

func TestRenderMonth(t *testing.T) {
	month := time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC)
	view := Month([]Event{
		{
			ID:         "forest-1",
			Title:      "Forest classroom",
			Start:      time.Date(2026, 6, 3, 9, 30, 0, 0, time.UTC),
			End:        time.Date(2026, 6, 3, 12, 0, 0, 0, time.UTC),
			Location:   "Oak grove",
			Status:     StatusOpen,
			Capacity:   12,
			Registered: 8,
			Color:      "#527a4b",
			ResourceID: "forest",
		},
	}, Options{Month: month, WeekStart: time.Monday, Limit: 2})
	html := gosx.RenderHTML(RenderMonth(view, RenderOptions{
		AddHref:      "/admin/calendar/new",
		ShowCapacity: true,
		DayHref: func(day time.Time) string {
			return "/admin/calendar/day/" + day.Format("2006-01-02")
		},
	}))
	for _, want := range []string{
		`class="calendar-widget"`,
		`data-calendar-start="2026-06-01"`,
		`href="/admin/calendar/new"`,
		`href="/admin/calendar/day/2026-06-03"`,
		`data-event-id="forest-1"`,
		`calendar-widget__event--open`,
		`--calendar-event-color: #527a4b`,
		`Forest classroom`,
		`9:30 AM-12:00 PM - Oak grove - 8/12`,
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("expected %q in calendar html: %s", want, html)
		}
	}
}

func TestRenderMonthEmptyState(t *testing.T) {
	view := Month(nil, Options{
		Month:     time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC),
		WeekStart: time.Monday,
	})
	html := gosx.RenderHTML(RenderMonth(view, RenderOptions{
		Class:     "schedule",
		EmptyText: "Nothing on the books.",
	}))
	if !strings.Contains(html, `class="schedule"`) || !strings.Contains(html, "Nothing on the books.") {
		t.Fatalf("expected custom class and empty state: %s", html)
	}
}

func TestEventLabels(t *testing.T) {
	event := Event{
		Title:      "Morning program",
		Start:      time.Date(2026, 6, 3, 9, 0, 0, 0, time.UTC),
		End:        time.Date(2026, 6, 3, 11, 30, 0, 0, time.UTC),
		Capacity:   10,
		Registered: 4,
	}
	if EventTimeLabel(event) != "9:00 AM-11:30 AM" {
		t.Fatalf("unexpected time label: %q", EventTimeLabel(event))
	}
	if CapacityLabel(event) != "4/10" {
		t.Fatalf("unexpected capacity label: %q", CapacityLabel(event))
	}
	event.AllDay = true
	if EventTimeLabel(event) != "All day" {
		t.Fatalf("unexpected all-day label: %q", EventTimeLabel(event))
	}
}

func TestEventLabelsUseTimezone(t *testing.T) {
	event := Event{
		Title:    "Forest program",
		Start:    time.Date(2026, 6, 3, 16, 0, 0, 0, time.UTC),
		End:      time.Date(2026, 6, 3, 18, 30, 0, 0, time.UTC),
		Timezone: "America/Los_Angeles",
	}
	if EventTimeLabel(event) != "9:00 AM-11:30 AM" {
		t.Fatalf("unexpected timezone-adjusted label: %q", EventTimeLabel(event))
	}
}

func TestRenderMonthSanitizesStatusAndColor(t *testing.T) {
	view := Month([]Event{
		{
			ID:     "unsafe",
			Title:  "Registration",
			Start:  time.Date(2026, 6, 3, 9, 0, 0, 0, time.UTC),
			Status: Status("Open Now!"),
			Color:  "#fff;display:block",
		},
	}, Options{Month: time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC), WeekStart: time.Monday})
	html := gosx.RenderHTML(RenderMonth(view, RenderOptions{}))
	if !strings.Contains(html, `calendar-widget__event--open-now`) {
		t.Fatalf("expected sanitized status class: %s", html)
	}
	if strings.Contains(html, `style=`) {
		t.Fatalf("expected invalid color to be dropped: %s", html)
	}
}
