package calendarmigrate_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"m31labs.dev/gosx-admin/calendar"
	"m31labs.dev/gosx-admin/schedule"
	"m31labs.dev/gosx-admin/schedule/calendarmigrate"
)

var now = time.Date(2027, 1, 2, 12, 0, 0, 0, time.UTC)

func TestCalendarMigrateConvert(t *testing.T) {
	start := time.Date(2027, 6, 18, 10, 0, 0, 0, time.UTC)
	seed := calendar.MemorySeed{
		Resources: []calendar.Resource{{ID: "room", Kind: "space", Label: "Studio", Description: "Useful room", Color: "red", Capacity: 12, Href: "/old"}},
		Events: []calendar.Event{
			{ID: "draft", Title: "Draft", Description: "Summary", Start: start, End: start, Timezone: "America/Los_Angeles", Status: calendar.StatusDraft, ResourceKind: "old-kind", ResourceID: "room", Color: "red", Registered: 7, Href: "/event"},
			{ID: "fallback", Title: "Fallback", Start: start, End: start.Add(time.Hour), Status: calendar.StatusOpen},
			{ID: "all-day", Title: "Two days", Start: time.Date(2027, 6, 18, 0, 0, 0, 0, time.UTC), End: time.Date(2027, 6, 19, 12, 0, 0, 0, time.UTC), AllDay: true, Timezone: "UTC", Status: calendar.StatusScheduled},
			{ID: "end-empty", Title: "One day", Start: start, AllDay: true, Timezone: "UTC", Status: calendar.StatusFull},
			{ID: "cancelled", Title: "Cancelled", Start: start, End: start.Add(time.Hour), Timezone: "UTC", Status: calendar.StatusCancelled},
		},
		Registrations: []calendar.Registration{
			{ID: "pending", EventID: "draft", Name: "Alex Adult", Email: "alex@example.test", Quantity: 2, Status: calendar.RegistrationPending, Notes: "note", Created: now},
			{ID: "confirmed", EventID: "draft", Name: "Blair Adult", Email: "blair@example.test", Quantity: 3, Status: calendar.RegistrationConfirmed, Created: now},
			{ID: "waitlist", EventID: "draft", Status: calendar.RegistrationWaitlist},
			{ID: "cancelled-reg", EventID: "draft", Status: calendar.RegistrationCancelled},
		},
	}
	converted, err := calendarmigrate.Convert(seed, "America/Los_Angeles", now)
	if err != nil {
		t.Fatal(err)
	}
	if len(converted.Resources) != 1 {
		t.Fatalf("resources = %d", len(converted.Resources))
	}
	resource := converted.Resources[0]
	if resource.ID != "room" || resource.Kind != "space" || resource.Label != "Studio" || resource.Summary != "Useful room" || resource.Capacity != 12 {
		t.Fatalf("resource conversion = %#v", resource)
	}
	byID := map[string]schedule.Event{}
	for _, e := range converted.Events {
		byID[e.ID] = e
	}
	if byID["draft"].Summary != "Summary" || byID["draft"].Zone != "America/Los_Angeles" || byID["draft"].Status != schedule.EventDraft || byID["draft"].ResourceID != "room" {
		t.Fatalf("draft conversion = %#v", byID["draft"])
	}
	if byID["draft"].End.Sub(byID["draft"].Start) != time.Hour {
		t.Fatalf("equal end duration = %s", byID["draft"].End.Sub(byID["draft"].Start))
	}
	if byID["fallback"].Zone != "America/Los_Angeles" || byID["fallback"].Status != schedule.EventPublished {
		t.Fatalf("fallback conversion = %#v", byID["fallback"])
	}
	if byID["all-day"].StartDate != (schedule.Date{Year: 2027, Month: time.June, Day: 18}) || byID["all-day"].EndDate != (schedule.Date{Year: 2027, Month: time.June, Day: 20}) {
		t.Fatalf("all-day dates = %s through %s", byID["all-day"].StartDate, byID["all-day"].EndDate)
	}
	if byID["end-empty"].EndDate != byID["end-empty"].StartDate.AddDays(1) || byID["end-empty"].Status != schedule.EventPublished {
		t.Fatalf("empty-end conversion = %#v", byID["end-empty"])
	}
	if byID["cancelled"].Status != schedule.EventCancelled {
		t.Fatalf("cancelled status = %q", byID["cancelled"].Status)
	}
	regs := map[string]schedule.Registration{}
	for _, r := range converted.Registrations {
		regs[r.ID] = r
	}
	if regs["pending"].PartyID != "alex@example.test" || regs["pending"].Label != "Alex Adult" || regs["pending"].Seats != 2 || regs["pending"].State != schedule.StateRequested || regs["pending"].Meta["notes"] != "note" {
		t.Fatalf("pending conversion = %#v", regs["pending"])
	}
	if regs["confirmed"].State != schedule.StateConfirmed || regs["confirmed"].Seats != 3 {
		t.Fatalf("confirmed conversion = %#v", regs["confirmed"])
	}
	if regs["waitlist"].State != schedule.StateWaitlisted || !regs["waitlist"].RequestedAt.Equal(now) {
		t.Fatalf("waitlist conversion = %#v", regs["waitlist"])
	}
	if regs["cancelled-reg"].State != schedule.StateCancelled {
		t.Fatalf("cancelled registration = %#v", regs["cancelled-reg"])
	}
	if byID["draft"].Category != "" || byID["draft"].Capacity != 0 {
		t.Fatalf("legacy color/registered value leaked into schedule event: %#v", byID["draft"])
	}

	bad := calendar.MemorySeed{Events: []calendar.Event{{ID: "bad", Title: "", Start: start, End: start.Add(time.Hour), Timezone: "UTC"}}}
	_, err = calendarmigrate.Convert(bad, "UTC", now)
	var validation *schedule.ValidationError
	if !errors.As(err, &validation) || validation.Fields["events[0].title"] == "" {
		t.Fatalf("invalid event error = %#v", err)
	}
}

