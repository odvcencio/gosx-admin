package mdppstudio

import (
	"context"
	"errors"
	"sort"
	"sync"
	"time"
)

var (
	ErrNotFound     = errors.New("mdppstudio: not found")
	ErrExists       = errors.New("mdppstudio: already exists")
	ErrConflict     = errors.New("mdppstudio: draft changed")
	ErrLocked       = errors.New("mdppstudio: draft is locked")
	ErrDuplicateKey = errors.New("mdppstudio: idempotency key already stored")
	ErrTooLarge     = errors.New("mdppstudio: source too large")
)

// Preview is the last good server render of a saved revision.
type Preview struct {
	Target          Target
	RendererVersion string
	Hash            string
	HTML            string
	Text            string
	Revision        int64
	At              time.Time
}

// Draft is one document being written.
type Draft struct {
	ID        string
	Kind      string
	Status    string
	Locale    string
	Title     string
	Source    string
	Revision  int64
	Locked    bool
	Template  string
	Author    string
	UpdatedBy string
	Created   time.Time
	Updated   time.Time
	Preview   Preview
}

// VersionKind says why a version exists.
type VersionKind string

const (
	VersionSave     VersionKind = "save"
	VersionAutosave VersionKind = "autosave"
	VersionRestore  VersionKind = "restore"
	VersionConflict VersionKind = "conflict"
)

// Version is an immutable copy of a draft's source.
type Version struct {
	ID              string
	DraftID         string
	Revision        int64
	Kind            VersionKind
	Source          string
	Author          string
	At              time.Time
	RestoredFrom    string
	RendererVersion string
	OutputHash      string
}

// VersionSummary is a Version without its source, for history lists.
type VersionSummary struct {
	ID       string      `json:"id"`
	Revision int64       `json:"revision"`
	Kind     VersionKind `json:"kind"`
	Author   string      `json:"author"`
	At       time.Time   `json:"at"`
}

// Summary returns v without its source.
func (v Version) Summary() VersionSummary {
	return VersionSummary{ID: v.ID, Revision: v.Revision, Kind: v.Kind, Author: v.Author, At: v.At}
}

// DraftReceipt records the first result of an idempotent draft write.
type DraftReceipt struct {
	Key      string
	DraftID  string
	Revision int64
}

// DraftWrite is one draft create or update.
type DraftWrite struct {
	Draft    Draft
	Expected int64
	Version  *Version
	Key      string
}

// DraftStore persists drafts and versions. PutDraft commits the draft,
// optional version, and receipt together. Stores never delete versions.
type DraftStore interface {
	Draft(ctx context.Context, id string) (Draft, error)
	PutDraft(ctx context.Context, w DraftWrite) (Draft, error)
	AddVersion(ctx context.Context, v Version) error
	Versions(ctx context.Context, draftID string, limit int) ([]Version, error)
	Version(ctx context.Context, draftID, versionID string) (Version, error)
	Receipt(ctx context.Context, key string) (DraftReceipt, error)
}

// MemoryDraftStore is the reference DraftStore, for tests and small sites.
type MemoryDraftStore struct {
	mu       sync.RWMutex
	drafts   map[string]Draft
	versions map[string]map[string]Version
	receipts map[string]DraftReceipt
}

var _ DraftStore = (*MemoryDraftStore)(nil)

// NewMemoryDraftStore returns an empty store.
func NewMemoryDraftStore() *MemoryDraftStore {
	return &MemoryDraftStore{drafts: map[string]Draft{}, versions: map[string]map[string]Version{}, receipts: map[string]DraftReceipt{}}
}

func contextErr(ctx context.Context) error {
	if ctx == nil {
		return nil
	}
	return ctx.Err()
}

