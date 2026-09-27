package schedule

import "time"

// RegistrationState is where a registration is in its life.
type RegistrationState string

const (
	// StateRequested waits for review. It does not use a seat.
	StateRequested RegistrationState = "requested"
	// StateHeld uses seats until HoldUntil.
	StateHeld RegistrationState = "held"
	// StateConfirmed uses seats.
	StateConfirmed RegistrationState = "confirmed"
	// StateWaitlisted waits for a seat. It does not use a seat.
	StateWaitlisted RegistrationState = "waitlisted"
	// StateCancelled ended by request or by staff.
	StateCancelled RegistrationState = "cancelled"
	// StateExpired is a hold that ended without confirmation.
	StateExpired RegistrationState = "expired"
)

// Active reports whether s is requested, held, confirmed, or waitlisted.
func (s RegistrationState) Active() bool {
	return s == StateRequested || s == StateHeld || s == StateConfirmed || s == StateWaitlisted
}

// Registration is one request for seats at one event.
type Registration struct {
	ID      string
	EventID string
	// PartyID is the consumer's opaque ID for who registered.
	PartyID string
	// Label is display text for admin views.
	Label string
	// Seats is the number of seats; Book sets 0 to 1.
	Seats int
	State RegistrationState
	// HoldKind is a consumer label, for example "offer" or "payment".
	HoldKind string
	// HoldUntil ends a hold: the hold counts while at is before HoldUntil.
	HoldUntil time.Time
	// RequestedAt is when the request was first made. The waitlist orders by
	// RequestedAt, then ID.
	RequestedAt time.Time
	// Reason explains the last cancellation, expiry, or override.
	Reason string
	// Override records that this registration was admitted over capacity.
	Override bool
	// Key is the idempotency key of the request that created it.
	Key      string
	Revision Revision
	Created  time.Time
	Updated  time.Time
	Meta     map[string]string
}

// Counts reports whether r uses seats at instant at: confirmed, or held with
// at before HoldUntil.
func (r Registration) Counts(at time.Time) bool {
	return r.State == StateConfirmed || (r.State == StateHeld && !r.HoldUntil.IsZero() && at.Before(r.HoldUntil))
}

// Transition is one registration state change. Operations put transitions in
// the Change they apply; a store writes its audit or outbox rows for them in
// the same transaction.
type Transition struct {
	RegistrationID string
	EventID        string
	// From is "" for a new registration.
	From     RegistrationState
	To       RegistrationState
	At       time.Time
	Actor    string
	Reason   string
	Key      string
	Override bool
}

// Op names an operation in a Receipt.
type Op string

const (
	OpBook         Op = "book"
	OpAdmit        Op = "admit"
	OpConfirm      Op = "confirm"
	OpCancel       Op = "cancel"
	OpOffer        Op = "offer"
	OpSaveEvent    Op = "save-event"
	OpSaveResource Op = "save-resource"
)

// Receipt records the first result of an idempotent operation.
type Receipt struct {
	Key            string
	Op             Op
	EventID        string
	ResourceID     string
	RegistrationID string
	// Revision is the written record's revision after the operation.
	Revision Revision
	At       time.Time
}