func TestCalendarMigrateLoadTwice(t *testing.T) {
	store := schedule.NewMemoryStore()
	seed := calendarmigrate.Seed{Resources: []schedule.Resource{{ID: "room", Kind: "space", Label: "Room"}}, Events: []schedule.Event{{ID: "event", Title: "Migrated", Zone: "UTC", Status: schedule.EventPublished, Start: now.Add(time.Hour), End: now.Add(2 * time.Hour), Capacity: 1}}, Registrations: []schedule.Registration{
		{ID: "a", EventID: "event", PartyID: "a@example.test", Seats: 1, State: schedule.StateConfirmed, RequestedAt: now},
		{ID: "b", EventID: "event", PartyID: "b@example.test", Seats: 1, State: schedule.StateConfirmed, RequestedAt: now.Add(time.Second)},
	}}
	if err := calendarmigrate.Load(context.Background(), store, seed, "migration", now); err != nil {
		t.Fatal(err)
	}
	eventBefore, err := store.Event(context.Background(), "event")
	if err != nil {
		t.Fatal(err)
	}
	if err := calendarmigrate.Load(context.Background(), store, seed, "migration", now); err != nil {
		t.Fatal(err)
	}
	eventAfter, err := store.Event(context.Background(), "event")
	if err != nil {
		t.Fatal(err)
	}
	if eventAfter.Revision != eventBefore.Revision || eventAfter.RosterRevision != eventBefore.RosterRevision {
		t.Fatalf("repeated migration changed revisions: before=%d/%d after=%d/%d", eventBefore.Revision, eventBefore.RosterRevision, eventAfter.Revision, eventAfter.RosterRevision)
	}
	roster, err := store.Roster(context.Background(), "event")
	if err != nil {
		t.Fatal(err)
	}
	if len(roster.Registrations) != 2 {
		t.Fatalf("registrations = %d", len(roster.Registrations))
	}
	if !roster.Registrations[1].Override || roster.Registrations[1].Reason != "migrated" {
		t.Fatalf("over-capacity registration = %#v", roster.Registrations[1])
	}
	if _, err := store.Receipt(context.Background(), "migrate:roster:event"); err != nil {
		t.Fatal(err)
	}
	transitions := store.Transitions()
	if len(transitions) != 2 {
		t.Fatalf("transitions = %d, want exactly 2", len(transitions))
	}
}
