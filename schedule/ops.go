// Every operation requires a non-empty Key and a non-zero At, and returns
// ErrInvalid without them. ExpireHolds is the exception: it needs no key.
// Before computing, an operation looks up the key. A receipt for the same Op
// and record returns the first result with Replayed true; a receipt for
// anything else returns ErrKeyReused. When the store answers ErrDuplicateKey
// (a concurrent first use), the operation repeats that lookup. Book, Admit,
// and OfferNext re-read the roster and retry up to 3 times when Apply returns
// ErrConflict; after that they return ErrConflict. Confirm and Cancel retry
// the same way, except that a stored registration revision different from
// Expected returns ErrConflict at once.
package schedule

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Result is a registration operation's outcome.
type Result struct {
	Registration Registration
	// Replayed reports that the key was already used and this is its first
	// result.
	Replayed bool
}

// BookRequest asks for seats at an event.
type BookRequest struct {
	EventID string
	// RegistrationID is the new record's ID. Empty generates 32 lower-case
	// hex characters.
	RegistrationID string
	PartyID        string
	Label          string
	// Seats defaults to 1.
	Seats int
	// Hold > 0 creates a hold that ends at At+Hold. Hold 0 confirms at once.
	Hold     time.Duration
	HoldKind string
	// Review records the request as StateRequested, which uses no seat,
	// for staff to Admit later.
	Review bool
	// Override, when not empty, admits over capacity and is recorded as the
	// reason.
	Override string
	Key      string
	Actor    string
	At       time.Time
}

// Book creates a registration. With AvailabilityAt closed it returns
// ErrClosed. With Review it records StateRequested. Otherwise, when the event
// is unlimited or Remaining >= Seats, or Override is set, it records
// StateHeld (Hold > 0) or StateConfirmed; when full it records
// StateWaitlisted if the event has a waitlist, else returns ErrFull.
// RequestedAt is At.
func Book(ctx context.Context, s Store, req BookRequest) (Result, error) {
	if err := validOperation(req.Key, req.At); err != nil {
		return Result{}, err
	}
	if req.Hold < 0 || req.Seats < 0 {
		return Result{}, validation("seats", "Seats and hold length cannot be negative.")
	}
	if replay, ok, err := registrationReplay(ctx, s, req.Key, OpBook, req.EventID, ""); err != nil || ok {
		return replay, err
	}
	if req.RegistrationID == "" {
		id, err := randomID()
		if err != nil {
			return Result{}, err
		}
		req.RegistrationID = id
	}
	seats := req.Seats
	if seats == 0 {
		seats = 1
	}
	for attempt := 0; attempt < 3; attempt++ {
		roster, err := s.Roster(ctx, req.EventID)
		if err != nil {
			return Result{}, err
		}
		occupancy := Occupy(roster.Event, roster.Registrations, req.At)
		availability := AvailabilityAt(roster.Event, occupancy, req.At)
		if availability == AvailabilityClosed {
			return Result{}, ErrClosed
		}
		state := StateConfirmed
		if req.Review {
			state = StateRequested
		} else if occupancy.Unlimited || occupancy.Remaining >= seats || req.Override != "" {
			if req.Hold > 0 {
				state = StateHeld
			}
		} else if roster.Event.Waitlist {
			state = StateWaitlisted
		} else {
			return Result{}, ErrFull
		}
		r := Registration{ID: req.RegistrationID, EventID: req.EventID, PartyID: req.PartyID, Label: req.Label,
			Seats: seats, State: state, RequestedAt: req.At, Created: req.At, Updated: req.At, Key: req.Key}
		if state == StateHeld {
			r.HoldKind, r.HoldUntil = req.HoldKind, req.At.Add(req.Hold)
		}
		if req.Override != "" && !req.Review {
			r.Override, r.Reason = true, req.Override
		}
		tr := transitionFor(r, "", req.Actor, req.At, req.Key)
		tr.Reason = r.Reason
		receipt := Receipt{Key: req.Key, Op: OpBook, EventID: req.EventID, RegistrationID: r.ID, Revision: 1, At: req.At}
		updated, err := s.Apply(ctx, Change{EventID: req.EventID, EventRevision: roster.Event.Revision, RosterRevision: roster.Event.RosterRevision, Put: []Registration{r}, Transitions: []Transition{tr}, Receipt: receipt})
		if err == nil {
			return resultFromRoster(updated, r.ID), nil
		}
		if errors.Is(err, ErrDuplicateKey) {
			return duplicateRegistration(ctx, s, req.Key, OpBook, req.EventID, "", err)
		}
		if errors.Is(err, ErrConflict) {
			continue
		}
		return Result{}, err
	}
	return Result{}, ErrConflict
}

