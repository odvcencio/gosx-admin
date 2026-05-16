package calendar

import (
	"testing"
	"time"
)

func TestMemoryScheduleStoreContracts(t *testing.T) {
	var _ ScheduleStore = (*MemoryScheduleStore)(nil)
}

func TestMemoryScheduleStoreListsWithCapacity(t *testing.T) {
	start := time.Date(2026, 6, 8, 9, 0, 0, 0, time.UTC)
	store := NewMemoryScheduleStore(MemorySeed{
		Resources: []Resource{{ID: "forest", Kind: "program", Label: "Forest", Capacity: 3}},
		Events: []Event{{
			ID:           "forest-am",
			Title:        "Forest AM",
			Start:        start,
			ResourceKind: "program",
			ResourceID:   "forest",
			Status:       StatusOpen,
			Capacity:     3,
		}},
		Registrations: []Registration{
			{ID: "reg_1", EventID: "forest-am", ResourceKind: "program", ResourceID: "forest", Quantity: 2, Status: RegistrationConfirmed},
			{ID: "reg_2", EventID: "forest-am", ResourceKind: "program", ResourceID: "forest", Quantity: 1, Status: RegistrationPending},
		},
	})
	events := store.ListEvents(Filter{ResourceID: "forest"})
	if len(events) != 1 || events[0].Registered != 3 || events[0].Status != StatusFull {
		t.Fatalf("expected capacity to be applied: %#v", events)
	}
	resources := store.ListResources(ResourceFilter{Kind: "program"})
	if len(resources) != 1 || resources[0].ID != "forest" {
		t.Fatalf("unexpected resources: %#v", resources)
	}
}

func TestMemoryScheduleStoreSavesAndSnapshots(t *testing.T) {
	store := NewMemoryScheduleStore(MemorySeed{})
	resource, err := store.SaveResource(Resource{ID: "garden", Kind: "program", Label: "Garden"})
	if err != nil {
		t.Fatal(err)
	}
	event, err := store.SaveEvent(Event{Title: "Garden PM", Start: time.Date(2026, 6, 11, 13, 0, 0, 0, time.UTC), ResourceKind: resource.Kind, ResourceID: resource.ID, Capacity: 5})
	if err != nil {
		t.Fatal(err)
	}
	registration, err := store.SaveRegistration(Registration{EventID: event.ID, ResourceKind: resource.Kind, ResourceID: resource.ID, Quantity: 2, Status: RegistrationConfirmed})
	if err != nil {
		t.Fatal(err)
	}
	if event.ID != "event_1" || registration.ID != "reg_1" {
		t.Fatalf("unexpected generated IDs: %#v %#v", event, registration)
	}
	snapshot := store.Snapshot()
	snapshot.Events[0].Title = "Changed"
	events := store.ListEvents(Filter{})
	if events[0].Title == "Changed" {
		t.Fatalf("expected snapshot to be isolated")
	}
}

func TestMemoryScheduleStoreValidation(t *testing.T) {
	store := NewMemoryScheduleStore(MemorySeed{})
	if _, err := store.SaveResource(Resource{Label: "Missing ID"}); err == nil {
		t.Fatal("expected invalid resource error")
	}
	if _, err := store.SaveEvent(Event{Title: "Missing start"}); err == nil {
		t.Fatal("expected invalid event error")
	}
	if _, err := store.SaveRegistration(Registration{ID: "reg"}); err == nil {
		t.Fatal("expected invalid registration error")
	}
}
