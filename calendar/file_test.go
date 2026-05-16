package calendar

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestFileScheduleStoreContracts(t *testing.T) {
	var _ ScheduleStore = (*FileScheduleStore)(nil)
}

func TestOpenFileScheduleStoreMissingFileStartsEmptyAndPersistsMutations(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "schedule.json")
	store, err := OpenFileScheduleStore(path)
	if err != nil {
		t.Fatal(err)
	}
	if events := store.ListEvents(Filter{}); len(events) != 0 {
		t.Fatalf("expected missing file to open empty, got %#v", events)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("expected missing file not to be created until mutation, stat error: %v", err)
	}
	resource, err := store.SaveResource(Resource{ID: "forest", Kind: "program", Label: "Forest", Capacity: 8})
	if err != nil {
		t.Fatal(err)
	}
	event, err := store.SaveEvent(Event{
		Title:        "Forest morning",
		Start:        time.Date(2026, 6, 8, 9, 0, 0, 0, time.UTC),
		ResourceKind: resource.Kind,
		ResourceID:   resource.ID,
		Status:       StatusOpen,
		Capacity:     8,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.SaveRegistration(Registration{
		EventID:      event.ID,
		ResourceKind: resource.Kind,
		ResourceID:   resource.ID,
		Name:         "Ada",
		Quantity:     2,
		Status:       RegistrationConfirmed,
	}); err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenFileScheduleStore(path)
	if err != nil {
		t.Fatal(err)
	}
	events := reopened.ListEvents(Filter{ResourceID: "forest"})
	if len(events) != 1 || events[0].Title != "Forest morning" || events[0].Registered != 2 {
		t.Fatalf("expected persisted event with registration count, got %#v", events)
	}
	resources := reopened.ListResources(ResourceFilter{Kind: "program"})
	if len(resources) != 1 || resources[0].ID != "forest" {
		t.Fatalf("expected persisted resource, got %#v", resources)
	}
	registrations := reopened.ListRegistrations(RegistrationFilter{EventID: event.ID})
	if len(registrations) != 1 || registrations[0].Name != "Ada" {
		t.Fatalf("expected persisted registration, got %#v", registrations)
	}
}

func TestNewFileScheduleStoreReplacesSnapshot(t *testing.T) {
	path := filepath.Join(t.TempDir(), "schedule.json")
	start := time.Date(2026, 7, 1, 10, 0, 0, 0, time.UTC)
	if _, err := NewFileScheduleStore(path, MemorySeed{
		Resources: []Resource{{ID: "forest", Kind: "program", Label: "Forest"}},
		Events:    []Event{{ID: "forest-am", Title: "Forest AM", Start: start, ResourceKind: "program", ResourceID: "forest"}},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := NewFileScheduleStore(path, MemorySeed{
		Resources: []Resource{{ID: "garden", Kind: "program", Label: "Garden"}},
		Events:    []Event{{ID: "garden-am", Title: "Garden AM", Start: start, ResourceKind: "program", ResourceID: "garden"}},
	}); err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenFileScheduleStore(path)
	if err != nil {
		t.Fatal(err)
	}
	events := reopened.ListEvents(Filter{})
	if len(events) != 1 || events[0].ID != "garden-am" {
		t.Fatalf("expected replacement seed to win, got %#v", events)
	}
	resources := reopened.ListResources(ResourceFilter{})
	if len(resources) != 1 || resources[0].ID != "garden" {
		t.Fatalf("expected replacement resources, got %#v", resources)
	}
}

func TestFileScheduleStoreEmptyPathError(t *testing.T) {
	if _, err := OpenFileScheduleStore(""); err == nil {
		t.Fatal("expected open empty path error")
	}
	if _, err := NewFileScheduleStore("   ", MemorySeed{}); err == nil {
		t.Fatal("expected new empty path error")
	}
}

func TestFileScheduleStoreRegistrationCountsAppliedOnListEvents(t *testing.T) {
	path := filepath.Join(t.TempDir(), "schedule.json")
	store, err := NewFileScheduleStore(path, MemorySeed{
		Events: []Event{{
			ID:       "forest-am",
			Title:    "Forest AM",
			Start:    time.Date(2026, 8, 3, 9, 0, 0, 0, time.UTC),
			Status:   StatusOpen,
			Capacity: 3,
		}},
		Registrations: []Registration{
			{ID: "one", EventID: "forest-am", Quantity: 2, Status: RegistrationConfirmed},
			{ID: "two", EventID: "forest-am", Quantity: 1, Status: RegistrationPending},
			{ID: "three", EventID: "forest-am", Quantity: 10, Status: RegistrationCancelled},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	events := store.ListEvents(Filter{})
	if len(events) != 1 || events[0].Registered != 3 || events[0].Status != StatusFull {
		t.Fatalf("expected registration counts on listed events, got %#v", events)
	}
}