// AdmitRequest moves a requested or waitlisted registration into a seat.
type AdmitRequest struct {
	RegistrationID string
	Expected       Revision
	Hold           time.Duration
	HoldKind       string
	Override       string
	Key            string
	Actor          string
	At             time.Time
}

// Admit changes a requested or waitlisted registration to held (Hold > 0) or
// confirmed when it fits, or when Override is set. It returns ErrFull when it
// does not fit and ErrTransition for any other state.
func Admit(ctx context.Context, s Store, req AdmitRequest) (Result, error) {
	if err := validOperation(req.Key, req.At); err != nil {
		return Result{}, err
	}
	if req.Hold < 0 {
		return Result{}, validation("hold", "Hold length cannot be negative.")
	}
	if replay, ok, err := registrationReplay(ctx, s, req.Key, OpAdmit, "", req.RegistrationID); err != nil || ok {
		return replay, err
	}
	for attempt := 0; attempt < 3; attempt++ {
		current, err := s.Registration(ctx, req.RegistrationID)
		if err != nil {
			return Result{}, err
		}
		if current.Revision != req.Expected {
			return Result{}, ErrConflict
		}
		if current.State != StateRequested && current.State != StateWaitlisted {
			return Result{}, ErrTransition
		}
		roster, err := s.Roster(ctx, current.EventID)
		if err != nil {
			return Result{}, err
		}
		current, ok := findRegistration(roster.Registrations, req.RegistrationID)
		if !ok {
			return Result{}, ErrNotFound
		}
		if current.Revision != req.Expected {
			return Result{}, ErrConflict
		}
		occupancy := Occupy(roster.Event, roster.Registrations, req.At)
		if !occupancy.Unlimited && occupancy.Remaining < current.Seats && req.Override == "" {
			return Result{}, ErrFull
		}
		before := current.State
		current.State = StateConfirmed
		if req.Hold > 0 {
			current.State, current.HoldKind, current.HoldUntil = StateHeld, req.HoldKind, req.At.Add(req.Hold)
		} else {
			current.HoldKind, current.HoldUntil = "", time.Time{}
		}
		current.Override = req.Override != ""
		current.Reason = req.Override
		current.Updated = req.At
		tr := transitionFor(current, before, req.Actor, req.At, req.Key)
		receipt := Receipt{Key: req.Key, Op: OpAdmit, EventID: current.EventID, RegistrationID: current.ID, Revision: current.Revision + 1, At: req.At}
		updated, err := s.Apply(ctx, Change{EventID: current.EventID, EventRevision: roster.Event.Revision, RosterRevision: roster.Event.RosterRevision, Put: []Registration{current}, Transitions: []Transition{tr}, Receipt: receipt})
		if err == nil {
			return resultFromRoster(updated, current.ID), nil
		}
		if errors.Is(err, ErrDuplicateKey) {
			return duplicateRegistration(ctx, s, req.Key, OpAdmit, "", req.RegistrationID, err)
		}
		if errors.Is(err, ErrConflict) {
			continue
		}
		return Result{}, err
	}
	return Result{}, ErrConflict
}

// ConfirmRequest confirms a hold.
type ConfirmRequest struct {
	RegistrationID string
	Expected       Revision
	Key            string
	Actor          string
	At             time.Time
}

