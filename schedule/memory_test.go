package schedule_test

import (
	"testing"

	"m31labs.dev/gosx-admin/schedule"
	"m31labs.dev/gosx-admin/schedule/scheduletest"
)

func TestMemoryStoreConformance(t *testing.T) {
	for _, withReopen := range []bool{false, true} {
		name := "without-Reopen"
		if withReopen {
			name = "with-Reopen"
		}
		t.Run(name, func(t *testing.T) {
			scheduletest.Run(t, func(t *testing.T) scheduletest.Harness {
				store := schedule.NewMemoryStore()
				h := scheduletest.Harness{Store: store, Transitions: store.Transitions}
				if withReopen {
					h.Reopen = func() schedule.Store { return store }
				}
				return h
			})
		})
	}
}
