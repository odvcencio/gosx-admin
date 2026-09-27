package schedule_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"golang.org/x/net/html"
	"m31labs.dev/gosx"
	"m31labs.dev/gosx-admin/schedule"
	"m31labs.dev/gosx-admin/schedule/scheduletest"
)

func fixtureEvent(id string) schedule.Event {
	start := scheduletest.Epoch.Add(24 * time.Hour)
	return schedule.Event{ID: id, Title: "A calendar event", Summary: "Summary", Location: "A place", Zone: "UTC", Status: schedule.EventPublished,
		Start: start, End: start.Add(time.Hour), Capacity: 2, Waitlist: true, PublishedAt: scheduletest.Epoch, Created: scheduletest.Epoch, Updated: scheduletest.Epoch}
}

func TestDateRoundTrip(t *testing.T) {
	d, err := schedule.ParseDate("2028-02-29")
	if err != nil {
		t.Fatal(err)
	}
	if got := d.String(); got != "2028-02-29" {
		t.Fatalf("String() = %q", got)
	}
	data, err := d.MarshalText()
	if err != nil {
		t.Fatal(err)
	}
	var decoded schedule.Date
	if err := decoded.UnmarshalText(data); err != nil {
		t.Fatal(err)
	}
	if decoded != d {
		t.Fatalf("text round trip = %#v, want %#v", decoded, d)
	}
	if _, err := schedule.ParseDate("2027-02-30"); err == nil {
		t.Fatal("ParseDate accepted 2027-02-30")
	}
	if got, err := (schedule.Date{}).MarshalText(); err != nil || string(got) != "" {
		t.Fatalf("zero MarshalText = %q, %v", got, err)
	}
	var zero schedule.Date
	if err := zero.UnmarshalText(nil); err != nil || !zero.IsZero() {
		t.Fatalf("empty UnmarshalText = %#v, %v", zero, err)
	}
}

func TestDateAddDaysAcrossMonthAndYear(t *testing.T) {
	if got := (schedule.Date{Year: 2027, Month: time.December, Day: 31}).AddDays(1); got != (schedule.Date{Year: 2028, Month: time.January, Day: 1}) {
		t.Fatalf("new year = %#v", got)
	}
	if got := (schedule.Date{Year: 2028, Month: time.March, Day: 1}).AddDays(-1); got != (schedule.Date{Year: 2028, Month: time.February, Day: 29}) {
		t.Fatalf("leap day = %#v", got)
	}
}

func TestDateStartSkippedMidnight(t *testing.T) {
	loc, err := time.LoadLocation("America/Santiago")
	if err != nil {
		t.Fatal(err)
	}
	got := (schedule.Date{Year: 2027, Month: time.September, Day: 5}).Start(loc)
	want := time.Date(2027, 9, 5, 4, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Fatalf("Start = %s, want %s", got, want)
	}
}

func TestDateStartEveryDay2027(t *testing.T) {
	for _, zone := range []string{"America/Santiago", "America/Los_Angeles", "Asia/Kathmandu"} {
		loc, err := time.LoadLocation(zone)
		if err != nil {
			t.Fatal(err)
		}
		for d := (schedule.Date{Year: 2027, Month: time.January, Day: 1}); d.Year == 2027; d = d.AddDays(1) {
			start := d.Start(loc)
			if got := schedule.DateOf(start, loc); got != d {
				t.Fatalf("%s Start(%s) local date = %s", zone, d, got)
			}
			if got := schedule.DateOf(start.Add(-time.Nanosecond), loc); got != d.AddDays(-1) {
				t.Fatalf("%s one ns before Start(%s) = %s", zone, d, got)
			}
		}
	}
}

