// Package draftstoretest contains the DraftStore conformance suite.
package draftstoretest

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"m31labs.dev/gosx-admin/mdppstudio"
)

// Harness is one fresh, empty store under test.
type Harness struct {
	Store mdppstudio.DraftStore
	// Reopen returns a new handle on the same data. Nil skips reopen checks.
	Reopen func() mdppstudio.DraftStore
}

// Run runs every conformance subtest with t.Run; open returns a fresh store.
func Run(t *testing.T, open func(t *testing.T) Harness) {
	t.Helper()
	run := func(name string, fn func(*testing.T, Harness)) {
		t.Run(name, func(t *testing.T) {
			h := open(t)
			if h.Store == nil {
				t.Fatal("open returned a nil store")
			}
			fn(t, h)
		})
	}
	seed := func(t *testing.T, h Harness, id string) mdppstudio.Draft {
		t.Helper()
		d, err := h.Store.PutDraft(context.Background(), mdppstudio.DraftWrite{Expected: 0, Draft: mdppstudio.Draft{ID: id, Source: "initial", Author: "a", UpdatedBy: "a", Updated: time.Unix(1, 0)}})
		if err != nil {
			t.Fatal(err)
		}
		return d
	}
	run("CreateStartsAtRevisionOne", func(t *testing.T, h Harness) {
		d := seed(t, h, "draft")
		if d.Revision != 1 {
			t.Fatalf("revision = %d", d.Revision)
		}
	})
	run("UpdateNeedsExpectedRevision", func(t *testing.T, h Harness) {
		d := seed(t, h, "draft")
		if _, err := h.Store.PutDraft(context.Background(), mdppstudio.DraftWrite{Draft: mdppstudio.Draft{ID: d.ID, Source: "next"}, Expected: 3}); !errors.Is(err, mdppstudio.ErrConflict) {
			t.Fatalf("error = %v", err)
		}
	})
	run("CreateDuplicateIsExists", func(t *testing.T, h Harness) {
		seed(t, h, "draft")
		if _, err := h.Store.PutDraft(context.Background(), mdppstudio.DraftWrite{Draft: mdppstudio.Draft{ID: "draft"}}); !errors.Is(err, mdppstudio.ErrExists) {
			t.Fatalf("error = %v", err)
		}
	})
	run("LockedRejectsWrites", func(t *testing.T, h Harness) {
		d := seed(t, h, "draft")
		d.Locked = true
		d.Revision = 0
		if _, err := h.Store.PutDraft(context.Background(), mdppstudio.DraftWrite{Draft: d, Expected: 1}); err != nil {
			t.Fatal(err)
		}
		d.Source = "next"
		if _, err := h.Store.PutDraft(context.Background(), mdppstudio.DraftWrite{Draft: d, Expected: 2}); !errors.Is(err, mdppstudio.ErrLocked) {
			t.Fatalf("error = %v", err)
		}
	})
	run("VersionStoredWithWrite", func(t *testing.T, h Harness) {
		d := mdppstudio.Draft{ID: "draft", Source: "v1"}
		v := &mdppstudio.Version{ID: id(1), Source: "v1"}
		if _, err := h.Store.PutDraft(context.Background(), mdppstudio.DraftWrite{Draft: d, Version: v}); err != nil {
			t.Fatal(err)
		}
		got, err := h.Store.Version(context.Background(), d.ID, v.ID)
		if err != nil || got.Source != "v1" || got.Revision != 1 {
			t.Fatalf("version = %#v, %v", got, err)
		}
	})
	run("WriteIsAtomic", func(t *testing.T, h Harness) {
		_, err := h.Store.PutDraft(context.Background(), mdppstudio.DraftWrite{Draft: mdppstudio.Draft{ID: "draft", Source: "v1"}, Version: &mdppstudio.Version{ID: id(2), DraftID: "elsewhere"}})
		if err == nil {
			t.Fatal("expected failed write")
		}
		if _, err = h.Store.Draft(context.Background(), "draft"); !errors.Is(err, mdppstudio.ErrNotFound) {
			t.Fatalf("draft changed on failed write: %v", err)
		}
	})
	run("DuplicateKeyWritesNothing", func(t *testing.T, h Harness) {
		d := seed(t, h, "draft")
		next := d
		next.Source = "next"
		if _, err := h.Store.PutDraft(context.Background(), mdppstudio.DraftWrite{Draft: next, Expected: 1, Key: "key"}); err != nil {
			t.Fatal(err)
		}
		next.Source = "other"
		if _, err := h.Store.PutDraft(context.Background(), mdppstudio.DraftWrite{Draft: next, Expected: 2, Key: "key"}); !errors.Is(err, mdppstudio.ErrDuplicateKey) {
			t.Fatalf("error = %v", err)
		}
		got, _ := h.Store.Draft(context.Background(), d.ID)
		if got.Revision != 2 || got.Source != "next" {
			t.Fatalf("draft changed: %#v", got)
		}
	})
	run("ReceiptReturnsFirstRevision", func(t *testing.T, h Harness) {
		d := seed(t, h, "draft")
		d.Source = "next"
		if _, err := h.Store.PutDraft(context.Background(), mdppstudio.DraftWrite{Draft: d, Expected: 1, Key: "key"}); err != nil {
			t.Fatal(err)
		}
		r, err := h.Store.Receipt(context.Background(), "key")
		if err != nil || r.Revision != 2 || r.DraftID != d.ID {
			t.Fatalf("receipt = %#v, %v", r, err)
		}
	})
	run("AddVersionLeavesDraft", func(t *testing.T, h Harness) {
		d := seed(t, h, "draft")
		if err := h.Store.AddVersion(context.Background(), mdppstudio.Version{ID: id(3), DraftID: d.ID, Source: "typed", Kind: mdppstudio.VersionConflict}); err != nil {
			t.Fatal(err)
		}
		got, _ := h.Store.Draft(context.Background(), d.ID)
		if got.Revision != 1 || got.Source != "initial" {
			t.Fatalf("draft changed: %#v", got)
		}
	})
	run("VersionsNewestFirstAndLimit", func(t *testing.T, h Harness) {
		d := seed(t, h, "draft")
		for i := 1; i <= 3; i++ {
			if err := h.Store.AddVersion(context.Background(), mdppstudio.Version{ID: id(i), DraftID: d.ID, At: time.Unix(int64(i), 0)}); err != nil {
				t.Fatal(err)
			}
		}
		vs, err := h.Store.Versions(context.Background(), d.ID, 2)
		if err != nil || len(vs) != 2 || vs[0].ID != id(3) {
			t.Fatalf("versions = %#v, %v", vs, err)
		}
	})
	run("VersionsNeverDeleted", func(t *testing.T, h Harness) {
		d := seed(t, h, "draft")
		v := mdppstudio.Version{ID: id(4), DraftID: d.ID}
		if err := h.Store.AddVersion(context.Background(), v); err != nil {
			t.Fatal(err)
		}
		d.Source = "next"
		if _, err := h.Store.PutDraft(context.Background(), mdppstudio.DraftWrite{Draft: d, Expected: 1}); err != nil {
			t.Fatal(err)
		}
		if _, err := h.Store.Version(context.Background(), d.ID, v.ID); err != nil {
			t.Fatal(err)
		}
	})
	run("ContextDoneReturnsError", func(t *testing.T, h Harness) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if _, err := h.Store.Draft(ctx, "draft"); err == nil {
			t.Fatal("expected context error")
		}
	})
	run("ReopenKeepsEverything", func(t *testing.T, h Harness) {
		if h.Reopen == nil {
			t.Skip("store does not support reopen")
		}
		d := mdppstudio.Draft{ID: "draft", Source: "saved"}
		v := &mdppstudio.Version{ID: id(5), Source: "saved"}
		if _, err := h.Store.PutDraft(context.Background(), mdppstudio.DraftWrite{Draft: d, Version: v, Key: "persist"}); err != nil {
			t.Fatal(err)
		}
		store := h.Reopen()
		got, err := store.Draft(context.Background(), d.ID)
		if err != nil || got.Revision != 1 {
			t.Fatalf("draft = %#v, %v", got, err)
		}
		if _, err = store.Version(context.Background(), d.ID, v.ID); err != nil {
			t.Fatal(err)
		}
		receipt, err := store.Receipt(context.Background(), "persist")
		if err != nil || receipt.Revision != 1 {
			t.Fatalf("receipt = %#v, %v", receipt, err)
		}
	})
}

func id(n int) string { return fmt.Sprintf("%032x", n) }