// Confirm changes a held registration to confirmed. It returns
// ErrHoldExpired when At is at or after HoldUntil, and ErrTransition for any
// state but held.
func Confirm(ctx context.Context, s Store, req ConfirmRequest) (Result, error) {
	if err := validOperation(req.Key, req.At); err != nil {
		return Result{}, err
	}
	if replay, ok, err := registrationReplay(ctx, s, req.Key, OpConfirm, "", req.RegistrationID); err != nil || ok {
		return replay, err
	}
	for attempt := 0; attempt < 3; attempt++ {
		roster, err := rosterForRegistration(ctx, s, req.RegistrationID)
		if err != nil {
			return Result{}, err
		}
		current, ok := findRegistration(roster.Registrations, req.RegistrationID)
		if !ok {
			return Result{}, ErrNotFound
		}
		if current.Revision != req.Expected {
			return Result{}, ErrConflict
		}
		if current.State != StateHeld {
			return Result{}, ErrTransition
		}
		if !req.At.Before(current.HoldUntil) {
			return Result{}, ErrHoldExpired
		}
		before := current.State
		current.State, current.HoldKind, current.HoldUntil = StateConfirmed, "", time.Time{}
		current.Updated = req.At
		tr := transitionFor(current, before, req.Actor, req.At, req.Key)
		receipt := Receipt{Key: req.Key, Op: OpConfirm, EventID: current.EventID, RegistrationID: current.ID, Revision: current.Revision + 1, At: req.At}
		updated, err := s.Apply(ctx, Change{EventID: current.EventID, EventRevision: roster.Event.Revision, RosterRevision: roster.Event.RosterRevision, Put: []Registration{current}, Transitions: []Transition{tr}, Receipt: receipt})
		if err == nil {
			return resultFromRoster(updated, current.ID), nil
		}
		if errors.Is(err, ErrDuplicateKey) {
			return duplicateRegistration(ctx, s, req.Key, OpConfirm, "", req.RegistrationID, err)
		}
		if errors.Is(err, ErrConflict) {
			continue
		}
		return Result{}, err
	}
	return Result{}, ErrConflict
}

// CancelRequest ends a registration.
type CancelRequest struct {
	RegistrationID string
	Expected       Revision
	Reason         string
	Key            string
	Actor          string
	At             time.Time
}

// Cancel changes a requested, held, confirmed, or waitlisted registration to
// cancelled. It returns ErrTransition for cancelled or expired ones; a
// replay of the same key returns the first result. Cancel does not offer
// the freed seat; call OfferNext.
func Cancel(ctx context.Context, s Store, req CancelRequest) (Result, error) {
	if err := validOperation(req.Key, req.At); err != nil {
		return Result{}, err
	}
	if replay, ok, err := registrationReplay(ctx, s, req.Key, OpCancel, "", req.RegistrationID); err != nil || ok {
		return replay, err
	}
	for attempt := 0; attempt < 3; attempt++ {
		roster, err := rosterForRegistration(ctx, s, req.RegistrationID)
		if err != nil {
			return Result{}, err
		}
		current, ok := findRegistration(roster.Registrations, req.RegistrationID)
		if !ok {
			return Result{}, ErrNotFound
		}
		if current.Revision != req.Expected {
			return Result{}, ErrConflict
		}
		if !current.State.Active() {
			return Result{}, ErrTransition
		}
		before := current.State
		current.State, current.HoldKind, current.HoldUntil = StateCancelled, "", time.Time{}
		current.Reason, current.Updated = req.Reason, req.At
		tr := transitionFor(current, before, req.Actor, req.At, req.Key)
		receipt := Receipt{Key: req.Key, Op: OpCancel, EventID: current.EventID, RegistrationID: current.ID, Revision: current.Revision + 1, At: req.At}
		updated, err := s.Apply(ctx, Change{EventID: current.EventID, EventRevision: roster.Event.Revision, RosterRevision: roster.Event.RosterRevision, Put: []Registration{current}, Transitions: []Transition{tr}, Receipt: receipt})
		if err == nil {
			return resultFromRoster(updated, current.ID), nil
		}
		if errors.Is(err, ErrDuplicateKey) {
			return duplicateRegistration(ctx, s, req.Key, OpCancel, "", req.RegistrationID, err)
		}
		if errors.Is(err, ErrConflict) {
			continue
		}
		return Result{}, err
	}
	return Result{}, ErrConflict
}

