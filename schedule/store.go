package schedule

import (
	"context"
	"time"
)

// EventQuery selects events. Results are ordered by Start, then ID.
type EventQuery struct {
	// From and To select events that overlap [From, To): Start < To and
	// End > From. Zero means unbounded on that side.
	From time.Time
	To   time.Time
	// The filters below match any listed value; an empty list matches all.
	ResourceID   string
	Categories   []string
	Visibilities []string
	Statuses     []EventStatus
	// Limit caps the result; 0 means no cap.
	Limit int
}

// ResourceQuery selects resources, ordered by Kind, Label, then ID.
type ResourceQuery struct {
	Kind string
	// Archived filters on Resource.Archived when non-nil.
	Archived *bool
}

// RegistrationQuery selects registrations, ordered by RequestedAt, then ID.
type RegistrationQuery struct {
	EventID string
	PartyID string
	States  []RegistrationState
}

// Roster is one event with all its registrations, in RequestedAt, ID order.
type Roster struct {
	Event         Event
	Registrations []Registration
}

// EventWrite is one event create or update.
type EventWrite struct {
	// Event is the new state. The store ignores Revision and RosterRevision.
	Event Event
	// Expected is 0 to create, or the event revision the write was computed
	// from.
	Expected Revision
	// ExpectedRoster, when non-zero, also requires this RosterRevision.
	// SaveEvent sets it when capacity goes down.
	ExpectedRoster Revision
	// Receipt is stored with the write when Receipt.Key is not empty.
	Receipt Receipt
	// Previous is the state the write replaces; nil on create. The store may
	// use it for its audit row.
	Previous *Event
	Actor    string
	Reason   string
}

// ResourceWrite is one resource create or update.
type ResourceWrite struct {
	Resource Resource
	Expected Revision
	Receipt  Receipt
	Previous *Resource
	Actor    string
	Reason   string
}

// Change is an atomic set of registration writes for one event.
type Change struct {
	EventID string
	// EventRevision and RosterRevision are the event's revisions when the
	// change was computed.
	EventRevision  Revision
	RosterRevision Revision
	// Put holds registrations to insert (Revision 0) or replace (Revision =
	// the stored revision). Every EventID must equal Change.EventID.
	Put []Registration
	// Transitions describe the state changes in Put.
	Transitions []Transition
	// Receipt is stored with the change when Receipt.Key is not empty.
	Receipt Receipt
}

// EventStore persists events.
//
// PutEvent contract: with Expected 0 it creates the event (ErrExists when the
// ID is taken) with Revision 1 and RosterRevision 1. Otherwise it updates only
// when the stored Revision equals Expected and, if ExpectedRoster is non-zero,
// the stored RosterRevision equals ExpectedRoster (else ErrConflict); it sets
// Revision to Expected+1 and keeps RosterRevision. A missing event is
// ErrNotFound. When Receipt.Key is not empty and already stored, it returns
// ErrDuplicateKey. The event, receipt, and any audit rows commit together or
// not at all. It returns the stored event.
type EventStore interface {
	Event(ctx context.Context, id string) (Event, error)
	Events(ctx context.Context, q EventQuery) ([]Event, error)
	PutEvent(ctx context.Context, w EventWrite) (Event, error)
}

// ResourceStore persists resources. PutResource follows the PutEvent contract
// without the roster check.
type ResourceStore interface {
	Resource(ctx context.Context, id string) (Resource, error)
	Resources(ctx context.Context, q ResourceQuery) ([]Resource, error)
	PutResource(ctx context.Context, w ResourceWrite) (Resource, error)
}

// RegistrationStore persists registrations.
//
// Apply contract: it returns ErrNotFound when the event is missing;
// ErrConflict when the stored event Revision or RosterRevision differs from
// the Change, or when a replaced registration's stored Revision differs from
// its Put.Revision; ErrExists when an inserted ID is taken; ErrInvalid when a
// Put names another event; and ErrDuplicateKey when Receipt.Key is already
// stored. Otherwise it writes every Put (inserted Revision 1, replaced
// Revision+1), adds 1 to the event's RosterRevision, stores the receipt, and
// handles the Transitions, all in one transaction. It returns the new Roster.
type RegistrationStore interface {
	Roster(ctx context.Context, eventID string) (Roster, error)
	Registration(ctx context.Context, id string) (Registration, error)
	Registrations(ctx context.Context, q RegistrationQuery) ([]Registration, error)
	Apply(ctx context.Context, c Change) (Roster, error)
}

// ReceiptStore looks up idempotency receipts. Receipt returns ErrNotFound for
// an unknown key.
type ReceiptStore interface {
	Receipt(ctx context.Context, key string) (Receipt, error)
}

// Store is everything the operations need. Every method returns ctx.Err()
// when ctx is done before it starts, and is safe for concurrent use.
type Store interface {
	EventStore
	ResourceStore
	RegistrationStore
	ReceiptStore
}
