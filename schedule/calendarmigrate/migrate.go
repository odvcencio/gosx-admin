// Package calendarmigrate converts calendar v0.2 seeds to schedule records.
package calendarmigrate

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"m31labs.dev/gosx-admin/calendar"
	"m31labs.dev/gosx-admin/schedule"
)

// Seed is converted data, ready to load.
type Seed struct {
	Resources     []schedule.Resource
	Events        []schedule.Event
	Registrations []schedule.Registration
}

// Convert maps a v0.2 calendar.MemorySeed to schedule records. zone is used
// for events whose Timezone is empty. now stamps RequestedAt for
// registrations without Created. It returns a *schedule.ValidationError
// keyed by "events[i].field" for records that fail NormalizeEvent.
func Convert(seed calendar.MemorySeed, zone string, now time.Time) (Seed, error) {
	out := Seed{Resources: make([]schedule.Resource, 0, len(seed.Resources)), Events: make([]schedule.Event, 0, len(seed.Events)), Registrations: make([]schedule.Registration, 0, len(seed.Registrations))}
	fields := make(map[string]string)
	for i, old := range seed.Resources {
		resource, err := schedule.NormalizeResource(schedule.Resource{ID: old.ID, Kind: old.Kind, Label: old.Label, Summary: old.Description, Capacity: old.Capacity, Archived: old.Archived})
		if err != nil {
			var validation *schedule.ValidationError
			if errors.As(err, &validation) {
				for key, value := range validation.Fields {
					fields[fmt.Sprintf("resources[%d].%s", i, key)] = value
				}
			}
			continue
		}
		out.Resources = append(out.Resources, resource)
	}
	for i, old := range seed.Events {
		zoneName := old.Timezone
		if zoneName == "" {
			zoneName = zone
		}
		status := schedule.EventDraft
		switch old.Status {
		case calendar.StatusScheduled, calendar.StatusOpen, calendar.StatusFull:
			status = schedule.EventPublished
		case calendar.StatusCancelled:
			status = schedule.EventCancelled
		case calendar.StatusDraft:
			status = schedule.EventDraft
		}
		event := schedule.Event{ID: old.ID, Title: old.Title, Summary: old.Description, Location: old.Location, Zone: zoneName, AllDay: old.AllDay, ResourceID: old.ResourceID,
			Status: status, Capacity: old.Capacity, Visibility: ""}
		if old.AllDay {
			loc, err := time.LoadLocation(zoneName)
			if err != nil {
				loc = time.UTC
			}
			event.StartDate = schedule.DateOf(old.Start, loc)
			if old.End.IsZero() || !old.End.After(old.Start) {
				event.EndDate = event.StartDate.AddDays(1)
			} else {
				event.EndDate = schedule.DateOf(old.End, loc).AddDays(1)
			}
		} else {
			event.Start, event.End = old.Start, old.End
			if !event.Start.IsZero() && (event.End.IsZero() || event.End.Equal(event.Start)) {
				event.End = event.Start.Add(time.Hour)
			}
		}
		normalized, err := schedule.NormalizeEvent(event)
		if err != nil {
			var validation *schedule.ValidationError
			if errors.As(err, &validation) {
				for key, value := range validation.Fields {
					fields[fmt.Sprintf("events[%d].%s", i, key)] = value
				}
			}
			continue
		}
		out.Events = append(out.Events, normalized)
	}
	for _, old := range seed.Registrations {
		state := schedule.StateRequested
		switch old.Status {
		case calendar.RegistrationConfirmed:
			state = schedule.StateConfirmed
		case calendar.RegistrationWaitlist:
			state = schedule.StateWaitlisted
		case calendar.RegistrationCancelled:
			state = schedule.StateCancelled
		case calendar.RegistrationPending:
			state = schedule.StateRequested
		}
		requestedAt := old.Created
		if requestedAt.IsZero() {
			requestedAt = now
		}
		meta := map[string]string(nil)
		if old.Notes != "" {
			meta = map[string]string{"notes": old.Notes}
		}
		out.Registrations = append(out.Registrations, schedule.Registration{ID: old.ID, EventID: old.EventID, PartyID: old.Email, Label: old.Name, Seats: old.Quantity,
			State: state, RequestedAt: requestedAt, Created: old.Created, Updated: old.Updated, Meta: meta})
	}
	if len(fields) != 0 {
		return Seed{}, &schedule.ValidationError{Fields: fields}
	}
	return out, nil
}

// Load writes seed into s. Resources and events go through SaveResource and
// SaveEvent with keys "migrate:" + record ID. Each event's registrations go
// in one Apply that inserts them with their converted states, bypassing the
// capacity rules, with Transition.Reason "migrated", Override set on seats
// beyond capacity, and the receipt key "migrate:roster:" + event ID. A
// repeated Load is a no-op.
func Load(ctx context.Context, s schedule.Store, seed Seed, actor string, at time.Time) error {
	if at.IsZero() {
		return &schedule.ValidationError{Fields: map[string]string{"at": "Choose a migration time."}}
	}
	for _, resource := range seed.Resources {
		_, err := schedule.SaveResource(ctx, s, schedule.SaveResourceRequest{Resource: resource, Key: "migrate:" + resource.ID, Actor: actor, At: at})
		if err != nil {
			return err
		}
	}
	for _, event := range seed.Events {
		result, err := schedule.SaveEvent(ctx, s, schedule.SaveEventRequest{Event: event, Key: "migrate:" + event.ID, Actor: actor, At: at})
		if err != nil {
			return err
		}
		event = result.Event
		key := "migrate:roster:" + event.ID
		if _, err := s.Receipt(ctx, key); err == nil {
			continue
		} else if !errors.Is(err, schedule.ErrNotFound) {
			return err
		}
		roster, err := s.Roster(ctx, event.ID)
		if err != nil {
			return err
		}
		regs := make([]schedule.Registration, 0)
		for _, r := range seed.Registrations {
			if r.EventID == event.ID {
				regs = append(regs, r)
			}
		}
		scheduleOrder(regs)
		used := 0
		puts := make([]schedule.Registration, 0, len(regs))
		transitions := make([]schedule.Transition, 0, len(regs))
		for _, r := range regs {
			r.Key = ""
			if r.Seats <= 0 {
				r.Seats = 1
			}
			override := false
			if r.Counts(at) {
				if event.Capacity > 0 && used+r.Seats > event.Capacity {
					override = true
					r.Override = true
					r.Reason = "migrated"
				}
				used += r.Seats
			}
			puts = append(puts, r)
			transitions = append(transitions, schedule.Transition{RegistrationID: r.ID, EventID: event.ID, To: r.State, At: at, Actor: actor, Reason: "migrated", Override: override})
		}
		_, err = s.Apply(ctx, schedule.Change{EventID: event.ID, EventRevision: roster.Event.Revision, RosterRevision: roster.Event.RosterRevision, Put: puts, Transitions: transitions,
			Receipt: schedule.Receipt{Key: key, EventID: event.ID, At: at}})
		if errors.Is(err, schedule.ErrDuplicateKey) {
			if _, receiptErr := s.Receipt(ctx, key); receiptErr == nil {
				continue
			}
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func scheduleOrder(regs []schedule.Registration) {
	sort.Slice(regs, func(i, j int) bool {
		if regs[i].RequestedAt.Equal(regs[j].RequestedAt) {
			return regs[i].ID < regs[j].ID
		}
		return regs[i].RequestedAt.Before(regs[j].RequestedAt)
	})
}