// ExpireRequest ends lapsed holds for one event.
type ExpireRequest struct {
	EventID string
	Actor   string
	At      time.Time
}

// ExpireHolds changes every held registration of the event whose HoldUntil
// is at or before At to expired, in one Change with no receipt, and returns
// them. A second pass at the same At finds none and writes nothing, so
// repeated passes create one transition per hold.
func ExpireHolds(ctx context.Context, s Store, req ExpireRequest) ([]Registration, error) {
	if req.At.IsZero() {
		return nil, validation("at", "Choose an expiry time.")
	}
	for attempt := 0; attempt < 3; attempt++ {
		roster, err := s.Roster(ctx, req.EventID)
		if err != nil {
			return nil, err
		}
		puts := make([]Registration, 0)
		transitions := make([]Transition, 0)
		for _, r := range roster.Registrations {
			if r.State != StateHeld || r.HoldUntil.After(req.At) {
				continue
			}
			before := r.State
			r.State, r.HoldKind, r.HoldUntil, r.Updated = StateExpired, "", time.Time{}, req.At
			tr := transitionFor(r, before, req.Actor, req.At, "")
			puts, transitions = append(puts, r), append(transitions, tr)
		}
		if len(puts) == 0 {
			return nil, nil
		}
		updated, err := s.Apply(ctx, Change{EventID: req.EventID, EventRevision: roster.Event.Revision, RosterRevision: roster.Event.RosterRevision, Put: puts, Transitions: transitions})
		if err == nil {
			out := make([]Registration, 0, len(puts))
			for _, r := range puts {
				if got, ok := findRegistration(updated.Registrations, r.ID); ok {
					out = append(out, got)
				}
			}
			return out, nil
		}
		if errors.Is(err, ErrConflict) {
			continue
		}
		return nil, err
	}
	return nil, ErrConflict
}

// OfferRequest offers a freed seat to the waitlist.
type OfferRequest struct {
	EventID string
	// Hold is the offer's length; it must be > 0. Offer holds end at
	// At+Hold as an absolute instant, so 72 hours is 72 hours across a
	// clock change.
	Hold     time.Duration
	HoldKind string
	// Eligible reports whether a waitlisted registration may still get an
	// offer. Nil treats every registration as eligible.
	Eligible func(Registration) bool
	Key      string
	Actor    string
	At       time.Time
}

// OfferNext takes the waitlist in WaitlistOrder, skips registrations that
// Eligible rejects or whose Seats exceed Remaining, and changes the first
// match to held until At+Hold. It returns false with no write when none
// qualifies or the event is closed or full.
func OfferNext(ctx context.Context, s Store, req OfferRequest) (Result, bool, error) {
	if err := validOperation(req.Key, req.At); err != nil {
		return Result{}, false, err
	}
	if req.Hold <= 0 {
		return Result{}, false, validation("hold", "Offer hold length must be positive.")
	}
	if replay, ok, err := registrationReplay(ctx, s, req.Key, OpOffer, req.EventID, ""); err != nil || ok {
		return replay, ok, err
	}
	for attempt := 0; attempt < 3; attempt++ {
		roster, err := s.Roster(ctx, req.EventID)
		if err != nil {
			return Result{}, false, err
		}
		o := Occupy(roster.Event, roster.Registrations, req.At)
		availability := AvailabilityAt(roster.Event, o, req.At)
		if availability == AvailabilityClosed || o.Full {
			return Result{}, false, nil
		}
		var chosen Registration
		found := false
		for _, r := range WaitlistOrder(roster.Registrations) {
			if req.Eligible != nil && !req.Eligible(r) {
				continue
			}
			if r.Seats <= 0 || (!o.Unlimited && r.Seats > o.Remaining) {
				continue
			}
			chosen, found = r, true
			break
		}
		if !found {
			return Result{}, false, nil
		}
		before := chosen.State
		chosen.State, chosen.HoldKind, chosen.HoldUntil = StateHeld, req.HoldKind, req.At.Add(req.Hold)
		chosen.Updated = req.At
		tr := transitionFor(chosen, before, req.Actor, req.At, req.Key)
		receipt := Receipt{Key: req.Key, Op: OpOffer, EventID: req.EventID, RegistrationID: chosen.ID, Revision: chosen.Revision + 1, At: req.At}
		updated, err := s.Apply(ctx, Change{EventID: req.EventID, EventRevision: roster.Event.Revision, RosterRevision: roster.Event.RosterRevision, Put: []Registration{chosen}, Transitions: []Transition{tr}, Receipt: receipt})
		if err == nil {
			return resultFromRoster(updated, chosen.ID), true, nil
		}
		if errors.Is(err, ErrDuplicateKey) {
			result, err := duplicateRegistration(ctx, s, req.Key, OpOffer, req.EventID, "", err)
			return result, err == nil, err
		}
		if errors.Is(err, ErrConflict) {
			continue
		}
		return Result{}, false, err
	}
	return Result{}, false, ErrConflict
}

