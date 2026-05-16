package calendar

import (
	"fmt"
	"strings"
	"time"
)

type ResourceView struct {
	ID            string
	Kind          string
	Label         string
	Description   string
	Color         string
	Capacity      int
	CapacityLabel string
	Href          string
	Archived      bool
	StatusLabel   string
	StatusClass   string
}

type RegistrationView struct {
	ID           string
	EventID      string
	ResourceKind string
	ResourceID   string
	Name         string
	Email        string
	Quantity     int
	Status       RegistrationStatus
	StatusLabel  string
	StatusClass  string
	Countable    bool
	Notes        string
	Created      time.Time
	Updated      time.Time
}

type EventView struct {
	ID                string
	Title             string
	Description       string
	Start             time.Time
	End               time.Time
	TimeLabel         string
	Location          string
	ResourceKind      string
	ResourceID        string
	Status            Status
	StatusLabel       string
	StatusClass       string
	Color             string
	Capacity          int
	Registered        int
	Remaining         int
	CapacityLabel     string
	AvailabilityLabel string
	Full              bool
	HasCapacity       bool
	Href              string
}

func ResourceViews(resources []Resource) []ResourceView {
	resources = NormalizeResources(resources)
	out := make([]ResourceView, 0, len(resources))
	for _, resource := range resources {
		out = append(out, ResourceView{
			ID:            resource.ID,
			Kind:          resource.Kind,
			Label:         resource.Label,
			Description:   resource.Description,
			Color:         resource.Color,
			Capacity:      resource.Capacity,
			CapacityLabel: resourceCapacityLabel(resource.Capacity),
			Href:          resource.Href,
			Archived:      resource.Archived,
			StatusLabel:   resourceStatusLabel(resource.Archived),
			StatusClass:   "calendar-resource--" + resourceStatusToken(resource.Archived),
		})
	}
	return out
}

func ResourceMaps(resources []Resource) []map[string]any {
	views := ResourceViews(resources)
	out := make([]map[string]any, 0, len(views))
	for _, view := range views {
		out = append(out, map[string]any{
			"id":            view.ID,
			"kind":          view.Kind,
			"label":         view.Label,
			"description":   view.Description,
			"color":         view.Color,
			"capacity":      view.Capacity,
			"capacityLabel": view.CapacityLabel,
			"href":          view.Href,
			"archived":      view.Archived,
			"statusLabel":   view.StatusLabel,
			"statusClass":   view.StatusClass,
		})
	}
	return out
}

func RegistrationViews(registrations []Registration) []RegistrationView {
	registrations = NormalizeRegistrations(registrations)
	out := make([]RegistrationView, 0, len(registrations))
	for _, registration := range registrations {
		out = append(out, RegistrationView{
			ID:           registration.ID,
			EventID:      registration.EventID,
			ResourceKind: registration.ResourceKind,
			ResourceID:   registration.ResourceID,
			Name:         registration.Name,
			Email:        registration.Email,
			Quantity:     registration.Quantity,
			Status:       registration.Status,
			StatusLabel:  labelize(string(registration.Status)),
			StatusClass:  "calendar-registration--" + statusClassToken(string(registration.Status)),
			Countable:    CountableRegistration(registration.Status),
			Notes:        registration.Notes,
			Created:      registration.Created,
			Updated:      registration.Updated,
		})
	}
	return out
}

func RegistrationMaps(registrations []Registration) []map[string]any {
	views := RegistrationViews(registrations)
	out := make([]map[string]any, 0, len(views))
	for _, view := range views {
		out = append(out, map[string]any{
			"id":           view.ID,
			"eventID":      view.EventID,
			"resourceKind": view.ResourceKind,
			"resourceID":   view.ResourceID,
			"name":         view.Name,
			"email":        view.Email,
			"quantity":     view.Quantity,
			"status":       string(view.Status),
			"statusLabel":  view.StatusLabel,
			"statusClass":  view.StatusClass,
			"countable":    view.Countable,
			"notes":        view.Notes,
			"created":      view.Created,
			"updated":      view.Updated,
		})
	}
	return out
}

func EventViews(events []Event) []EventView {
	events = NormalizeEvents(events)
	out := make([]EventView, 0, len(events))
	for _, event := range events {
		availability := EventAvailability(event)
		out = append(out, EventView{
			ID:                event.ID,
			Title:             event.Title,
			Description:       event.Description,
			Start:             event.Start,
			End:               event.End,
			TimeLabel:         EventTimeLabel(event),
			Location:          event.Location,
			ResourceKind:      event.ResourceKind,
			ResourceID:        event.ResourceID,
			Status:            event.Status,
			StatusLabel:       labelize(string(event.Status)),
			StatusClass:       "calendar-event--" + statusClassToken(string(event.Status)),
			Color:             event.Color,
			Capacity:          availability.Capacity,
			Registered:        availability.Registered,
			Remaining:         availability.Remaining,
			CapacityLabel:     CapacityLabel(event),
			AvailabilityLabel: availability.Label,
			Full:              availability.Full,
			HasCapacity:       availability.HasCapacity,
			Href:              event.Href,
		})
	}
	return out
}

func EventMaps(events []Event) []map[string]any {
	views := EventViews(events)
	out := make([]map[string]any, 0, len(views))
	for _, view := range views {
		out = append(out, map[string]any{
			"id":                view.ID,
			"title":             view.Title,
			"description":       view.Description,
			"start":             view.Start,
			"end":               view.End,
			"timeLabel":         view.TimeLabel,
			"location":          view.Location,
			"resourceKind":      view.ResourceKind,
			"resourceID":        view.ResourceID,
			"status":            string(view.Status),
			"statusLabel":       view.StatusLabel,
			"statusClass":       view.StatusClass,
			"color":             view.Color,
			"capacity":          view.Capacity,
			"registered":        view.Registered,
			"remaining":         view.Remaining,
			"capacityLabel":     view.CapacityLabel,
			"availabilityLabel": view.AvailabilityLabel,
			"full":              view.Full,
			"hasCapacity":       view.HasCapacity,
			"href":              view.Href,
		})
	}
	return out
}

func resourceCapacityLabel(capacity int) string {
	if capacity <= 0 {
		return "Open capacity"
	}
	return fmt.Sprintf("Capacity %d", capacity)
}

func resourceStatusLabel(archived bool) string {
	if archived {
		return "Archived"
	}
	return "Active"
}

func resourceStatusToken(archived bool) string {
	if archived {
		return "archived"
	}
	return "active"
}

func statusClassToken(value string) string {
	return statusToken(Status(value))
}

func labelize(value string) string {
	value = strings.TrimSpace(strings.ReplaceAll(value, "_", " "))
	value = strings.ReplaceAll(value, "-", " ")
	if value == "" {
		return ""
	}
	parts := strings.Fields(value)
	for i, part := range parts {
		if len(part) == 0 {
			continue
		}
		parts[i] = strings.ToUpper(part[:1]) + part[1:]
	}
	return strings.Join(parts, " ")
}
