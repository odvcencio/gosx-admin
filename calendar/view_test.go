package calendar

import (
	"testing"
	"time"
)

func TestResourceMaps(t *testing.T) {
	resources := []Resource{
		{ID: "studio", Kind: "space", Label: "Studio", Capacity: -4},
		{ID: "garden", Kind: "program", Label: "Garden", Capacity: 8},
		{ID: "forest", Kind: "program", Label: "Forest", Capacity: 12, Archived: true, Href: "/admin/resources/forest"},
		{Label: "Missing ID"},
	}

	got := ResourceMaps(resources)
	if len(got) != 3 {
		t.Fatalf("expected invalid resources to be skipped, got %#v", got)
	}
	if got[0]["id"] != "forest" || got[0]["capacityLabel"] != "Capacity 12" || got[0]["statusLabel"] != "Archived" || got[0]["statusClass"] != "calendar-resource--archived" {
		t.Fatalf("unexpected forest resource map: %#v", got[0])
	}
	if got[2]["id"] != "studio" || got[2]["capacity"].(int) != 0 || got[2]["capacityLabel"] != "Open capacity" || got[2]["statusClass"] != "calendar-resource--active" {
		t.Fatalf("unexpected studio resource map: %#v", got[2])
	}
}

func TestRegistrationViews(t *testing.T) {
	now := time.Date(2026, 9, 1, 8, 0, 0, 0, time.UTC)
	registrations := []Registration{
		{ID: "wait", EventID: "forest-am", Name: "Wait List", Status: RegistrationWaitlist, Created: now.Add(time.Hour)},
		{ID: "confirmed", EventID: "forest-am", Name: "Confirmed", Status: RegistrationConfirmed, Quantity: 2, Created: now},
		{ID: "missing-event"},
	}

	got := RegistrationViews(registrations)
	if len(got) != 2 {
		t.Fatalf("expected invalid registrations to be skipped, got %#v", got)
	}
	if got[0].ID != "confirmed" || got[0].StatusLabel != "Confirmed" || got[0].StatusClass != "calendar-registration--confirmed" || !got[0].Countable {
		t.Fatalf("unexpected confirmed registration view: %#v", got[0])
	}
	if got[1].ID != "wait" || got[1].StatusLabel != "Waitlist" || got[1].Countable {
		t.Fatalf("unexpected waitlist registration view: %#v", got[1])
	}
}

func TestEventMaps(t *testing.T) {
	start := time.Date(2026, 9, 3, 9, 0, 0, 0, time.UTC)
	events := []Event{
		{
			ID:          "forest-am",
			Title:       "Forest AM",
			Description: "Outdoor program",
			Start:       start,
			End:         start.Add(150 * time.Minute),
			Location:    "Oak grove",
			Status:      StatusOpen,
			Color:       "#527a4b",
			Capacity:    10,
			Registered:  10,
			Href:        "/admin/calendar/events/forest-am",
		},
		{ID: "bad", Start: start},
	}

	got := EventMaps(events)
	if len(got) != 1 {
		t.Fatalf("expected invalid events to be skipped, got %#v", got)
	}
	view := got[0]
	if view["id"] != "forest-am" || view["timeLabel"] != "9:00 AM-11:30 AM" || view["capacityLabel"] != "10/10" {
		t.Fatalf("unexpected basic event map fields: %#v", view)
	}
	if view["availabilityLabel"] != "10/10 full" || view["remaining"].(int) != 0 || view["full"] != true || view["hasCapacity"] != true {
		t.Fatalf("unexpected event availability fields: %#v", view)
	}
	if view["statusLabel"] != "Open" || view["statusClass"] != "calendar-event--open" {
		t.Fatalf("unexpected event status fields: %#v", view)
	}
}

func TestLabelize(t *testing.T) {
	if got := labelize("schedule-tour"); got != "Schedule Tour" {
		t.Fatalf("unexpected hyphen label: %q", got)
	}
	if got := labelize("field_trip"); got != "Field Trip" {
		t.Fatalf("unexpected underscore label: %q", got)
	}
}