// EventResult is SaveEvent's outcome.
type EventResult struct {
	Event    Event
	Replayed bool
}

// SaveEventRequest creates or updates an event.
type SaveEventRequest struct {
	Event Event
	// Expected is 0 to create (an empty Event.ID generates one), or the
	// revision the editor saw.
	Expected Revision
	// Override, when not empty, allows Capacity below the seats in use and
	// is recorded as the reason.
	Override string
	Key      string
	Actor    string
	Reason   string
	At       time.Time
}

// SaveEvent normalizes the event and writes it with PutEvent. On create,
// Sequence is 0 and PublishedAt is At when the status is published. On
// update it returns ErrConflict when the stored revision is not Expected,
// and ErrTransition for published -> draft. When the stored event is
// published or cancelled and MaterialChange is true, Sequence becomes the
// stored Sequence + 1 and PublishedAt becomes At; a first publication keeps
// Sequence 0 and sets PublishedAt.
//
// When Capacity goes down (from unlimited to a limit, or to a smaller
// limit), SaveEvent reads the roster. If the seats in use at At exceed the
// new Capacity and Override is empty, it returns a *ValidationError on
// "capacity". Otherwise it writes with ExpectedRoster set to the roster's
// RosterRevision. When that write fails only because the roster changed
// (the stored event revision still equals Expected), it repeats the check
// and the write, up to 3 times, then returns ErrConflict.
func SaveEvent(ctx context.Context, s Store, req SaveEventRequest) (EventResult, error) {
	if err := validOperation(req.Key, req.At); err != nil {
		return EventResult{}, err
	}
	req.Event.ID = strings.TrimSpace(req.Event.ID)
	if replay, ok, err := eventReplay(ctx, s, req.Key, req.Event.ID); err != nil || ok {
		return replay, err
	}
	if req.Event.ID == "" && req.Expected == 0 {
		id, err := randomID()
		if err != nil {
			return EventResult{}, err
		}
		req.Event.ID = id
	}
	normalized, err := NormalizeEvent(req.Event)
	if err != nil {
		return EventResult{}, err
	}
	for attempt := 0; attempt < 3; attempt++ {
		var previous *Event
		var roster Roster
		expectedRoster := Revision(0)
		capacityDown := false
		if req.Expected != 0 {
			old, err := s.Event(ctx, req.Event.ID)
			if err != nil {
				return EventResult{}, err
			}
			if old.Revision != req.Expected {
				return EventResult{}, ErrConflict
			}
			previous = &old
			if old.Status == EventPublished && normalized.Status == EventDraft {
				return EventResult{}, ErrTransition
			}
			capacityDown = old.Capacity == 0 && normalized.Capacity > 0 || (old.Capacity > 0 && normalized.Capacity < old.Capacity)
			if capacityDown {
				roster, err = s.Roster(ctx, req.Event.ID)
				if err != nil {
					return EventResult{}, err
				}
				expectedRoster = roster.Event.RosterRevision
			}
		}
		next := normalized
		if previous == nil {
			next.Sequence = 0
			if next.Status == EventPublished {
				next.PublishedAt = req.At
			} else {
				next.PublishedAt = time.Time{}
			}
			next.Created = req.At
		} else {
			old := *previous
			next.Created = old.Created
			next.Sequence, next.PublishedAt = old.Sequence, old.PublishedAt
			if old.Status == EventDraft && next.Status == EventPublished {
				next.Sequence, next.PublishedAt = 0, req.At
			} else if (old.Status == EventPublished || old.Status == EventCancelled) && MaterialChange(old, next) {
				next.Sequence, next.PublishedAt = old.Sequence+1, req.At
			}
		}
		next.Updated = req.At
		if capacityDown && Occupy(roster.Event, roster.Registrations, req.At).Used > next.Capacity && req.Override == "" {
			return EventResult{}, validation("capacity", "Capacity cannot be lower than seats in use.")
		}
		write := EventWrite{Event: next, Expected: req.Expected, ExpectedRoster: expectedRoster, Actor: req.Actor, Reason: req.Reason, Previous: previous,
			Receipt: Receipt{Key: req.Key, Op: OpSaveEvent, EventID: next.ID, Revision: req.Expected + 1, At: req.At}}
		if req.Expected == 0 {
			write.Receipt.Revision = 1
		}
		if req.Override != "" && capacityDown {
			write.Reason = req.Override
		}
		stored, err := s.PutEvent(ctx, write)
		if err == nil {
			return EventResult{Event: stored}, nil
		}
		if errors.Is(err, ErrDuplicateKey) {
			return duplicateEvent(ctx, s, req.Key, req.Event.ID, err)
		}
		if errors.Is(err, ErrConflict) && capacityDown {
			current, readErr := s.Event(ctx, req.Event.ID)
			if readErr != nil {
				return EventResult{}, readErr
			}
			if current.Revision != req.Expected {
				return EventResult{}, ErrConflict
			}
			if current.RosterRevision != expectedRoster {
				continue
			}
		}
		return EventResult{}, err
	}
	return EventResult{}, ErrConflict
}

