package calendar

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

type RegistrationStatus string

const (
	RegistrationPending   RegistrationStatus = "pending"
	RegistrationConfirmed RegistrationStatus = "confirmed"
	RegistrationWaitlist  RegistrationStatus = "waitlist"
	RegistrationCancelled RegistrationStatus = "cancelled"
)

type Resource struct {
	ID          string
	Kind        string
	Label       string
	Description string
	Color       string
	Capacity    int
	Href        string
	Archived    bool
}

type ResourceFilter struct {
	Kind     string
	Query    string
	Archived *bool
}

type Registration struct {
	ID           string
	EventID      string
	ResourceKind string
	ResourceID   string
	Name         string
	Email        string
	Quantity     int
	Status       RegistrationStatus
	Notes        string
	Created      time.Time
	Updated      time.Time
}

type RegistrationFilter struct {
	EventID      string
	ResourceKind string
	ResourceID   string
	Statuses     []RegistrationStatus
}

type ScheduleStore interface {
	Store
	ListResources(ResourceFilter) []Resource
	ListRegistrations(RegistrationFilter) []Registration
}

type Availability struct {
	Capacity    int
	Registered  int
	Remaining   int
	HasCapacity bool
	Full        bool
	Label       string
}

func NormalizeResource(resource Resource) (Resource, bool) {
	resource.ID = strings.TrimSpace(resource.ID)
	resource.Kind = strings.TrimSpace(resource.Kind)
	resource.Label = strings.TrimSpace(resource.Label)
	resource.Description = strings.TrimSpace(resource.Description)
	resource.Color = strings.TrimSpace(resource.Color)
	resource.Href = strings.TrimSpace(resource.Href)
	if resource.ID == "" || resource.Label == "" {
		return Resource{}, false
	}
	if resource.Kind == "" {
		resource.Kind = "resource"
	}
	if resource.Capacity < 0 {
		resource.Capacity = 0
	}
	return resource, true
}

func NormalizeResources(resources []Resource) []Resource {
	out := make([]Resource, 0, len(resources))
	for _, resource := range resources {
		if normalized, ok := NormalizeResource(resource); ok {
			out = append(out, normalized)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Kind == out[j].Kind {
			return out[i].Label < out[j].Label
		}
		return out[i].Kind < out[j].Kind
	})
	return out
}

func FilterResources(resources []Resource, filter ResourceFilter) []Resource {
	kind := strings.TrimSpace(filter.Kind)
	query := strings.ToLower(strings.TrimSpace(filter.Query))
	out := []Resource{}
	for _, resource := range NormalizeResources(resources) {
		if kind != "" && resource.Kind != kind {
			continue
		}
		if filter.Archived != nil && resource.Archived != *filter.Archived {
			continue
		}
		if query != "" && !strings.Contains(strings.ToLower(resource.Label+" "+resource.Description), query) {
			continue
		}
		out = append(out, resource)
	}
	return out
}

func NormalizeRegistration(registration Registration) (Registration, bool) {
	registration.ID = strings.TrimSpace(registration.ID)
	registration.EventID = strings.TrimSpace(registration.EventID)
	registration.ResourceKind = strings.TrimSpace(registration.ResourceKind)
	registration.ResourceID = strings.TrimSpace(registration.ResourceID)
	registration.Name = strings.TrimSpace(registration.Name)
	registration.Email = strings.TrimSpace(registration.Email)
	registration.Notes = strings.TrimSpace(registration.Notes)
	if registration.ID == "" || registration.EventID == "" {
		return Registration{}, false
	}
	if registration.Quantity <= 0 {
		registration.Quantity = 1
	}
	if registration.Status == "" {
		registration.Status = RegistrationPending
	}
	return registration, true
}

func NormalizeRegistrations(registrations []Registration) []Registration {
	out := make([]Registration, 0, len(registrations))
	for _, registration := range registrations {
		if normalized, ok := NormalizeRegistration(registration); ok {
			out = append(out, normalized)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].EventID == out[j].EventID {
			if out[i].Created.Equal(out[j].Created) {
				return out[i].Name < out[j].Name
			}
			return out[i].Created.Before(out[j].Created)
		}
		return out[i].EventID < out[j].EventID
	})
	return out
}

func FilterRegistrations(registrations []Registration, filter RegistrationFilter) []Registration {
	eventID := strings.TrimSpace(filter.EventID)
	resourceKind := strings.TrimSpace(filter.ResourceKind)
	resourceID := strings.TrimSpace(filter.ResourceID)
	statuses := map[RegistrationStatus]bool{}
	for _, status := range filter.Statuses {
		if status != "" {
			statuses[status] = true
		}
	}
	out := []Registration{}
	for _, registration := range NormalizeRegistrations(registrations) {
		if eventID != "" && registration.EventID != eventID {
			continue
		}
		if resourceKind != "" && registration.ResourceKind != resourceKind {
			continue
		}
		if resourceID != "" && registration.ResourceID != resourceID {
			continue
		}
		if len(statuses) > 0 && !statuses[registration.Status] {
			continue
		}
		out = append(out, registration)
	}
	return out
}

func ApplyRegistrationCounts(events []Event, registrations []Registration) []Event {
	counts := registrationCounts(registrations)
	out := NormalizeEvents(events)
	for i := range out {
		event := &out[i]
		event.Registered = counts[event.ID]
		if event.Capacity > 0 && event.Registered >= event.Capacity && event.Status != StatusCancelled && event.Status != StatusDraft {
			event.Status = StatusFull
		}
	}
	return out
}

func EventAvailability(event Event) Availability {
	event, ok := NormalizeEvent(event)
	if !ok {
		return Availability{}
	}
	availability := Availability{
		Capacity:    event.Capacity,
		Registered:  event.Registered,
		HasCapacity: event.Capacity > 0,
	}
	if !availability.HasCapacity {
		availability.Label = "Open"
		return availability
	}
	availability.Remaining = event.Capacity - event.Registered
	if availability.Remaining < 0 {
		availability.Remaining = 0
	}
	availability.Full = availability.Remaining == 0
	availability.Label = fmt.Sprintf("%d/%d", event.Registered, event.Capacity)
	if availability.Full {
		availability.Label += " full"
	}
	return availability
}

func CountableRegistration(status RegistrationStatus) bool {
	return status == RegistrationPending || status == RegistrationConfirmed
}

func registrationCounts(registrations []Registration) map[string]int {
	counts := map[string]int{}
	for _, registration := range NormalizeRegistrations(registrations) {
		if CountableRegistration(registration.Status) {
			counts[registration.EventID] += registration.Quantity
		}
	}
	return counts
}
