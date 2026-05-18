package calendar

import (
	"testing"
	"time"
)

func TestDefaultScheduleWidgetContractCoversStudioStatesAndControls(t *testing.T) {
	contract := DefaultScheduleWidgetContract(ScheduleWidgetOptions{
		Key:            "forest schedule",
		Label:          "Forest schedule",
		PublicHref:     "/schedule",
		AdminHref:      "/admin/schedule",
		RegisterAction: "schedule.register",
	})

	if contract.Key != "forest-schedule" || contract.Recipe.Key != "forest-schedule-widget" {
		t.Fatalf("unexpected normalized contract keys: %#v", contract)
	}
	wantStates := []WidgetState{
		WidgetStateOpen,
		WidgetStateFull,
		WidgetStateWaitlist,
		WidgetStateClosed,
		WidgetStateSelected,
		WidgetStateToday,
		WidgetStateStaffOnly,
		WidgetStateDraft,
	}
	if len(contract.States) != len(wantStates) {
		t.Fatalf("unexpected states: %#v", contract.States)
	}
	for index, state := range wantStates {
		if contract.States[index].Key != state || contract.States[index].ClassSuffix != string(state) {
			t.Fatalf("unexpected state at %d: %#v", index, contract.States[index])
		}
	}

	controls := map[string]WidgetControl{}
	for _, control := range contract.Recipe.Controls {
		controls[control.Key] = control
	}
	for _, key := range []string{"density", "day-cell-shape", "availability-emphasis", "mobile-mode", "staff-visibility"} {
		control, ok := controls[key]
		if !ok || !control.Required || control.Default == "" || len(control.Options) == 0 {
			t.Fatalf("expected required configured control %q, got %#v", key, control)
		}
	}
}

func TestScheduleWidgetReadinessTracksMissingRoutesAndActions(t *testing.T) {
	contract := DefaultScheduleWidgetContract(ScheduleWidgetOptions{})
	checks := ScheduleWidgetReadiness(contract)
	if len(checks) != 4 {
		t.Fatalf("unexpected checks: %#v", checks)
	}
	if checks[3].Key != "actions" || checks[3].Status != WidgetReadinessWatch {
		t.Fatalf("expected action check to watch missing app wiring: %#v", checks[3])
	}

	contract = DefaultScheduleWidgetContract(ScheduleWidgetOptions{
		PublicHref:     "/schedule",
		AdminHref:      "/admin/schedule",
		RegisterAction: "schedule.register",
	})
	checks = ScheduleWidgetReadiness(contract)
	for _, check := range checks {
		if check.Status != WidgetReadinessReady {
			t.Fatalf("expected fully wired contract to be ready, got %#v", checks)
		}
	}
}

func TestScheduleWidgetContractViewIncludesRecipeDataAndReadiness(t *testing.T) {
	view := ScheduleWidgetContractView(DefaultScheduleWidgetContract(ScheduleWidgetOptions{
		PublicHref:     "/schedule",
		AdminHref:      "/admin/schedule",
		RegisterAction: "schedule.register",
	}))
	if view["key"] != "schedule" || view["publicURL"] != "/schedule" || view["adminURL"] != "/admin/schedule" {
		t.Fatalf("unexpected contract view: %#v", view)
	}
	recipe := view["recipe"].(map[string]any)
	controls := recipe["controls"].([]map[string]any)
	if len(controls) != 5 || controls[0]["key"] != "density" {
		t.Fatalf("unexpected recipe view: %#v", recipe)
	}
	states := view["states"].([]map[string]any)
	if len(states) != 8 || states[2]["key"] != "waitlist" {
		t.Fatalf("unexpected states view: %#v", states)
	}
	readiness := view["readiness"].([]map[string]any)
	if readiness[3]["status"] != string(WidgetReadinessReady) {
		t.Fatalf("unexpected readiness view: %#v", readiness)
	}
}

func TestWidgetStatesForEvent(t *testing.T) {
	day := time.Date(2026, 9, 14, 9, 0, 0, 0, time.UTC)
	states := WidgetStatesForEvent(Event{
		Title:      "Forest AM",
		Start:      day,
		Capacity:   10,
		Registered: 10,
		Status:     StatusOpen,
	}, WidgetStateOptions{
		Today:     day,
		Selected:  day,
		Waitlist:  true,
		StaffOnly: true,
	})
	want := []WidgetState{WidgetStateFull, WidgetStateWaitlist, WidgetStateSelected, WidgetStateToday, WidgetStateStaffOnly}
	if len(states) != len(want) {
		t.Fatalf("unexpected states: %#v", states)
	}
	for index, state := range want {
		if states[index] != state {
			t.Fatalf("unexpected state order: %#v", states)
		}
	}

	closed := WidgetStatesForEvent(Event{Title: "Draft", Start: day, Status: StatusCancelled}, WidgetStateOptions{})
	if len(closed) != 1 || closed[0] != WidgetStateClosed {
		t.Fatalf("expected cancelled event to map to closed: %#v", closed)
	}
}

func TestNormalizeEmptyScheduleWidgetContractAddsDefaults(t *testing.T) {
	contract := NormalizeScheduleWidgetContract(ScheduleWidgetContract{})
	if contract.Key != "schedule" || len(contract.States) != 8 {
		t.Fatalf("expected empty contract to normalize with defaults: %#v", contract)
	}
}
