package schedule

import (
	"context"
	"sort"
	"sync"
)

// MemoryStore is the reference Store. It keeps everything in memory behind
// one mutex, copies records in and out, and records applied Transitions. Use
// it in tests and small single-process sites.
type MemoryStore struct {
	mu            sync.Mutex
	events        map[string]Event
	resources     map[string]Resource
	registrations map[string]Registration
	receipts      map[string]Receipt
	transitions   []Transition
}

var _ Store = (*MemoryStore)(nil)

// NewMemoryStore returns an empty store.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{events: map[string]Event{}, resources: map[string]Resource{}, registrations: map[string]Registration{}, receipts: map[string]Receipt{}}
}

// Transitions returns every transition Apply has recorded, in order.
func (m *MemoryStore) Transitions() []Transition {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]Transition(nil), m.transitions...)
}

func (m *MemoryStore) begin(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	m.mu.Lock()
	if err := ctx.Err(); err != nil {
		m.mu.Unlock()
		return err
	}
	return nil
}

// Event implements EventStore.
func (m *MemoryStore) Event(ctx context.Context, id string) (Event, error) {
	if err := m.begin(ctx); err != nil {
		return Event{}, err
	}
	defer m.mu.Unlock()
	e, ok := m.events[id]
	if !ok {
		return Event{}, ErrNotFound
	}
	return cloneEvent(e), nil
}

// Events implements EventStore.
func (m *MemoryStore) Events(ctx context.Context, q EventQuery) ([]Event, error) {
	if err := m.begin(ctx); err != nil {
		return nil, err
	}
	defer m.mu.Unlock()
	out := make([]Event, 0, len(m.events))
	for _, event := range m.events {
		if !q.From.IsZero() && !event.End.After(q.From) {
			continue
		}
		if !q.To.IsZero() && !event.Start.Before(q.To) {
			continue
		}
		if q.ResourceID != "" && event.ResourceID != q.ResourceID {
			continue
		}
		if !contains(q.Categories, event.Category) || !contains(q.Visibilities, event.Visibility) || !containsStatus(q.Statuses, event.Status) {
			continue
		}
		out = append(out, cloneEvent(event))
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Start.Equal(out[j].Start) {
			return out[i].ID < out[j].ID
		}
		return out[i].Start.Before(out[j].Start)
	})
	if q.Limit > 0 && len(out) > q.Limit {
		out = out[:q.Limit]
	}
	return out, nil
}

// PutEvent implements EventStore.
func (m *MemoryStore) PutEvent(ctx context.Context, w EventWrite) (Event, error) {
	if err := m.begin(ctx); err != nil {
		return Event{}, err
	}
	defer m.mu.Unlock()
	if w.Receipt.Key != "" {
		if _, ok := m.receipts[w.Receipt.Key]; ok {
			return Event{}, ErrDuplicateKey
		}
	}
	e, exists := m.events[w.Event.ID]
	if w.Expected == 0 {
		if exists {
			return Event{}, ErrExists
		}
		w.Event.Revision, w.Event.RosterRevision = 1, 1
	} else {
		if !exists {
			return Event{}, ErrNotFound
		}
		if e.Revision != w.Expected || (w.ExpectedRoster != 0 && e.RosterRevision != w.ExpectedRoster) {
			return Event{}, ErrConflict
		}
		w.Event.Revision, w.Event.RosterRevision = w.Expected+1, e.RosterRevision
	}
	m.events[w.Event.ID] = cloneEvent(w.Event)
	if w.Receipt.Key != "" {
		m.receipts[w.Receipt.Key] = w.Receipt
	}
	return cloneEvent(w.Event), nil
}

// Resource implements ResourceStore.
func (m *MemoryStore) Resource(ctx context.Context, id string) (Resource, error) {
	if err := m.begin(ctx); err != nil {
		return Resource{}, err
	}
	defer m.mu.Unlock()
	r, ok := m.resources[id]
	if !ok {
		return Resource{}, ErrNotFound
	}
	return cloneResource(r), nil
}

// Resources implements ResourceStore.
func (m *MemoryStore) Resources(ctx context.Context, q ResourceQuery) ([]Resource, error) {
	if err := m.begin(ctx); err != nil {
		return nil, err
	}
	defer m.mu.Unlock()
	out := make([]Resource, 0, len(m.resources))
	for _, r := range m.resources {
		if q.Kind != "" && r.Kind != q.Kind {
			continue
		}
		if q.Archived != nil && r.Archived != *q.Archived {
			continue
		}
		out = append(out, cloneResource(r))
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Kind != out[j].Kind {
			return out[i].Kind < out[j].Kind
		}
		if out[i].Label != out[j].Label {
			return out[i].Label < out[j].Label
		}
		return out[i].ID < out[j].ID
	})
	return out, nil
}

// PutResource implements ResourceStore.
func (m *MemoryStore) PutResource(ctx context.Context, w ResourceWrite) (Resource, error) {
	if err := m.begin(ctx); err != nil {
		return Resource{}, err
	}
	defer m.mu.Unlock()
	if w.Receipt.Key != "" {
		if _, ok := m.receipts[w.Receipt.Key]; ok {
			return Resource{}, ErrDuplicateKey
		}
	}
	r, exists := m.resources[w.Resource.ID]
	if w.Expected == 0 {
		if exists {
			return Resource{}, ErrExists
		}
		w.Resource.Revision = 1
	} else {
		if !exists {
			return Resource{}, ErrNotFound
		}
		if r.Revision != w.Expected {
			return Resource{}, ErrConflict
		}
		w.Resource.Revision = w.Expected + 1
	}
	m.resources[w.Resource.ID] = cloneResource(w.Resource)
	if w.Receipt.Key != "" {
		m.receipts[w.Receipt.Key] = w.Receipt
	}
	return cloneResource(w.Resource), nil
}