// ResourceResult is SaveResource's outcome.
type ResourceResult struct {
	Resource Resource
	Replayed bool
}

// SaveResourceRequest creates or updates a resource.
type SaveResourceRequest struct {
	Resource Resource
	Expected Revision
	Key      string
	Actor    string
	Reason   string
	At       time.Time
}

// SaveResource normalizes and writes a resource with PutResource.
func SaveResource(ctx context.Context, s Store, req SaveResourceRequest) (ResourceResult, error) {
	if err := validOperation(req.Key, req.At); err != nil {
		return ResourceResult{}, err
	}
	if replay, ok, err := resourceReplay(ctx, s, req.Key, req.Resource.ID); err != nil || ok {
		return replay, err
	}
	next, err := NormalizeResource(req.Resource)
	if err != nil {
		return ResourceResult{}, err
	}
	var previous *Resource
	if req.Expected != 0 {
		old, err := s.Resource(ctx, next.ID)
		if err != nil {
			return ResourceResult{}, err
		}
		if old.Revision != req.Expected {
			return ResourceResult{}, ErrConflict
		}
		previous = &old
		next.Created = old.Created
	} else {
		next.Created = req.At
	}
	next.Updated = req.At
	write := ResourceWrite{Resource: next, Expected: req.Expected, Actor: req.Actor, Reason: req.Reason, Previous: previous,
		Receipt: Receipt{Key: req.Key, Op: OpSaveResource, ResourceID: next.ID, Revision: req.Expected + 1, At: req.At}}
	if req.Expected == 0 {
		write.Receipt.Revision = 1
	}
	stored, err := s.PutResource(ctx, write)
	if err == nil {
		return ResourceResult{Resource: stored}, nil
	}
	if errors.Is(err, ErrDuplicateKey) {
		return duplicateResource(ctx, s, req.Key, req.Resource.ID, err)
	}
	return ResourceResult{}, err
}

func validOperation(key string, at time.Time) error {
	if strings.TrimSpace(key) == "" {
		return validation("key", "Enter an idempotency key.")
	}
	if at.IsZero() {
		return validation("at", "Choose an operation time.")
	}
	return nil
}

