// Package scheduletest checks that a Store follows the schedule contracts.
package scheduletest

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sync"
	"testing"
	"time"

	"m31labs.dev/gosx-admin/schedule"
)

// Harness is one fresh, empty store under test.
type Harness struct {
	Store schedule.Store
	// Transitions returns the transitions the store persisted, in order.
	// Nil skips the transition checks.
	Transitions func() []schedule.Transition
	// Reopen returns a new handle on the same data, for example a new
	// connection to the same file. Nil skips the reopen checks.
	Reopen func() schedule.Store
}

// Epoch is the fixed start instant for the suite's clock: 2027-03-01 12:00
// UTC, a Monday.
var Epoch = time.Date(2027, 3, 1, 12, 0, 0, 0, time.UTC)

// Run runs every conformance subtest with t.Run. open must return a fresh,
// empty store for each subtest. The subtests are listed in the design's test
// plan; each checks results again after Reopen when that is set.
func Run(t *testing.T, open func(t *testing.T) Harness) {
	t.Helper()
	if open == nil {
		t.Fatal("open is nil")
	}
	newHarness := func(t *testing.T) Harness {
		t.Helper()
		h := open(t)
		if h.Store == nil {
			t.Fatal("Harness.Store is nil")
		}
		return h
	}

	t.Run("EventCreateStartsAtRevisionOne", func(t *testing.T) {
		h := newHarness(t)
		e := putEvent(t, h.Store, event("event", 1))
		if e.Revision != 1 || e.RosterRevision != 1 {
			t.Fatalf("revisions = %d/%d, want 1/1", e.Revision, e.RosterRevision)
		}
		checkReopen(t, h, []string{e.ID}, nil, nil)
	})
	t.Run("EventUpdateNeedsExpectedRevision", func(t *testing.T) {
		h := newHarness(t)
		e := putEvent(t, h.Store, event("event", 1))
		changed := e
		changed.Title = "Changed"
		if _, err := h.Store.PutEvent(context.Background(), schedule.EventWrite{Event: changed, Expected: e.Revision}); err != nil {
			t.Fatal(err)
		}
		changed.Title = "Stale"
		if _, err := h.Store.PutEvent(context.Background(), schedule.EventWrite{Event: changed, Expected: e.Revision}); !errors.Is(err, schedule.ErrConflict) {
			t.Fatalf("PutEvent error = %v, want ErrConflict", err)
		}
		stored := getEvent(t, h.Store, e.ID)
		if stored.Title != "Changed" || stored.Revision != 2 {
			t.Fatalf("stored event = %#v", stored)
		}
		checkReopen(t, h, []string{e.ID}, nil, nil)
	})
	t.Run("EventCreateDuplicateIsExists", func(t *testing.T) {
		h := newHarness(t)
		e := event("event", 1)
		putEvent(t, h.Store, e)
		if _, err := h.Store.PutEvent(context.Background(), schedule.EventWrite{Event: e}); !errors.Is(err, schedule.ErrExists) {
			t.Fatalf("error = %v, want ErrExists", err)
		}
	})
	t.Run("EventMissingIsNotFound", func(t *testing.T) {
		h := newHarness(t)
		if _, err := h.Store.Event(context.Background(), "missing"); !errors.Is(err, schedule.ErrNotFound) {
			t.Fatalf("Event error = %v", err)
		}
		if _, err := h.Store.PutEvent(context.Background(), schedule.EventWrite{Event: event("missing", 1), Expected: 1}); !errors.Is(err, schedule.ErrNotFound) {
			t.Fatalf("PutEvent error = %v", err)
		}
	})
	t.Run("EventRoundTrip", func(t *testing.T) {
		h := newHarness(t)
		e := event("event", 1)
		e.ResourceID, e.Summary, e.Location, e.Category, e.Visibility = "room", "summary", "location", "category", "public"
		e.AllDay, e.StartDate, e.EndDate = true, schedule.Date{Year: 2027, Month: time.March, Day: 1}, schedule.Date{Year: 2027, Month: time.March, Day: 3}
		e.Capacity, e.Waitlist = 14, true
		e.BookingOpens, e.BookingCloses = Epoch.Add(-time.Hour), Epoch.Add(time.Hour)
		e.Sequence, e.PublishedAt, e.Created, e.Updated = 8, Epoch.Add(-time.Minute), Epoch.Add(-time.Hour), Epoch
		e.Meta = map[string]string{"one": "1", "two": "2"}
		stored := putEvent(t, h.Store, e)
		got := getEvent(t, h.Store, e.ID)
		stored.Revision, stored.RosterRevision = 1, 1
		if !equalEvent(got, stored) {
			t.Fatalf("round trip differs:\n got %#v\nwant %#v", got, stored)
		}
		checkReopen(t, h, []string{e.ID}, nil, nil)
	})
	t.Run("EventDuplicateKeyWritesNothing", func(t *testing.T) {
		h := newHarness(t)
		e := putEvent(t, h.Store, event("event", 1))
		w := schedule.EventWrite{Event: e, Expected: e.Revision, Receipt: schedule.Receipt{Key: "key", EventID: e.ID}}
		if _, err := h.Store.PutEvent(context.Background(), w); err != nil {
			t.Fatal(err)
		}
		w.Event.Title = "must not write"
		if _, err := h.Store.PutEvent(context.Background(), w); !errors.Is(err, schedule.ErrDuplicateKey) {
			t.Fatalf("error = %v", err)
		}
		if got := getEvent(t, h.Store, e.ID); got.Title != e.Title || got.Revision != 2 {
			t.Fatalf("event changed: %#v", got)
		}
		checkReopen(t, h, []string{e.ID}, nil, []string{"key"})
	})
	t.Run("EventsOverlapAndOrder", func(t *testing.T) {
		h := newHarness(t)
		from, to := Epoch, Epoch.Add(time.Hour)
		for _, e := range []schedule.Event{eventSpan("end-at-from", from.Add(-time.Hour), from), eventSpan("inside-b", from.Add(time.Minute), to), eventSpan("inside-a", from, to.Add(-time.Minute)), eventSpan("start-at-to", to, to.Add(time.Hour))} {
			putEvent(t, h.Store, e)
		}
		got, err := h.Store.Events(context.Background(), schedule.EventQuery{From: from, To: to})
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 2 || got[0].ID != "inside-a" || got[1].ID != "inside-b" {
			t.Fatalf("overlap/order = %v", ids(got))
		}
	})
	t.Run("EventsFilters", func(t *testing.T) {
		h := newHarness(t)
		one, two, three := event("one", 1), event("two", 2), event("three", 3)
		one.ResourceID, one.Category, one.Visibility, one.Status = "r1", "a", "public", schedule.EventPublished
		two.ResourceID, two.Category, two.Visibility, two.Status = "r1", "b", "shared", schedule.EventCancelled
		three.ResourceID, three.Category, three.Visibility, three.Status = "r2", "a", "public", schedule.EventDraft
		for _, e := range []schedule.Event{one, two, three} {
			putEvent(t, h.Store, e)
		}
		cases := []struct {
			name string
			q    schedule.EventQuery
			want []string
		}{
			{"resource", schedule.EventQuery{ResourceID: "r1"}, []string{"one", "two"}},
			{"category", schedule.EventQuery{Categories: []string{"a"}}, []string{"one", "three"}},
			{"visibility", schedule.EventQuery{Visibilities: []string{"shared"}}, []string{"two"}},
			{"status", schedule.EventQuery{Statuses: []schedule.EventStatus{schedule.EventDraft}}, []string{"three"}},
			{"combined and limit", schedule.EventQuery{ResourceID: "r1", Categories: []string{"a", "b"}, Visibilities: []string{"public", "shared"}, Statuses: []schedule.EventStatus{schedule.EventPublished, schedule.EventCancelled}, Limit: 1}, []string{"one"}},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				got, err := h.Store.Events(context.Background(), tc.q)
				if err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(ids(got), tc.want) {
					t.Fatalf("ids = %v, want %v", ids(got), tc.want)
				}
			})
		}
	})
	t.Run("ResourceLifecycle", func(t *testing.T) {
		h := newHarness(t)
		ctx := context.Background()
		resources := []schedule.Resource{{ID: "z", Kind: "room", Label: "Zulu"}, {ID: "b", Kind: "room", Label: "Alpha"}, {ID: "a", Kind: "room", Label: "Alpha"}, {ID: "x", Kind: "other", Label: "X", Archived: true}}
		for _, r := range resources {
			if _, err := h.Store.PutResource(ctx, schedule.ResourceWrite{Resource: r}); err != nil {
				t.Fatal(err)
			}
		}
		archived := false
		got, err := h.Store.Resources(ctx, schedule.ResourceQuery{Kind: "room", Archived: &archived})
		if err != nil {
			t.Fatal(err)
		}
		if got[0].ID != "a" || got[1].ID != "b" || got[2].ID != "z" {
			t.Fatalf("resource order = %#v", got)
		}
		changed := got[0]
		changed.Label = "Changed"
		updated, err := h.Store.PutResource(ctx, schedule.ResourceWrite{Resource: changed, Expected: got[0].Revision})
		if err != nil {
			t.Fatal(err)
		}
		if updated.Revision != 2 {
			t.Fatalf("revision = %d", updated.Revision)
		}
		if _, err := h.Store.PutResource(ctx, schedule.ResourceWrite{Resource: changed, Expected: got[0].Revision}); !errors.Is(err, schedule.ErrConflict) {
			t.Fatalf("stale update error = %v", err)
		}
	})
	t.Run("ApplyAdvancesRosterRevision", func(t *testing.T) {
		h := newHarness(t)
		e := putEvent(t, h.Store, event("event", 1))
		roster := applyInsert(t, h.Store, e, "r1", "key")
		if roster.Event.Revision != e.Revision || roster.Event.RosterRevision != e.RosterRevision+1 {
			t.Fatalf("revisions = %d/%d", roster.Event.Revision, roster.Event.RosterRevision)
		}
		checkReopen(t, h, []string{e.ID}, []string{"r1"}, []string{"key"})
	})
	t.Run("ApplyStaleRosterRevisionIsConflict", func(t *testing.T) {
		h := newHarness(t)
		e := putEvent(t, h.Store, event("event", 1))
		stale := e
		applyInsert(t, h.Store, e, "r1", "key1")
		_, err := h.Store.Apply(context.Background(), schedule.Change{EventID: e.ID, EventRevision: stale.Revision, RosterRevision: stale.RosterRevision, Put: []schedule.Registration{registration("r2", e.ID)}})
		if !errors.Is(err, schedule.ErrConflict) {
			t.Fatalf("Apply error = %v", err)
		}
	})
	t.Run("ApplyStaleEventRevisionIsConflict", func(t *testing.T) {
		h := newHarness(t)
		e := putEvent(t, h.Store, event("event", 1))
		changed := e
		changed.Title = "updated"
		updated, err := h.Store.PutEvent(context.Background(), schedule.EventWrite{Event: changed, Expected: e.Revision})
		if err != nil {
			t.Fatal(err)
		}
		_, err = h.Store.Apply(context.Background(), schedule.Change{EventID: e.ID, EventRevision: e.Revision, RosterRevision: updated.RosterRevision, Put: []schedule.Registration{registration("r1", e.ID)}})
		if !errors.Is(err, schedule.ErrConflict) {
			t.Fatalf("Apply error = %v", err)
		}
	})
	t.Run("ApplyStaleRegistrationRevisionIsConflict", func(t *testing.T) {
		h := newHarness(t)
		e := putEvent(t, h.Store, event("event", 1))
		roster := applyInsert(t, h.Store, e, "r1", "")
		r := roster.Registrations[0]
		updated, err := h.Store.Apply(context.Background(), schedule.Change{EventID: e.ID, EventRevision: roster.Event.Revision, RosterRevision: roster.Event.RosterRevision, Put: []schedule.Registration{r}})
		if err != nil {
			t.Fatal(err)
		}
		stale := r
		_, err = h.Store.Apply(context.Background(), schedule.Change{EventID: e.ID, EventRevision: updated.Event.Revision, RosterRevision: updated.Event.RosterRevision, Put: []schedule.Registration{stale}})
		if !errors.Is(err, schedule.ErrConflict) {
			t.Fatalf("Apply error = %v", err)
		}
	})
	t.Run("ApplyInsertExistingIsExists", func(t *testing.T) {
		h := newHarness(t)
		e := putEvent(t, h.Store, event("event", 1))
		roster := applyInsert(t, h.Store, e, "r1", "")
		_, err := h.Store.Apply(context.Background(), schedule.Change{EventID: e.ID, EventRevision: roster.Event.Revision, RosterRevision: roster.Event.RosterRevision, Put: []schedule.Registration{registration("r1", e.ID)}})
		if !errors.Is(err, schedule.ErrExists) {
			t.Fatalf("Apply error = %v", err)
		}
	})
	t.Run("ApplyOtherEventIsInvalid", func(t *testing.T) {
		h := newHarness(t)
		e := putEvent(t, h.Store, event("event", 1))
		_, err := h.Store.Apply(context.Background(), schedule.Change{EventID: e.ID, EventRevision: e.Revision, RosterRevision: e.RosterRevision, Put: []schedule.Registration{registration("r1", "other")}})
		if !errors.Is(err, schedule.ErrInvalid) {
			t.Fatalf("Apply error = %v", err)
		}
	})
	t.Run("ApplyIsAtomic", func(t *testing.T) {
		h := newHarness(t)
		e := putEvent(t, h.Store, event("event", 1))
		key := "atomic"
		_, err := h.Store.Apply(context.Background(), schedule.Change{EventID: e.ID, EventRevision: e.Revision, RosterRevision: e.RosterRevision, Put: []schedule.Registration{registration("r1", e.ID), registration("r2", "other")}, Receipt: schedule.Receipt{Key: key}})
		if !errors.Is(err, schedule.ErrInvalid) {
			t.Fatalf("Apply error = %v", err)
		}
		if _, err := h.Store.Registration(context.Background(), "r1"); !errors.Is(err, schedule.ErrNotFound) {
			t.Fatalf("partial registration write: %v", err)
		}
		if _, err := h.Store.Receipt(context.Background(), key); !errors.Is(err, schedule.ErrNotFound) {
			t.Fatalf("partial receipt write: %v", err)
		}
	})
	t.Run("ApplyDuplicateKeyWritesNothing", func(t *testing.T) {
		h := newHarness(t)
		e := putEvent(t, h.Store, event("event", 1))
		key := "duplicate"
		first := schedule.Change{EventID: e.ID, EventRevision: e.Revision, RosterRevision: e.RosterRevision, Put: []schedule.Registration{registration("r1", e.ID)}, Receipt: schedule.Receipt{Key: key}}
		if _, err := h.Store.Apply(context.Background(), first); err != nil {
			t.Fatal(err)
		}
		roster, _ := h.Store.Roster(context.Background(), e.ID)
		second := schedule.Change{EventID: e.ID, EventRevision: roster.Event.Revision, RosterRevision: roster.Event.RosterRevision, Put: []schedule.Registration{registration("r2", e.ID)}, Receipt: schedule.Receipt{Key: key}}
		if _, err := h.Store.Apply(context.Background(), second); !errors.Is(err, schedule.ErrDuplicateKey) {
			t.Fatalf("Apply error = %v", err)
		}
		if _, err := h.Store.Registration(context.Background(), "r2"); !errors.Is(err, schedule.ErrNotFound) {
			t.Fatalf("partial registration write: %v", err)
		}
	})
	t.Run("ApplyRecordsTransitions", func(t *testing.T) {
		h := newHarness(t)
		if h.Transitions == nil {
			t.Skip("Harness.Transitions is nil")
		}
		e := putEvent(t, h.Store, event("event", 1))
		r1, r2 := registration("r1", e.ID), registration("r2", e.ID)
		transitions := []schedule.Transition{{RegistrationID: r1.ID, EventID: e.ID, To: schedule.StateRequested}, {RegistrationID: r2.ID, EventID: e.ID, To: schedule.StateConfirmed}}
		_, err := h.Store.Apply(context.Background(), schedule.Change{EventID: e.ID, EventRevision: e.Revision, RosterRevision: e.RosterRevision, Put: []schedule.Registration{r1, r2}, Transitions: transitions})
		if err != nil {
			t.Fatal(err)
		}
		if got := h.Transitions(); !reflect.DeepEqual(got, transitions) {
			t.Fatalf("transitions = %#v", got)
		}
	})
	t.Run("ContextDoneReturnsError", func(t *testing.T) {
		h := newHarness(t)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		e := event("event", 1)
		r := schedule.Resource{ID: "resource", Label: "Room"}
		reg := registration("r", e.ID)
		checks := []struct {
			name string
			err  error
		}{
			{"Event", func() error { _, e := h.Store.Event(ctx, "x"); return e }()},
			{"Events", func() error { _, e := h.Store.Events(ctx, schedule.EventQuery{}); return e }()},
			{"PutEvent", func() error { _, e := h.Store.PutEvent(ctx, schedule.EventWrite{Event: e}); return e }()},
			{"Resource", func() error { _, e := h.Store.Resource(ctx, "x"); return e }()},
			{"Resources", func() error { _, e := h.Store.Resources(ctx, schedule.ResourceQuery{}); return e }()},
			{"PutResource", func() error { _, e := h.Store.PutResource(ctx, schedule.ResourceWrite{Resource: r}); return e }()},
			{"Roster", func() error { _, e := h.Store.Roster(ctx, "x"); return e }()},
			{"Registration", func() error { _, e := h.Store.Registration(ctx, "x"); return e }()},
			{"Registrations", func() error { _, e := h.Store.Registrations(ctx, schedule.RegistrationQuery{}); return e }()},
			{"Apply", func() error { _, e := h.Store.Apply(ctx, schedule.Change{EventID: e.ID}); return e }()},
			{"Receipt", func() error { _, e := h.Store.Receipt(ctx, "x"); return e }()},
		}
		for _, check := range checks {
			if !errors.Is(check.err, context.Canceled) {
				t.Errorf("%s error = %v, want context.Canceled", check.name, check.err)
			}
		}
		_ = reg
	})
	t.Run("BookCapacityOneRace", func(t *testing.T) {
		for _, waitlist := range []bool{true, false} {
			t.Run(fmt.Sprintf("waitlist-%t", waitlist), func(t *testing.T) {
				h := newHarness(t)
				e := event("event", 1)
				e.Capacity, e.Waitlist = 1, waitlist
				putEvent(t, h.Store, e)
				var mu sync.Mutex
				confirmed, waitlisted, full := 0, 0, 0
				var wg sync.WaitGroup
				for i := 0; i < 8; i++ {
					i := i
					wg.Add(1)
					go func() {
						defer wg.Done()
						req := schedule.BookRequest{EventID: e.ID, RegistrationID: fmt.Sprintf("r%d", i), PartyID: fmt.Sprintf("p%d", i), Seats: 1, Key: fmt.Sprintf("%032x", i+1), Actor: "test", At: Epoch}
						result, err := schedule.Book(context.Background(), h.Store, req)
						mu.Lock()
						defer mu.Unlock()
						if err != nil {
							if errors.Is(err, schedule.ErrFull) {
								full++
							} else {
								t.Errorf("Book: %v", err)
							}
							return
						}
						switch result.Registration.State {
						case schedule.StateConfirmed:
							confirmed++
						case schedule.StateWaitlisted:
							waitlisted++
						}
					}()
				}
				wg.Wait()
				if confirmed != 1 {
					t.Fatalf("confirmed = %d, want 1", confirmed)
				}
				if waitlist && waitlisted != 7 {
					t.Fatalf("waitlisted = %d, want 7", waitlisted)
				}
				if !waitlist && full != 7 {
					t.Fatalf("full = %d, want 7", full)
				}
			})
		}
	})
	t.Run("BookSameKeyRace", func(t *testing.T) {
		h := newHarness(t)
		e := event("event", 1)
		e.Capacity = 10
		putEvent(t, h.Store, e)
		var wg sync.WaitGroup
		ids := make(chan string, 8)
		errs := make(chan error, 8)
		for i := 0; i < 8; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				r, err := schedule.Book(context.Background(), h.Store, schedule.BookRequest{EventID: e.ID, PartyID: "party", Seats: 1, Key: "same-key", At: Epoch})
				if err != nil {
					errs <- err
					return
				}
				ids <- r.Registration.ID
			}()
		}
		wg.Wait()
		close(ids)
		close(errs)
		for err := range errs {
			t.Error(err)
		}
		var one string
		count := 0
		for id := range ids {
			if one == "" {
				one = id
			}
			if one != id {
				t.Fatalf("IDs = %q and %q", one, id)
			}
			count++
		}
		if count != 8 {
			t.Fatalf("results = %d, want 8", count)
		}
		roster, err := h.Store.Roster(context.Background(), e.ID)
		if err != nil {
			t.Fatal(err)
		}
		if len(roster.Registrations) != 1 {
			t.Fatalf("registrations = %d", len(roster.Registrations))
		}
	})
	t.Run("HoldCountsUntilExpiry", func(t *testing.T) {
		h := newHarness(t)
		e := event("event", 1)
		e.Capacity = 2
		putEvent(t, h.Store, e)
		_, err := schedule.Book(context.Background(), h.Store, schedule.BookRequest{EventID: e.ID, RegistrationID: "confirmed", Seats: 1, Key: "confirm-key", At: Epoch})
		if err != nil {
			t.Fatal(err)
		}
		_, err = schedule.Book(context.Background(), h.Store, schedule.BookRequest{EventID: e.ID, RegistrationID: "held", Seats: 1, Hold: 15 * time.Minute, HoldKind: "payment", Key: "hold-key", At: Epoch})
		if err != nil {
			t.Fatal(err)
		}
		roster, err := h.Store.Roster(context.Background(), e.ID)
		if err != nil {
			t.Fatal(err)
		}
		before := schedule.Occupy(roster.Event, roster.Registrations, Epoch.Add(14*time.Minute+59*time.Second))
		after := schedule.Occupy(roster.Event, roster.Registrations, Epoch.Add(15*time.Minute))
		if before.Used != 2 || before.Remaining != 0 || after.Used != 1 || after.Remaining != 1 {
			t.Fatalf("occupancy before=%#v after=%#v", before, after)
		}
	})
	t.Run("ExpireHoldsOnce", func(t *testing.T) {
		h := newHarness(t)
		e := event("event", 1)
		e.Capacity = 1
		putEvent(t, h.Store, e)
		_, err := schedule.Book(context.Background(), h.Store, schedule.BookRequest{EventID: e.ID, RegistrationID: "held", Hold: time.Minute, Key: "hold", At: Epoch})
		if err != nil {
			t.Fatal(err)
		}
		at := Epoch.Add(time.Minute)
		first, err := schedule.ExpireHolds(context.Background(), h.Store, schedule.ExpireRequest{EventID: e.ID, Actor: "worker", At: at})
		if err != nil {
			t.Fatal(err)
		}
		second, err := schedule.ExpireHolds(context.Background(), h.Store, schedule.ExpireRequest{EventID: e.ID, Actor: "worker", At: at})
		if err != nil {
			t.Fatal(err)
		}
		if len(first) != 1 || first[0].State != schedule.StateExpired || len(second) != 0 {
			t.Fatalf("first=%#v second=%#v", first, second)
		}
		if h.Transitions != nil {
			n := 0
			for _, tr := range h.Transitions() {
				if tr.RegistrationID == "held" && tr.To == schedule.StateExpired {
					n++
				}
			}
			if n != 1 {
				t.Fatalf("expiry transitions = %d", n)
			}
		}
	})
	t.Run("OfferNextFirstEligible", func(t *testing.T) {
		h := newHarness(t)
		e := event("event", 1)
		e.Capacity, e.Waitlist = 1, true
		putEvent(t, h.Store, e)
		roster, _ := h.Store.Roster(context.Background(), e.ID)
		regs := []schedule.Registration{{ID: "large", EventID: e.ID, Seats: 2, State: schedule.StateWaitlisted, RequestedAt: Epoch}, {ID: "ineligible", EventID: e.ID, Seats: 1, State: schedule.StateWaitlisted, RequestedAt: Epoch.Add(time.Second)}, {ID: "eligible", EventID: e.ID, Seats: 1, State: schedule.StateWaitlisted, RequestedAt: Epoch.Add(2 * time.Second)}}
		for _, r := range regs {
			updated, err := h.Store.Apply(context.Background(), schedule.Change{EventID: e.ID, EventRevision: roster.Event.Revision, RosterRevision: roster.Event.RosterRevision, Put: []schedule.Registration{r}})
			if err != nil {
				t.Fatal(err)
			}
			roster = updated
		}
		request := schedule.OfferRequest{EventID: e.ID, Hold: 72 * time.Hour, HoldKind: "offer", Eligible: func(r schedule.Registration) bool { return r.ID != "ineligible" }, Key: "offer-key", At: Epoch}
		first, ok, err := schedule.OfferNext(context.Background(), h.Store, request)
		if err != nil || !ok {
			t.Fatalf("OfferNext = %#v, %t, %v", first, ok, err)
		}
		if first.Registration.ID != "eligible" || first.Registration.State != schedule.StateHeld {
			t.Fatalf("offered %#v", first.Registration)
		}
		replay, ok, err := schedule.OfferNext(context.Background(), h.Store, request)
		if err != nil || !ok || !replay.Replayed || replay.Registration.ID != first.Registration.ID {
			t.Fatalf("replay = %#v, %t, %v", replay, ok, err)
		}
	})
	t.Run("OfferHoldIsAbsolute", func(t *testing.T) {
		cases := []struct {
			name string
			at   time.Time
			want time.Time
		}{{"before DST", time.Date(2027, 3, 8, 9, 0, 0, 0, time.UTC), time.Date(2027, 3, 11, 9, 0, 0, 0, time.UTC)}, {"cross DST", time.Date(2027, 3, 12, 9, 0, 0, 0, time.UTC), time.Date(2027, 3, 15, 9, 0, 0, 0, time.UTC)}}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				h := newHarness(t)
				e := event("event", 1)
				e.Start = tc.want.Add(time.Hour)
				e.End = e.Start.Add(time.Hour)
				e.Capacity, e.Waitlist = 1, true
				putEvent(t, h.Store, e)
				roster, _ := h.Store.Roster(context.Background(), e.ID)
				reg := schedule.Registration{ID: "wait", EventID: e.ID, Seats: 1, State: schedule.StateWaitlisted, RequestedAt: Epoch}
				roster, err := h.Store.Apply(context.Background(), schedule.Change{EventID: e.ID, EventRevision: roster.Event.Revision, RosterRevision: roster.Event.RosterRevision, Put: []schedule.Registration{reg}})
				if err != nil {
					t.Fatal(err)
				}
				res, ok, err := schedule.OfferNext(context.Background(), h.Store, schedule.OfferRequest{EventID: e.ID, Hold: 72 * time.Hour, HoldKind: "offer", Key: "offer", At: tc.at})
				if err != nil || !ok {
					t.Fatalf("OfferNext = %t, %v", ok, err)
				}
				if !res.Registration.HoldUntil.Equal(tc.want) {
					t.Fatalf("HoldUntil = %s, want %s", res.Registration.HoldUntil, tc.want)
				}
			})
		}
	})
	t.Run("CancelReleasesSeatOnce", func(t *testing.T) {
		h := newHarness(t)
		e := event("event", 1)
		e.Capacity, e.Waitlist = 1, true
		putEvent(t, h.Store, e)
		first, err := schedule.Book(context.Background(), h.Store, schedule.BookRequest{EventID: e.ID, RegistrationID: "first", Seats: 1, Key: "book", At: Epoch})
		if err != nil {
			t.Fatal(err)
		}
		_, err = schedule.Book(context.Background(), h.Store, schedule.BookRequest{EventID: e.ID, RegistrationID: "second", Seats: 1, Key: "wait", At: Epoch})
		if err != nil {
			t.Fatal(err)
		}
		request := schedule.CancelRequest{RegistrationID: first.Registration.ID, Expected: first.Registration.Revision, Reason: "cancelled", Key: "cancel", At: Epoch.Add(time.Minute)}
		cancelled, err := schedule.Cancel(context.Background(), h.Store, request)
		if err != nil {
			t.Fatal(err)
		}
		replay, err := schedule.Cancel(context.Background(), h.Store, request)
		if err != nil || !replay.Replayed || replay.Registration.ID != cancelled.Registration.ID {
			t.Fatalf("replay = %#v, %v", replay, err)
		}
		request.Key = "another-key"
		request.Expected = cancelled.Registration.Revision
		if _, err := schedule.Cancel(context.Background(), h.Store, request); !errors.Is(err, schedule.ErrTransition) {
			t.Fatalf("second cancel = %v", err)
		}
	})
	t.Run("ConfirmExpiredHoldFails", func(t *testing.T) {
		h := newHarness(t)
		e := event("event", 1)
		e.Capacity = 1
		putEvent(t, h.Store, e)
		booked, err := schedule.Book(context.Background(), h.Store, schedule.BookRequest{EventID: e.ID, RegistrationID: "held", Hold: time.Minute, Key: "hold", At: Epoch})
		if err != nil {
			t.Fatal(err)
		}
		_, err = schedule.Confirm(context.Background(), h.Store, schedule.ConfirmRequest{RegistrationID: "held", Expected: booked.Registration.Revision, Key: "confirm", At: Epoch.Add(time.Minute)})
		if !errors.Is(err, schedule.ErrHoldExpired) {
			t.Fatalf("Confirm error = %v", err)
		}
	})
	t.Run("CapacityCutRacesBooking", func(t *testing.T) {
		h := newHarness(t)
		e := event("event", 1)
		e.Capacity = 2
		putEvent(t, h.Store, e)
		if _, err := schedule.Book(context.Background(), h.Store, schedule.BookRequest{EventID: e.ID, RegistrationID: "initial", Seats: 1, Key: "initial", At: Epoch}); err != nil {
			t.Fatal(err)
		}
		var wg sync.WaitGroup
		var cutErr, bookErr error
		var mu sync.Mutex
		wg.Add(2)
		go func() {
			defer wg.Done()
			_, err := schedule.SaveEvent(context.Background(), h.Store, schedule.SaveEventRequest{Event: schedule.Event{ID: e.ID, Title: e.Title, Summary: e.Summary, Location: e.Location, Zone: e.Zone, Start: e.Start, End: e.End, Status: e.Status, Capacity: 1}, Expected: 1, Key: "cut", At: Epoch})
			mu.Lock()
			cutErr = err
			mu.Unlock()
		}()
		go func() {
			defer wg.Done()
			_, err := schedule.Book(context.Background(), h.Store, schedule.BookRequest{EventID: e.ID, RegistrationID: "race", Seats: 1, Key: "race", At: Epoch})
			mu.Lock()
			bookErr = err
			mu.Unlock()
		}()
		wg.Wait()
		stored := getEvent(t, h.Store, e.ID)
		roster, err := h.Store.Roster(context.Background(), e.ID)
		if err != nil {
			t.Fatal(err)
		}
		used := schedule.Occupy(roster.Event, roster.Registrations, Epoch).Used
		if cutErr == nil && bookErr == nil && used > stored.Capacity {
			t.Fatalf("both writes succeeded: used=%d capacity=%d", used, stored.Capacity)
		}
		if cutErr != nil && !errors.Is(cutErr, schedule.ErrConflict) && !errors.Is(cutErr, schedule.ErrInvalid) {
			var ve *schedule.ValidationError
			if !errors.As(cutErr, &ve) {
				t.Fatalf("capacity cut error = %v", cutErr)
			}
		}
	})
	t.Run("ReopenKeepsEverything", func(t *testing.T) {
		h := newHarness(t)
		if h.Reopen == nil {
			t.Skip("Harness.Reopen is nil")
		}
		e := putEvent(t, h.Store, event("event", 1))
		roster, err := h.Store.Apply(context.Background(), schedule.Change{EventID: e.ID, EventRevision: e.Revision, RosterRevision: e.RosterRevision, Put: []schedule.Registration{registration("r1", e.ID)}, Receipt: schedule.Receipt{Key: "receipt", Op: schedule.OpBook, EventID: e.ID, RegistrationID: "r1", Revision: 1, At: Epoch}})
		if err != nil {
			t.Fatal(err)
		}
		checkReopen(t, h, []string{e.ID}, []string{"r1"}, []string{"receipt"})
		if roster.Event.RosterRevision != e.RosterRevision+1 {
			t.Fatal("Apply did not advance roster revision")
		}
	})
}