func TestNormalizeEventRules(t *testing.T) {
	valid := fixtureEvent("valid")
	cases := []struct {
		name, key string
		mutate    func(*schedule.Event)
	}{
		{"id required", "id", func(e *schedule.Event) { e.ID = "" }},
		{"title required", "title", func(e *schedule.Event) { e.Title = "  " }},
		{"zone required and loadable", "zone", func(e *schedule.Event) { e.Zone = "Not/A_Real_Zone" }},
		{"start required", "start", func(e *schedule.Event) { e.Start = time.Time{} }},
		{"end after start", "end", func(e *schedule.Event) { e.End = e.Start }},
		{"all day start date", "start_date", func(e *schedule.Event) { e.AllDay = true; e.StartDate = schedule.Date{} }},
		{"all day end after start", "end_date", func(e *schedule.Event) {
			e.AllDay = true
			e.StartDate = schedule.Date{Year: 2027, Month: time.January, Day: 1}
			e.EndDate = e.StartDate
		}},
		{"nonnegative capacity", "capacity", func(e *schedule.Event) { e.Capacity = -1 }},
		{"booking window order", "booking", func(e *schedule.Event) {
			e.BookingOpens = scheduletest.Epoch.Add(time.Hour)
			e.BookingCloses = scheduletest.Epoch
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := valid
			tc.mutate(&e)
			_, err := schedule.NormalizeEvent(e)
			if !errors.Is(err, schedule.ErrInvalid) {
				t.Fatalf("error = %v, want ErrInvalid", err)
			}
			var validation *schedule.ValidationError
			if !errors.As(err, &validation) {
				t.Fatalf("error type = %T", err)
			}
			if _, ok := validation.Fields[tc.key]; !ok {
				t.Fatalf("fields = %#v, want %q", validation.Fields, tc.key)
			}
		})
	}
	blank := valid
	blank.Status = ""
	normalized, err := schedule.NormalizeEvent(blank)
	if err != nil || normalized.Status != schedule.EventDraft {
		t.Fatalf("default status = %q, %v", normalized.Status, err)
	}
}

func TestNormalizeAllDayDerivesInstants(t *testing.T) {
	e := fixtureEvent("all-day")
	e.AllDay = true
	e.Zone = "America/Santiago"
	e.StartDate = schedule.Date{Year: 2027, Month: time.September, Day: 5}
	e.EndDate = e.StartDate.AddDays(2)
	got, err := schedule.NormalizeEvent(e)
	if err != nil {
		t.Fatal(err)
	}
	loc, _ := time.LoadLocation(e.Zone)
	if !got.Start.Equal(e.StartDate.Start(loc)) || !got.End.Equal(e.EndDate.Start(loc)) {
		t.Fatalf("instants = %s to %s", got.Start, got.End)
	}
}

func TestMaterialChange(t *testing.T) {
	base := fixtureEvent("event")
	material := map[string]func(*schedule.Event){
		"title": func(e *schedule.Event) { e.Title = "other" }, "summary": func(e *schedule.Event) { e.Summary = "other" }, "location": func(e *schedule.Event) { e.Location = "other" },
		"zone": func(e *schedule.Event) { e.Zone = "America/Los_Angeles" }, "all day": func(e *schedule.Event) { e.AllDay = true },
		"start date": func(e *schedule.Event) { e.StartDate = schedule.Date{Year: 2027, Month: time.January, Day: 1} }, "end date": func(e *schedule.Event) { e.EndDate = schedule.Date{Year: 2027, Month: time.January, Day: 2} },
		"start": func(e *schedule.Event) { e.Start = e.Start.Add(time.Minute) }, "end": func(e *schedule.Event) { e.End = e.End.Add(time.Minute) }, "status": func(e *schedule.Event) { e.Status = schedule.EventCancelled },
	}
	for name, change := range material {
		t.Run(name, func(t *testing.T) {
			next := base
			change(&next)
			if !schedule.MaterialChange(base, next) {
				t.Fatal("MaterialChange = false")
			}
		})
	}
	for name, change := range map[string]func(*schedule.Event){"category": func(e *schedule.Event) { e.Category = "field trip" }, "visibility": func(e *schedule.Event) { e.Visibility = "public" }, "capacity": func(e *schedule.Event) { e.Capacity = 3 }, "meta": func(e *schedule.Event) { e.Meta = map[string]string{"x": "y"} }} {
		t.Run(name, func(t *testing.T) {
			next := base
			change(&next)
			if schedule.MaterialChange(base, next) {
				t.Fatal("MaterialChange = true")
			}
		})
	}
}