func validation(field, message string) error {
	return &ValidationError{Fields: map[string]string{field: message}}
}

func randomID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("schedule: generate ID: %w", err)
	}
	return hex.EncodeToString(b[:]), nil
}

func transitionFor(r Registration, from RegistrationState, actor string, at time.Time, key string) Transition {
	return Transition{RegistrationID: r.ID, EventID: r.EventID, From: from, To: r.State, At: at, Actor: actor, Reason: r.Reason, Key: key, Override: r.Override}
}

func resultFromRoster(roster Roster, id string) Result {
	r, _ := findRegistration(roster.Registrations, id)
	return Result{Registration: r}
}

func findRegistration(regs []Registration, id string) (Registration, bool) {
	for _, r := range regs {
		if r.ID == id {
			return r, true
		}
	}
	return Registration{}, false
}

func rosterForRegistration(ctx context.Context, s Store, id string) (Roster, error) {
	r, err := s.Registration(ctx, id)
	if err != nil {
		return Roster{}, err
	}
	return s.Roster(ctx, r.EventID)
}

func registrationReplay(ctx context.Context, s Store, key string, op Op, eventID, registrationID string) (Result, bool, error) {
	receipt, err := s.Receipt(ctx, key)
	if errors.Is(err, ErrNotFound) {
		return Result{}, false, nil
	}
	if err != nil {
		return Result{}, false, err
	}
	if receipt.Op != op || (eventID != "" && receipt.EventID != eventID) || (registrationID != "" && receipt.RegistrationID != registrationID) {
		return Result{}, false, ErrKeyReused
	}
	r, err := s.Registration(ctx, receipt.RegistrationID)
	if err != nil {
		return Result{}, false, err
	}
	return Result{Registration: r, Replayed: true}, true, nil
}

func duplicateRegistration(ctx context.Context, s Store, key string, op Op, eventID, registrationID string, original error) (Result, error) {
	r, ok, err := registrationReplay(ctx, s, key, op, eventID, registrationID)
	if err != nil {
		return Result{}, err
	}
	if !ok {
		return Result{}, original
	}
	return r, nil
}

func eventReplay(ctx context.Context, s Store, key, eventID string) (EventResult, bool, error) {
	receipt, err := s.Receipt(ctx, key)
	if errors.Is(err, ErrNotFound) {
		return EventResult{}, false, nil
	}
	if err != nil {
		return EventResult{}, false, err
	}
	if receipt.Op != OpSaveEvent || (eventID != "" && receipt.EventID != eventID) {
		return EventResult{}, false, ErrKeyReused
	}
	e, err := s.Event(ctx, receipt.EventID)
	if err != nil {
		return EventResult{}, false, err
	}
	return EventResult{Event: e, Replayed: true}, true, nil
}

func duplicateEvent(ctx context.Context, s Store, key, eventID string, original error) (EventResult, error) {
	r, ok, err := eventReplay(ctx, s, key, eventID)
	if err != nil {
		return EventResult{}, err
	}
	if !ok {
		return EventResult{}, original
	}
	return r, nil
}

func resourceReplay(ctx context.Context, s Store, key, resourceID string) (ResourceResult, bool, error) {
	receipt, err := s.Receipt(ctx, key)
	if errors.Is(err, ErrNotFound) {
		return ResourceResult{}, false, nil
	}
	if err != nil {
		return ResourceResult{}, false, err
	}
	if receipt.Op != OpSaveResource || (resourceID != "" && receipt.ResourceID != resourceID) {
		return ResourceResult{}, false, ErrKeyReused
	}
	r, err := s.Resource(ctx, receipt.ResourceID)
	if err != nil {
		return ResourceResult{}, false, err
	}
	return ResourceResult{Resource: r, Replayed: true}, true, nil
}

func duplicateResource(ctx context.Context, s Store, key, resourceID string, original error) (ResourceResult, error) {
	r, ok, err := resourceReplay(ctx, s, key, resourceID)
	if err != nil {
		return ResourceResult{}, err
	}
	if !ok {
		return ResourceResult{}, original
	}
	return r, nil
}