// Draft implements DraftStore.
func (m *MemoryDraftStore) Draft(ctx context.Context, id string) (Draft, error) {
	if err := contextErr(ctx); err != nil {
		return Draft{}, err
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	d, ok := m.drafts[id]
	if !ok {
		return Draft{}, ErrNotFound
	}
	return d, nil
}

// PutDraft implements DraftStore.
func (m *MemoryDraftStore) PutDraft(ctx context.Context, w DraftWrite) (Draft, error) {
	if err := contextErr(ctx); err != nil {
		return Draft{}, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if w.Key != "" {
		if _, ok := m.receipts[w.Key]; ok {
			return Draft{}, ErrDuplicateKey
		}
	}
	d := w.Draft
	if d.ID == "" {
		return Draft{}, ErrNotFound
	}
	if w.Expected == 0 {
		if _, ok := m.drafts[d.ID]; ok {
			return Draft{}, ErrExists
		}
		d.Revision = 1
		if d.Created.IsZero() {
			d.Created = d.Updated
		}
		if d.Created.IsZero() {
			d.Created = time.Now().UTC()
		}
	} else {
		current, ok := m.drafts[d.ID]
		if !ok {
			return Draft{}, ErrNotFound
		}
		if current.Revision != w.Expected {
			return Draft{}, ErrConflict
		}
		if current.Locked && d.Locked {
			return Draft{}, ErrLocked
		}
		d.Created = current.Created
		d.Revision = w.Expected + 1
	}
	if d.Updated.IsZero() {
		d.Updated = time.Now().UTC()
	}
	if d.UpdatedBy == "" {
		d.UpdatedBy = d.Author
	}
	if w.Version != nil {
		v := *w.Version
		if v.DraftID == "" {
			v.DraftID = d.ID
		}
		if v.DraftID != d.ID {
			return Draft{}, ErrNotFound
		}
		v.Revision = d.Revision
		if m.versions[d.ID] == nil {
			m.versions[d.ID] = map[string]Version{}
		}
		if _, ok := m.versions[d.ID][v.ID]; ok {
			return Draft{}, ErrExists
		}
		m.versions[d.ID][v.ID] = v
	}
	m.drafts[d.ID] = d
	if w.Key != "" {
		m.receipts[w.Key] = DraftReceipt{Key: w.Key, DraftID: d.ID, Revision: d.Revision}
	}
	return d, nil
}

// AddVersion stores a VersionConflict copy without changing the draft.
func (m *MemoryDraftStore) AddVersion(ctx context.Context, v Version) error {
	if err := contextErr(ctx); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.drafts[v.DraftID]; !ok {
		return ErrNotFound
	}
	if m.versions[v.DraftID] == nil {
		m.versions[v.DraftID] = map[string]Version{}
	}
	if _, ok := m.versions[v.DraftID][v.ID]; ok {
		return ErrExists
	}
	m.versions[v.DraftID][v.ID] = v
	return nil
}

// Versions returns up to limit versions, newest first; 0 means all.
func (m *MemoryDraftStore) Versions(ctx context.Context, draftID string, limit int) ([]Version, error) {
	if err := contextErr(ctx); err != nil {
		return nil, err
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	if _, ok := m.drafts[draftID]; !ok {
		return nil, ErrNotFound
	}
	versions := make([]Version, 0, len(m.versions[draftID]))
	for _, v := range m.versions[draftID] {
		versions = append(versions, v)
	}
	sort.SliceStable(versions, func(i, j int) bool {
		if versions[i].At.Equal(versions[j].At) {
			return versions[i].ID > versions[j].ID
		}
		return versions[i].At.After(versions[j].At)
	})
	if limit > 0 && len(versions) > limit {
		versions = versions[:limit]
	}
	return versions, nil
}

// Version implements DraftStore.
func (m *MemoryDraftStore) Version(ctx context.Context, draftID, versionID string) (Version, error) {
	if err := contextErr(ctx); err != nil {
		return Version{}, err
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	v, ok := m.versions[draftID][versionID]
	if !ok {
		return Version{}, ErrNotFound
	}
	return v, nil
}

// Receipt returns ErrNotFound for an unknown key.
func (m *MemoryDraftStore) Receipt(ctx context.Context, key string) (DraftReceipt, error) {
	if err := contextErr(ctx); err != nil {
		return DraftReceipt{}, err
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	r, ok := m.receipts[key]
	if !ok {
		return DraftReceipt{}, ErrNotFound
	}
	return r, nil
}