func TestOccupyHoldBoundary(t *testing.T) {
	e := fixtureEvent("event")
	e.Capacity = 2
	regs := []schedule.Registration{{EventID: e.ID, Seats: 1, State: schedule.StateConfirmed}, {EventID: e.ID, Seats: 1, State: schedule.StateHeld, HoldUntil: scheduletest.Epoch.Add(15 * time.Minute)}}
	before := schedule.Occupy(e, regs, scheduletest.Epoch.Add(14*time.Minute+59*time.Second))
	after := schedule.Occupy(e, regs, scheduletest.Epoch.Add(15*time.Minute))
	if before.Used != 2 || before.Remaining != 0 || after.Used != 1 || after.Remaining != 1 {
		t.Fatalf("before=%#v after=%#v", before, after)
	}
}

func TestOccupyOverride(t *testing.T) {
	e := fixtureEvent("event")
	e.Capacity = 2
	regs := []schedule.Registration{{EventID: e.ID, Seats: 1, State: schedule.StateConfirmed}, {EventID: e.ID, Seats: 2, State: schedule.StateConfirmed, Override: true}}
	got := schedule.Occupy(e, regs, scheduletest.Epoch)
	if got.Over != 1 || !got.Full || got.Used != 3 {
		t.Fatalf("Occupancy = %#v", got)
	}
}

