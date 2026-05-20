package calendar

import (
	"testing"
	"time"
)

func TestFilterResources(t *testing.T) {
	archived := false
	resources := []Resource{
		{ID: "forest", Kind: "program", Label: "Forest classroom", Description: "Outdoor days", Capacity: 12},
		{ID: "garden", Kind: "program", Label: "Garden atelier", Archived: true},
		{ID: "staff-room", Kind: "space", Label: "Staff room"},
		{Label: "Missing ID"},
	}
	got := FilterResources(resources, ResourceFilter{Kind: "program", Query: "forest", Archived: &archived})
	if len(got) != 1 || got[0].ID != "forest" || got[0].Capacity != 12 {
		t.Fatalf("unexpected resources: %#v", got)
	}
}

func TestNormalizeRegistration(t *testing.T) {
	registration, ok := NormalizeRegistration(Registration{
		ID:       " reg_1 ",
		EventID:  " session_1 ",
		Name:     " Ada ",
		Quantity: -2,
	})
	if !ok {
		t.Fatal("expected registration to normalize")
	}
	if registration.ID != "reg_1" || registration.EventID != "session_1" || registration.Quantity != 1 || registration.Status != RegistrationPending {
		t.Fatalf("unexpected registration: %#v", registration)
	}
	if _, ok := NormalizeRegistration(Registration{ID: "missing-event"}); ok {
		t.Fatal("expected missing event ID to be invalid")
	}
}

func TestFilterRegistrations(t *testing.T) {
	now := time.Date(2026, 8, 3, 10, 0, 0, 0, time.UTC)
	registrations := []Registration{
		{ID: "one", EventID: "forest-am", ResourceKind: "program", ResourceID: "forest", Status: RegistrationConfirmed, Quantity: 2, Created: now.Add(time.Hour)},
		{ID: "two", EventID: "forest-am", ResourceKind: "program", ResourceID: "forest", Status: RegistrationWaitlist, Created: now},
		{ID: "three", EventID: "garden-am", ResourceKind: "program", ResourceID: "garden", Status: RegistrationConfirmed, Created: now},
	}
	got := FilterRegistrations(registrations, RegistrationFilter{
		EventID:    "forest-am",
		Statuses:   []RegistrationStatus{RegistrationConfirmed},
		ResourceID: "forest",
	})
	if len(got) != 1 || got[0].ID != "one" {
		t.Fatalf("unexpected registrations: %#v", got)
	}
}

func TestApplyRegistrationCountsAndAvailability(t *testing.T) {
	start := time.Date(2026, 8, 3, 9, 0, 0, 0, time.UTC)
	events := []Event{
		{ID: "forest-am", Title: "Forest AM", Start: start, Capacity: 3, Status: StatusOpen},
		{ID: "garden-am", Title: "Garden AM", Start: start, Capacity: 6, Status: StatusOpen},
	}
	registrations := []Registration{
		{ID: "one", EventID: "forest-am", Quantity: 2, Status: RegistrationConfirmed},
		{ID: "two", EventID: "forest-am", Quantity: 1, Status: RegistrationPending},
		{ID: "three", EventID: "forest-am", Quantity: 5, Status: RegistrationWaitlist},
		{ID: "four", EventID: "garden-am", Quantity: 2, Status: RegistrationCancelled},
	}
	got := ApplyRegistrationCounts(events, registrations)
	if got[0].Registered != 3 || got[0].Status != StatusFull {
		t.Fatalf("expected full forest session, got %#v", got[0])
	}
	if got[1].Registered != 0 || got[1].Status != StatusOpen {
		t.Fatalf("expected cancelled registration not to count, got %#v", got[1])
	}
	availability := EventAvailability(got[0])
	if !availability.Full || availability.Remaining != 0 || availability.Label != "3/3 full" {
		t.Fatalf("unexpected availability: %#v", availability)
	}
	summary := SummarizeCapacity(got)
	if summary.FullEvents != 1 || summary.TotalCapacity != 9 || summary.Registered != 3 {
		t.Fatalf("unexpected capacity summary: %#v", summary)
	}
}

func TestEventAvailabilityWithoutCapacity(t *testing.T) {
	availability := EventAvailability(Event{Title: "Open house", Start: time.Now(), Registered: 99})
	if availability.HasCapacity || availability.Full || availability.Label != "Open" {
		t.Fatalf("unexpected open availability: %#v", availability)
	}
}