func event(id string, day int) schedule.Event {
	start := Epoch.Add(time.Duration(day) * 24 * time.Hour)
	return schedule.Event{ID: id, Title: "Event " + id, Zone: "UTC", Status: schedule.EventPublished, Start: start, End: start.Add(time.Hour), Capacity: 0, PublishedAt: Epoch, Created: Epoch, Updated: Epoch}
}

func eventSpan(id string, start, end time.Time) schedule.Event {
	e := event(id, 1)
	e.Start, e.End = start, end
	return e
}

func registration(id, eventID string) schedule.Registration {
	return schedule.Registration{ID: id, EventID: eventID, PartyID: "party", Label: id, Seats: 1, State: schedule.StateConfirmed, RequestedAt: Epoch, Created: Epoch, Updated: Epoch}
}

func putEvent(t *testing.T, s schedule.Store, e schedule.Event) schedule.Event {
	t.Helper()
	stored, err := s.PutEvent(context.Background(), schedule.EventWrite{Event: e})
	if err != nil {
		t.Fatal(err)
	}
	return stored
}

func getEvent(t *testing.T, s schedule.Store, id string) schedule.Event {
	t.Helper()
	e, err := s.Event(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	return e
}

func applyInsert(t *testing.T, s schedule.Store, e schedule.Event, id, key string) schedule.Roster {
	t.Helper()
	r, err := s.Apply(context.Background(), schedule.Change{EventID: e.ID, EventRevision: e.Revision, RosterRevision: e.RosterRevision, Put: []schedule.Registration{registration(id, e.ID)}, Receipt: schedule.Receipt{Key: key, EventID: e.ID, RegistrationID: id, At: Epoch}})
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func ids(events []schedule.Event) []string {
	out := make([]string, len(events))
	for i, e := range events {
		out[i] = e.ID
	}
	return out
}

func equalEvent(a, b schedule.Event) bool {
	return a.ID == b.ID && a.ResourceID == b.ResourceID && a.Title == b.Title && a.Summary == b.Summary && a.Location == b.Location && a.Category == b.Category && a.Visibility == b.Visibility && a.Status == b.Status && a.Zone == b.Zone && a.AllDay == b.AllDay && a.StartDate == b.StartDate && a.EndDate == b.EndDate && a.Start.Equal(b.Start) && a.End.Equal(b.End) && a.Capacity == b.Capacity && a.Waitlist == b.Waitlist && a.BookingOpens.Equal(b.BookingOpens) && a.BookingCloses.Equal(b.BookingCloses) && a.Sequence == b.Sequence && a.PublishedAt.Equal(b.PublishedAt) && a.Revision == b.Revision && a.RosterRevision == b.RosterRevision && a.Created.Equal(b.Created) && a.Updated.Equal(b.Updated) && reflect.DeepEqual(a.Meta, b.Meta)
}

func checkReopen(t *testing.T, h Harness, eventIDs, registrationIDs, keys []string) {
	t.Helper()
	if h.Reopen == nil {
		return
	}
	store := h.Reopen()
	if store == nil {
		t.Fatal("Reopen returned nil")
	}
	for _, id := range eventIDs {
		before, err := h.Store.Event(context.Background(), id)
		if err != nil {
			t.Fatal(err)
		}
		after, err := store.Event(context.Background(), id)
		if err != nil {
			t.Fatal(err)
		}
		if !equalEvent(before, after) {
			t.Errorf("reopened event %q differs", id)
		}
	}
	for _, id := range registrationIDs {
		before, err := h.Store.Registration(context.Background(), id)
		if err != nil {
			t.Fatal(err)
		}
		after, err := store.Registration(context.Background(), id)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(before, after) {
			t.Errorf("reopened registration %q differs", id)
		}
	}
	for _, key := range keys {
		before, err := h.Store.Receipt(context.Background(), key)
		if err != nil {
			t.Fatal(err)
		}
		after, err := store.Receipt(context.Background(), key)
		if err != nil {
			t.Fatal(err)
		}
		if before != after {
			t.Errorf("reopened receipt %q differs", key)
		}
	}
}