func TestAvailabilityOrder(t *testing.T) {
	at := scheduletest.Epoch
	cases := []struct {
		name      string
		event     schedule.Event
		occupancy schedule.Occupancy
		at        time.Time
		want      schedule.Availability
	}{
		{"draft closed", schedule.Event{Status: schedule.EventDraft}, schedule.Occupancy{}, at, schedule.AvailabilityClosed},
		{"before opens", schedule.Event{Status: schedule.EventPublished, Start: at.Add(time.Hour), BookingOpens: at.Add(time.Minute)}, schedule.Occupancy{}, at, schedule.AvailabilityClosed},
		{"at close", schedule.Event{Status: schedule.EventPublished, Start: at.Add(time.Hour), BookingCloses: at}, schedule.Occupancy{}, at, schedule.AvailabilityClosed},
		{"at start", schedule.Event{Status: schedule.EventPublished, Start: at}, schedule.Occupancy{}, at, schedule.AvailabilityClosed},
		{"available", schedule.Event{Status: schedule.EventPublished, Start: at.Add(time.Hour)}, schedule.Occupancy{Capacity: 1, Remaining: 1}, at, schedule.AvailabilityOpen},
		{"waitlist", schedule.Event{Status: schedule.EventPublished, Start: at.Add(time.Hour), Waitlist: true}, schedule.Occupancy{Capacity: 1, Full: true}, at, schedule.AvailabilityWaitlist},
		{"full", schedule.Event{Status: schedule.EventPublished, Start: at.Add(time.Hour)}, schedule.Occupancy{Capacity: 1, Full: true}, at, schedule.AvailabilityFull},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := schedule.AvailabilityAt(tc.event, tc.occupancy, tc.at); got != tc.want {
				t.Fatalf("AvailabilityAt = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestWaitlistOrderStable(t *testing.T) {
	at := scheduletest.Epoch
	regs := []schedule.Registration{{ID: "z", State: schedule.StateWaitlisted, RequestedAt: at}, {ID: "b", State: schedule.StateWaitlisted, RequestedAt: at}, {ID: "a", State: schedule.StateWaitlisted, RequestedAt: at}, {ID: "confirmed", State: schedule.StateConfirmed, RequestedAt: at}}
	got := schedule.WaitlistOrder(regs)
	if len(got) != 3 || got[0].ID != "a" || got[1].ID != "b" || got[2].ID != "z" {
		t.Fatalf("order = %#v", got)
	}
}

func TestBookDecisionTable(t *testing.T) {
	cases := []struct {
		name      string
		prepare   func(*schedule.MemoryStore, schedule.Event)
		request   schedule.BookRequest
		wantState schedule.RegistrationState
		wantErr   error
	}{
		{"closed", func(_ *schedule.MemoryStore, e schedule.Event) { e.Status = schedule.EventDraft }, schedule.BookRequest{}, "", schedule.ErrClosed},
		{"review", func(s *schedule.MemoryStore, e schedule.Event) { bookConfirmed(t, s, e, "occupied") }, schedule.BookRequest{Review: true}, schedule.StateRequested, nil},
		{"fits", func(_ *schedule.MemoryStore, _ schedule.Event) {}, schedule.BookRequest{}, schedule.StateConfirmed, nil},
		{"override", func(s *schedule.MemoryStore, e schedule.Event) { bookConfirmed(t, s, e, "occupied") }, schedule.BookRequest{Override: "approved override"}, schedule.StateConfirmed, nil},
		{"waitlist", func(s *schedule.MemoryStore, e schedule.Event) { bookConfirmed(t, s, e, "occupied") }, schedule.BookRequest{}, schedule.StateWaitlisted, nil},
		{"full", func(s *schedule.MemoryStore, e schedule.Event) { bookConfirmed(t, s, e, "occupied") }, schedule.BookRequest{}, "", schedule.ErrFull},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store := schedule.NewMemoryStore()
			e := fixtureEvent("event")
			e.Capacity = 1
			if tc.name == "closed" {
				e.Status = schedule.EventDraft
			}
			if tc.name == "full" {
				e.Waitlist = false
			}
			if _, err := store.PutEvent(context.Background(), schedule.EventWrite{Event: e}); err != nil {
				t.Fatal(err)
			}
			tc.prepare(store, e)
			req := tc.request
			req.EventID = e.ID
			req.RegistrationID = "new"
			req.PartyID = "party"
			req.Seats = 1
			req.Key = "new-key"
			req.At = scheduletest.Epoch
			got, err := schedule.Book(context.Background(), store, req)
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("Book error = %v, want %v", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got.Registration.State != tc.wantState {
				t.Fatalf("state = %q, want %q", got.Registration.State, tc.wantState)
			}
			if tc.name == "override" && (!got.Registration.Override || got.Registration.Reason != "approved override") {
				t.Fatalf("override not recorded: %#v", got.Registration)
			}
		})
	}
}

func bookConfirmed(t *testing.T, s *schedule.MemoryStore, e schedule.Event, id string) {
	t.Helper()
	_, err := schedule.Book(context.Background(), s, schedule.BookRequest{EventID: e.ID, RegistrationID: id, PartyID: id, Seats: 1, Key: "key-" + id, At: scheduletest.Epoch})
	if err != nil {
		t.Fatal(err)
	}
}

func TestSaveEventSequenceVector(t *testing.T) {
	store := schedule.NewMemoryStore()
	e := fixtureEvent("event")
	e.Sequence = 2
	e.PublishedAt = scheduletest.Epoch.Add(-time.Hour)
	stored, err := store.PutEvent(context.Background(), schedule.EventWrite{Event: e})
	if err != nil {
		t.Fatal(err)
	}
	updatedInput := stored
	updatedInput.Start = updatedInput.Start.Add(time.Hour)
	updatedInput.End = updatedInput.End.Add(time.Hour)
	updated, err := schedule.SaveEvent(context.Background(), store, schedule.SaveEventRequest{Event: updatedInput, Expected: stored.Revision, Key: "time-change", At: scheduletest.Epoch})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Event.Sequence != 3 || !updated.Event.PublishedAt.Equal(scheduletest.Epoch) {
		t.Fatalf("time edit sequence/stamp = %d/%s", updated.Event.Sequence, updated.Event.PublishedAt)
	}
	cancelInput := updated.Event
	cancelInput.Status = schedule.EventCancelled
	cancelled, err := schedule.SaveEvent(context.Background(), store, schedule.SaveEventRequest{Event: cancelInput, Expected: updated.Event.Revision, Key: "cancel", At: scheduletest.Epoch.Add(time.Minute)})
	if err != nil {
		t.Fatal(err)
	}
	if cancelled.Event.Sequence != 4 {
		t.Fatalf("cancel sequence = %d", cancelled.Event.Sequence)
	}
	draft := fixtureEvent("draft")
	draft.Status = schedule.EventDraft
	created, err := schedule.SaveEvent(context.Background(), store, schedule.SaveEventRequest{Event: draft, Key: "draft", At: scheduletest.Epoch})
	if err != nil {
		t.Fatal(err)
	}
	draft = created.Event
	draft.Title = "Draft edit"
	draftEdit, err := schedule.SaveEvent(context.Background(), store, schedule.SaveEventRequest{Event: draft, Expected: created.Event.Revision, Key: "draft-edit", At: scheduletest.Epoch.Add(time.Minute)})
	if err != nil {
		t.Fatal(err)
	}
	if draftEdit.Event.Sequence != 0 {
		t.Fatalf("draft edit sequence = %d", draftEdit.Event.Sequence)
	}
	published := fixtureEvent("published")
	published.ID = "draft-publish"
	published.Status = schedule.EventDraft
	first, err := schedule.SaveEvent(context.Background(), store, schedule.SaveEventRequest{Event: published, Key: "draft-publish", At: scheduletest.Epoch})
	if err != nil {
		t.Fatal(err)
	}
	published = first.Event
	published.Status = schedule.EventPublished
	firstPub, err := schedule.SaveEvent(context.Background(), store, schedule.SaveEventRequest{Event: published, Expected: first.Event.Revision, Key: "first-pub", At: scheduletest.Epoch.Add(time.Minute)})
	if err != nil {
		t.Fatal(err)
	}
	if firstPub.Event.Sequence != 0 || !firstPub.Event.PublishedAt.Equal(scheduletest.Epoch.Add(time.Minute)) {
		t.Fatalf("first publication = %#v", firstPub.Event)
	}
	backToDraft := firstPub.Event
	backToDraft.Status = schedule.EventDraft
	if _, err := schedule.SaveEvent(context.Background(), store, schedule.SaveEventRequest{Event: backToDraft, Expected: firstPub.Event.Revision, Key: "unpublish", At: scheduletest.Epoch.Add(2 * time.Minute)}); !errors.Is(err, schedule.ErrTransition) {
		t.Fatalf("published to draft error = %v", err)
	}
}

func TestSaveEventCapacityCutGuard(t *testing.T) {
	for _, override := range []string{"", "owner approved"} {
		t.Run(fmt.Sprintf("override-%t", override != ""), func(t *testing.T) {
			store := schedule.NewMemoryStore()
			e := fixtureEvent("event")
			e.Capacity = 2
			stored, err := store.PutEvent(context.Background(), schedule.EventWrite{Event: e})
			if err != nil {
				t.Fatal(err)
			}
			for i := 0; i < 2; i++ {
				_, err := schedule.Book(context.Background(), store, schedule.BookRequest{EventID: e.ID, RegistrationID: fmt.Sprintf("r%d", i), Seats: 1, Key: fmt.Sprintf("book%d", i), At: scheduletest.Epoch})
				if err != nil {
					t.Fatal(err)
				}
			}
			next, err := store.Event(context.Background(), e.ID)
			if err != nil {
				t.Fatal(err)
			}
			next.Capacity = 1
			result, err := schedule.SaveEvent(context.Background(), store, schedule.SaveEventRequest{Event: next, Expected: stored.Revision, Override: override, Key: "cut", Actor: "owner", At: scheduletest.Epoch})
			if override == "" {
				var ve *schedule.ValidationError
				if !errors.As(err, &ve) || ve.Fields["capacity"] == "" {
					t.Fatalf("cut error = %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if result.Event.Capacity != 1 || result.Event.RosterRevision != 3 {
				t.Fatalf("cut result = %#v", result.Event)
			}
		})
	}
}

func TestMonthGridShape(t *testing.T) {
	month := schedule.Date{Year: 2027, Month: time.February, Day: 1}
	today := schedule.Date{Year: 2027, Month: time.February, Day: 15}
	view := schedule.Month(nil, month, schedule.ViewOptions{WeekStart: time.Sunday, Today: today})
	if len(view.Weeks) != 6 {
		t.Fatalf("weeks = %d", len(view.Weeks))
	}
	for i, w := range view.Weeks {
		if len(w) != 7 {
			t.Fatalf("week %d days=%d", i, len(w))
		}
	}
	if view.Weeks[0][0].Date != (schedule.Date{Year: 2027, Month: time.January, Day: 31}) {
		t.Fatalf("first grid day = %#v", view.Weeks[0][0].Date)
	}
	if view.Weeks[2][1].Date != today || !view.Weeks[2][1].Today {
		t.Fatalf("today cell = %#v", view.Weeks[2][1])
	}
	if view.Weeks[0][0].InMonth || !view.Weeks[0][1].InMonth {
		t.Fatalf("InMonth flags are wrong")
	}
}

func TestMonthMultiDayAndLimit(t *testing.T) {
	start := schedule.Date{Year: 2027, Month: time.February, Day: 15}
	end := start.AddDays(3)
	span := fixtureEvent("span")
	span.AllDay, span.StartDate, span.EndDate = true, start, end
	events := []schedule.Event{span}
	for _, id := range []string{"one", "two", "three"} {
		e := fixtureEvent(id)
		e.AllDay = true
		e.StartDate = start
		e.EndDate = start.AddDays(1)
		events = append(events, e)
	}
	view := schedule.Month(events, schedule.Date{Year: 2027, Month: time.February, Day: 1}, schedule.ViewOptions{WeekStart: time.Monday, Limit: 2})
	var first, last *schedule.DayCell
	for wi := range view.Weeks {
		for di := range view.Weeks[wi] {
			cell := &view.Weeks[wi][di]
			if cell.Date == start {
				first = cell
			}
			if cell.Date == start.AddDays(2) {
				last = cell
			}
		}
	}
	if first == nil || last == nil {
		t.Fatal("target day missing")
	}
	if len(first.Items) != 2 || first.More != 2 {
		t.Fatalf("limited day has %d items and %d more", len(first.Items), first.More)
	}
	var firstSpan, lastSpan bool
	for _, item := range first.Items {
		if item.Event.ID == "span" {
			firstSpan = item.First && !item.Last
		}
	}
	for _, item := range last.Items {
		if item.Event.ID == "span" {
			lastSpan = !item.First && item.Last
		}
	}
	if !firstSpan || !lastSpan {
		t.Fatalf("multi-day flags first=%t last=%t", firstSpan, lastSpan)
	}
}

func TestTimedEventPlacementAcrossZones(t *testing.T) {
	la, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		t.Fatal(err)
	}
	e := fixtureEvent("late")
	e.Zone = "America/Los_Angeles"
	e.Start = time.Date(2027, 1, 15, 23, 30, 0, 0, la)
	e.End = e.Start.Add(time.Hour)
	view := schedule.Month([]schedule.Event{e}, schedule.Date{Year: 2027, Month: time.January, Day: 1}, schedule.ViewOptions{Zone: time.UTC, WeekStart: time.Sunday})
	day := schedule.Date{Year: 2027, Month: time.January, Day: 16}
	var found bool
	for _, week := range view.Weeks {
		for _, cell := range week {
			if cell.Date == day {
				for _, item := range cell.Items {
					if item.Event.ID == e.ID {
						found = true
						if !strings.Contains(item.TimeLabel, "PST") {
							t.Fatalf("time label = %q, want PST", item.TimeLabel)
						}
					}
				}
			}
		}
	}
	if !found {
		t.Fatal("event was not placed on January 16 in the UTC grid")
	}
}

func TestAgendaSkipsEmptyDays(t *testing.T) {
	from := schedule.Date{Year: 2027, Month: time.February, Day: 1}
	e1, e2 := fixtureEvent("a"), fixtureEvent("b")
	e1.AllDay = true
	e1.StartDate = from.AddDays(1)
	e1.EndDate = from.AddDays(2)
	e2.AllDay = true
	e2.StartDate = from.AddDays(3)
	e2.EndDate = from.AddDays(4)
	view := schedule.Agenda([]schedule.Event{e1, e2}, from, from.AddDays(5), schedule.ViewOptions{})
	if len(view.Days) != 2 || view.Days[0].Date != from.AddDays(1) || view.Days[1].Date != from.AddDays(3) {
		t.Fatalf("agenda days = %#v", view.Days)
	}
}

func TestRenderMonthContract(t *testing.T) {
	d := schedule.Date{Year: 2027, Month: time.February, Day: 15}
	e := fixtureEvent("cancelled")
	e.AllDay = true
	e.StartDate = d
	e.EndDate = d.AddDays(1)
	e.Status = schedule.EventCancelled
	e.Category = "public label"
	view := schedule.Month([]schedule.Event{e}, schedule.Date{Year: 2027, Month: time.February, Day: 1}, schedule.ViewOptions{Today: d, WeekStart: time.Monday})
	markup := gosx.RenderHTML(schedule.RenderMonth(view, schedule.RenderOptions{HeadingLevel: 2, ID: "calendar-main", CategoryLabel: func(s string) string { return s }, EventHref: func(e schedule.Event) string { return "/events/" + e.ID }, DayHref: func(d schedule.Date) string { return "/days/" + d.String() }, PageHref: func(d schedule.Date) string { return "/month/" + d.String() }}))
	for _, want := range []string{`role="table"`, `role="rowgroup"`, `role="row"`, `role="columnheader"`, `role="cell"`, `<abbr title="Monday">Mon</abbr>`, `aria-current="date"`, `gxa-visually-hidden gxa-cal__full-label`, `Cancelled`, `public label`, `rel="prev"`, `rel="next"`, `aria-labelledby="calendar-main-title"`} {
		if !strings.Contains(markup, want) {
			t.Errorf("markup missing %q", want)
		}
	}
	if err := checkCalendarMarkup(markup); err != nil {
		t.Fatalf("accessibility check: %v\n%s", err, markup)
	}
}

func TestRenderWeekAndAgendaHeadings(t *testing.T) {
	day := schedule.Date{Year: 2027, Month: time.February, Day: 1}
	week := schedule.Week(nil, day, schedule.ViewOptions{Today: day, WeekStart: time.Monday})
	weekHTML := gosx.RenderHTML(schedule.RenderWeek(week, schedule.RenderOptions{HeadingLevel: 2}))
	if !strings.Contains(weekHTML, "<h2") || !strings.Contains(weekHTML, "<h3") {
		t.Fatalf("week headings are not nested: %s", weekHTML)
	}
	if !strings.Contains(weekHTML, `aria-current="date"`) || !strings.Contains(weekHTML, `gxa-cal__day--today`) {
		t.Fatalf("week does not mark today: %s", weekHTML)
	}
	agendaEvent := fixtureEvent("today")
	agendaEvent.Start = day.Start(time.UTC).Add(time.Hour)
	agendaEvent.End = agendaEvent.Start.Add(time.Hour)
	agenda := schedule.Agenda([]schedule.Event{agendaEvent}, day, day.AddDays(7), schedule.ViewOptions{Today: day})
	agendaHTML := gosx.RenderHTML(schedule.RenderAgenda(agenda, schedule.RenderOptions{}))
	if !strings.Contains(agendaHTML, `aria-current="date"`) || !strings.Contains(agendaHTML, `gxa-cal__day--today`) {
		t.Fatalf("agenda does not mark today: %s", agendaHTML)
	}
	emptyAgenda := schedule.Agenda(nil, day, day.AddDays(7), schedule.ViewOptions{})
	emptyAgendaHTML := gosx.RenderHTML(schedule.RenderAgenda(emptyAgenda, schedule.RenderOptions{}))
	if !strings.Contains(emptyAgendaHTML, "No events.") || !strings.Contains(emptyAgendaHTML, `class="gxa-cal__empty"`) {
		t.Fatalf("empty agenda = %s", emptyAgendaHTML)
	}
}

func TestMonthInvalidWeekStartDefaultsSunday(t *testing.T) {
	month := schedule.Month(nil, schedule.Date{Year: 2027, Month: time.February, Day: 1}, schedule.ViewOptions{WeekStart: time.Weekday(9)})
	if len(month.Weekdays) != 7 || month.Weekdays[0] != "Sunday" || month.Weeks[0][0].Date.Weekday() != time.Sunday {
		t.Fatalf("invalid WeekStart result = %#v", month)
	}
}

func TestLabelsSpanish(t *testing.T) {
	labels := schedule.EnglishLabels()
	labels.Months = [12]string{"enero", "febrero", "marzo", "abril", "mayo", "junio", "julio", "agosto", "septiembre", "octubre", "noviembre", "diciembre"}
	labels.Weekdays = [7]string{"domingo", "lunes", "martes", "miércoles", "jueves", "viernes", "sábado"}
	labels.WeekdaysShort = [7]string{"dom", "lun", "mar", "mié", "jue", "vie", "sáb"}
	labels.AllDay = "Todo el día"
	labels.Cancelled = "Cancelado"
	labels.Draft = "Borrador"
	labels.MoreFormat = "+%d más"
	labels.Empty = "Sin eventos."
	labels.PrevMonth = "Mes anterior"
	labels.NextMonth = "Mes siguiente"
	labels.PrevWeek = "Semana anterior"
	labels.NextWeek = "Semana siguiente"
	labels.TimeFormat = "15:04"
	labels.DayLabel = func(d schedule.Date) string {
		return fmt.Sprintf("%s, %d de %s", labels.Weekdays[int(d.Weekday())], d.Day, labels.Months[int(d.Month)-1])
	}
	labels.MonthTitle = func(y int, m time.Month) string { return fmt.Sprintf("%s de %d", labels.Months[int(m)-1], y) }
	d := schedule.Date{Year: 2027, Month: time.February, Day: 1}
	e := fixtureEvent("e")
	e.AllDay = true
	e.StartDate = d
	e.EndDate = d.AddDays(1)
	e.Status = schedule.EventCancelled
	view := schedule.Month([]schedule.Event{e}, d, schedule.ViewOptions{Labels: labels, WeekStart: time.Monday})
	markup := gosx.RenderHTML(schedule.RenderMonth(view, schedule.RenderOptions{Labels: labels, PageHref: func(schedule.Date) string { return "#" }}))
	for _, want := range []string{"febrero de 2027", "Mes anterior", "Mes siguiente", "lunes", "lun", "Todo el día", "Cancelado"} {
		if !strings.Contains(markup, want) {
			t.Errorf("Spanish markup missing %q", want)
		}
	}
	for _, english := range []string{"February", "Previous month", "Next month", "All day", "Cancelled"} {
		if strings.Contains(markup, english) {
			t.Errorf("English text %q remains", english)
		}
	}
}

func checkCalendarMarkup(markup string) error {
	doc, err := html.Parse(strings.NewReader(markup))
	if err != nil {
		return err
	}
	ids := map[string]bool{}
	idrefs := make([]string, 0)
	anchors := make([]*html.Node, 0)
	tables := 0
	var visit func(*html.Node)
	visit = func(n *html.Node) {
		if n.Type == html.ElementNode {
			for _, a := range n.Attr {
				switch a.Key {
				case "id":
					if ids[a.Val] {
						err = fmt.Errorf("duplicate id %q", a.Val)
					}
					ids[a.Val] = true
				case "aria-labelledby":
					idrefs = append(idrefs, strings.Fields(a.Val)...)
				}
			}
			if n.Data == "table" {
				tables++
				if attr(n, "role") != "table" {
					err = fmt.Errorf("calendar table missing role")
				}
			}
			if n.Data == "abbr" && attr(n, "title") == "" {
				err = fmt.Errorf("weekday abbreviation missing title")
			}
			if n.Data == "a" {
				anchors = append(anchors, n)
			}
			if n.Data == "time" && attr(n, "datetime") == "" {
				err = fmt.Errorf("time missing datetime")
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			visit(c)
		}
	}
	visit(doc)
	if err != nil {
		return err
	}
	if tables != 1 {
		return fmt.Errorf("tables = %d", tables)
	}
	for _, ref := range idrefs {
		if !ids[ref] {
			return fmt.Errorf("aria-labelledby target %q not found", ref)
		}
	}
	for _, a := range anchors {
		if strings.TrimSpace(textContent(a)) == "" {
			return fmt.Errorf("anchor has no text")
		}
	}
	return nil
}

func attr(n *html.Node, name string) string {
	for _, a := range n.Attr {
		if a.Key == name {
			return a.Val
		}
	}
	return ""
}
func textContent(n *html.Node) string {
	var b strings.Builder
	var visit func(*html.Node)
	visit = func(n *html.Node) {
		if n.Type == html.TextNode {
			b.WriteString(n.Data)
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			visit(c)
		}
	}
	visit(n)
	return b.String()
}
