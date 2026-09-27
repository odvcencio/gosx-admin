package schedule_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"m31labs.dev/gosx-admin/schedule"
	"m31labs.dev/gosx-admin/schedule/scheduletest"
)

func TestConcurrentSchedulePaths(t *testing.T) {
	t.Run("booking and payment holds", func(t *testing.T) {
		store := schedule.NewMemoryStore()
		e := fixtureEvent("event")
		e.Capacity, e.Waitlist = 1, false
		if _, err := store.PutEvent(context.Background(), schedule.EventWrite{Event: e}); err != nil {
			t.Fatal(err)
		}
		var wg sync.WaitGroup
		var mu sync.Mutex
		held, full := 0, 0
		for i := 0; i < 8; i++ {
			i := i
			wg.Add(1)
			go func() {
				defer wg.Done()
				result, err := schedule.Book(context.Background(), store, schedule.BookRequest{EventID: e.ID, RegistrationID: string(rune('a' + i)), PartyID: string(rune('A' + i)), Seats: 1, Hold: 15 * time.Minute, HoldKind: "payment", Key: "hold-" + string(rune('a'+i)), At: scheduletest.Epoch})
				mu.Lock()
				defer mu.Unlock()
				if err != nil {
					if errors.Is(err, schedule.ErrFull) {
						full++
						return
					}
					t.Errorf("Book: %v", err)
					return
				}
				if result.Registration.State == schedule.StateHeld {
					held++
				} else {
					t.Errorf("registration state = %q", result.Registration.State)
				}
			}()
		}
		wg.Wait()
		if held != 1 || full != 7 {
			t.Fatalf("held=%d full=%d, want one hold and seven full", held, full)
		}
	})

	t.Run("waitlist offer race", func(t *testing.T) {
		store := schedule.NewMemoryStore()
		e := fixtureEvent("event")
		e.Capacity, e.Waitlist = 1, true
		if _, err := store.PutEvent(context.Background(), schedule.EventWrite{Event: e}); err != nil {
			t.Fatal(err)
		}
		roster, err := store.Roster(context.Background(), e.ID)
		if err != nil {
			t.Fatal(err)
		}
		for i := 0; i < 2; i++ {
			reg := schedule.Registration{ID: string(rune('a' + i)), EventID: e.ID, Seats: 1, State: schedule.StateWaitlisted, RequestedAt: scheduletest.Epoch.Add(time.Duration(i) * time.Second)}
			roster, err = store.Apply(context.Background(), schedule.Change{EventID: e.ID, EventRevision: roster.Event.Revision, RosterRevision: roster.Event.RosterRevision, Put: []schedule.Registration{reg}})
			if err != nil {
				t.Fatal(err)
			}
		}
		var wg sync.WaitGroup
		var mu sync.Mutex
		offers := 0
		var firstErr error
		for i := 0; i < 2; i++ {
			i := i
			wg.Add(1)
			go func() {
				defer wg.Done()
				_, ok, err := schedule.OfferNext(context.Background(), store, schedule.OfferRequest{EventID: e.ID, Hold: 72 * time.Hour, HoldKind: "offer", Key: "offer-" + string(rune('a'+i)), At: scheduletest.Epoch})
				mu.Lock()
				defer mu.Unlock()
				if err != nil {
					if !errors.Is(err, schedule.ErrConflict) {
						firstErr = err
					}
					return
				}
				if ok {
					offers++
				}
			}()
		}
		wg.Wait()
		if firstErr != nil {
			t.Fatal(firstErr)
		}
		if offers != 1 {
			t.Fatalf("successful offers = %d, want 1", offers)
		}
		got, err := store.Roster(context.Background(), e.ID)
		if err != nil {
			t.Fatal(err)
		}
		held, waitlisted := 0, 0
		for _, r := range got.Registrations {
			if r.State == schedule.StateHeld {
				held++
			}
			if r.State == schedule.StateWaitlisted {
				waitlisted++
			}
		}
		if held != 1 || waitlisted != 1 {
			t.Fatalf("held=%d waitlisted=%d", held, waitlisted)
		}
	})

	t.Run("Apply compare and set", func(t *testing.T) {
		store := schedule.NewMemoryStore()
		e := fixtureEvent("event")
		if _, err := store.PutEvent(context.Background(), schedule.EventWrite{Event: e}); err != nil {
			t.Fatal(err)
		}
		roster, err := store.Roster(context.Background(), e.ID)
		if err != nil {
			t.Fatal(err)
		}
		start := make(chan struct{})
		var wg sync.WaitGroup
		errs := make(chan error, 2)
		for _, id := range []string{"a", "b"} {
			id := id
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				_, err := store.Apply(context.Background(), schedule.Change{EventID: e.ID, EventRevision: roster.Event.Revision, RosterRevision: roster.Event.RosterRevision, Put: []schedule.Registration{{ID: id, EventID: e.ID, Seats: 1, State: schedule.StateConfirmed, RequestedAt: scheduletest.Epoch}}})
				errs <- err
			}()
		}
		close(start)
		wg.Wait()
		close(errs)
		success, conflict := 0, 0
		for err := range errs {
			if err == nil {
				success++
			} else if errors.Is(err, schedule.ErrConflict) {
				conflict++
			} else {
				t.Fatalf("Apply error = %v", err)
			}
		}
		if success != 1 || conflict != 1 {
			t.Fatalf("success=%d conflict=%d", success, conflict)
		}
		got, err := store.Roster(context.Background(), e.ID)
		if err != nil {
			t.Fatal(err)
		}
		if len(got.Registrations) != 1 || got.Event.RosterRevision != 2 {
			t.Fatalf("roster after race = %#v", got)
		}
	})
}
