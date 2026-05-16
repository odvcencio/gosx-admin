package calendar

import (
	"fmt"
	"strings"
	"sync"
)

type MemorySeed struct {
	Resources     []Resource
	Events        []Event
	Registrations []Registration
}

type MemoryScheduleStore struct {
	mu            sync.RWMutex
	resources     []Resource
	events        []Event
	registrations []Registration
	nextEventID   int
	nextRegID     int
}

var _ ScheduleStore = (*MemoryScheduleStore)(nil)

func NewMemoryScheduleStore(seed MemorySeed) *MemoryScheduleStore {
	store := &MemoryScheduleStore{
		resources:     NormalizeResources(seed.Resources),
		events:        NormalizeEvents(seed.Events),
		registrations: NormalizeRegistrations(seed.Registrations),
	}
	store.nextEventID = len(store.events) + 1
	store.nextRegID = len(store.registrations) + 1
	return store
}

func (s *MemoryScheduleStore) Snapshot() MemorySeed {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return MemorySeed{
		Resources:     cloneResources(s.resources),
		Events:        NormalizeEvents(s.events),
		Registrations: NormalizeRegistrations(s.registrations),
	}
}

func (s *MemoryScheduleStore) ListEvents(filter Filter) []Event {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return FilterEvents(ApplyRegistrationCounts(s.events, s.registrations), filter)
}

func (s *MemoryScheduleStore) SaveEvent(event Event) (Event, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if strings.TrimSpace(event.ID) == "" {
		event.ID = s.nextID("event")
	}
	normalized, ok := NormalizeEvent(event)
	if !ok {
		return Event{}, fmt.Errorf("event title and start are required")
	}
	for index, existing := range s.events {
		if existing.ID == normalized.ID {
			s.events[index] = normalized
			return normalized, nil
		}
	}
	s.events = append(s.events, normalized)
	return normalized, nil
}

func (s *MemoryScheduleStore) ListResources(filter ResourceFilter) []Resource {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return FilterResources(s.resources, filter)
}

func (s *MemoryScheduleStore) SaveResource(resource Resource) (Resource, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	normalized, ok := NormalizeResource(resource)
	if !ok {
		return Resource{}, fmt.Errorf("resource id and label are required")
	}
	for index, existing := range s.resources {
		if existing.ID == normalized.ID && existing.Kind == normalized.Kind {
			s.resources[index] = normalized
			return normalized, nil
		}
	}
	s.resources = append(s.resources, normalized)
	return normalized, nil
}

func (s *MemoryScheduleStore) ListRegistrations(filter RegistrationFilter) []Registration {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return FilterRegistrations(s.registrations, filter)
}

func (s *MemoryScheduleStore) SaveRegistration(registration Registration) (Registration, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if strings.TrimSpace(registration.ID) == "" {
		registration.ID = s.nextID("registration")
	}
	normalized, ok := NormalizeRegistration(registration)
	if !ok {
		return Registration{}, fmt.Errorf("registration id and event id are required")
	}
	for index, existing := range s.registrations {
		if existing.ID == normalized.ID {
			s.registrations[index] = normalized
			return normalized, nil
		}
	}
	s.registrations = append(s.registrations, normalized)
	return normalized, nil
}

func (s *MemoryScheduleStore) nextID(kind string) string {
	switch kind {
	case "registration":
		id := fmt.Sprintf("reg_%d", s.nextRegID)
		s.nextRegID++
		return id
	default:
		id := fmt.Sprintf("event_%d", s.nextEventID)
		s.nextEventID++
		return id
	}
}

func cloneResources(resources []Resource) []Resource {
	out := make([]Resource, len(resources))
	copy(out, resources)
	return out
}