// Roster implements RegistrationStore.
func (m *MemoryStore) Roster(ctx context.Context, eventID string) (Roster, error) {
	if err := m.begin(ctx); err != nil {
		return Roster{}, err
	}
	defer m.mu.Unlock()
	e, ok := m.events[eventID]
	if !ok {
		return Roster{}, ErrNotFound
	}
	regs := make([]Registration, 0)
	for _, r := range m.registrations {
		if r.EventID == eventID {
			regs = append(regs, cloneRegistration(r))
		}
	}
	sortRegistrations(regs)
	return Roster{Event: cloneEvent(e), Registrations: regs}, nil
}

// Registration implements RegistrationStore.
func (m *MemoryStore) Registration(ctx context.Context, id string) (Registration, error) {
	if err := m.begin(ctx); err != nil {
		return Registration{}, err
	}
	defer m.mu.Unlock()
	r, ok := m.registrations[id]
	if !ok {
		return Registration{}, ErrNotFound
	}
	return cloneRegistration(r), nil
}

// Registrations implements RegistrationStore.
func (m *MemoryStore) Registrations(ctx context.Context, q RegistrationQuery) ([]Registration, error) {
	if err := m.begin(ctx); err != nil {
		return nil, err
	}
	defer m.mu.Unlock()
	out := make([]Registration, 0, len(m.registrations))
	for _, r := range m.registrations {
		if q.EventID != "" && r.EventID != q.EventID {
			continue
		}
		if q.PartyID != "" && r.PartyID != q.PartyID {
			continue
		}
		if !containsState(q.States, r.State) {
			continue
		}
		out = append(out, cloneRegistration(r))
	}
	sortRegistrations(out)
	return out, nil
}

// Apply implements RegistrationStore.
func (m *MemoryStore) Apply(ctx context.Context, c Change) (Roster, error) {
	if err := m.begin(ctx); err != nil {
		return Roster{}, err
	}
	defer m.mu.Unlock()
	e, ok := m.events[c.EventID]
	if !ok {
		return Roster{}, ErrNotFound
	}
	if c.Receipt.Key != "" {
		if _, ok := m.receipts[c.Receipt.Key]; ok {
			return Roster{}, ErrDuplicateKey
		}
	}
	if e.Revision != c.EventRevision || e.RosterRevision != c.RosterRevision {
		return Roster{}, ErrConflict
	}
	seen := map[string]bool{}
	for _, r := range c.Put {
		if r.EventID != c.EventID {
			return Roster{}, ErrInvalid
		}
		if r.ID == "" || seen[r.ID] {
			return Roster{}, ErrExists
		}
		seen[r.ID] = true
		old, exists := m.registrations[r.ID]
		if r.Revision == 0 {
			if exists {
				return Roster{}, ErrExists
			}
		} else {
			if !exists || old.Revision != r.Revision {
				return Roster{}, ErrConflict
			}
		}
	}
	for _, r := range c.Put {
		if r.Revision == 0 {
			r.Revision = 1
		} else {
			r.Revision++
		}
		m.registrations[r.ID] = cloneRegistration(r)
	}
	e.RosterRevision++
	m.events[c.EventID] = e
	m.transitions = append(m.transitions, c.Transitions...)
	if c.Receipt.Key != "" {
		m.receipts[c.Receipt.Key] = c.Receipt
	}
	regs := make([]Registration, 0)
	for _, r := range m.registrations {
		if r.EventID == c.EventID {
			regs = append(regs, cloneRegistration(r))
		}
	}
	sortRegistrations(regs)
	return Roster{Event: cloneEvent(e), Registrations: regs}, nil
}

// Receipt implements ReceiptStore.
func (m *MemoryStore) Receipt(ctx context.Context, key string) (Receipt, error) {
	if err := m.begin(ctx); err != nil {
		return Receipt{}, err
	}
	defer m.mu.Unlock()
	r, ok := m.receipts[key]
	if !ok {
		return Receipt{}, ErrNotFound
	}
	return r, nil
}

func contains[T comparable](values []T, value T) bool {
	if len(values) == 0 {
		return true
	}
	for _, v := range values {
		if v == value {
			return true
		}
	}
	return false
}

func containsStatus(values []EventStatus, value EventStatus) bool { return contains(values, value) }
func containsState(values []RegistrationState, value RegistrationState) bool {
	return contains(values, value)
}

func sortRegistrations(regs []Registration) {
	sort.Slice(regs, func(i, j int) bool {
		if regs[i].RequestedAt.Equal(regs[j].RequestedAt) {
			return regs[i].ID < regs[j].ID
		}
		return regs[i].RequestedAt.Before(regs[j].RequestedAt)
	})
}

func cloneEvent(e Event) Event                      { e.Meta = cloneMap(e.Meta); return e }
func cloneResource(r Resource) Resource             { r.Meta = cloneMap(r.Meta); return r }
func cloneRegistration(r Registration) Registration { r.Meta = cloneMap(r.Meta); return r }
func cloneMap(m map[string]string) map[string]string {
	if m == nil {
		return nil
	}
	out := make(map[string]string, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}
