package calendar

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

type FileScheduleStore struct {
	mu     sync.Mutex
	path   string
	memory *MemoryScheduleStore
}

var _ ScheduleStore = (*FileScheduleStore)(nil)

func NewFileScheduleStore(path string, seed MemorySeed) (*FileScheduleStore, error) {
	store, err := newFileScheduleStore(path, seed)
	if err != nil {
		return nil, err
	}
	if err := store.persist(); err != nil {
		return nil, err
	}
	return store, nil
}

func OpenFileScheduleStore(path string) (*FileScheduleStore, error) {
	if path == "" {
		return nil, fmt.Errorf("calendar schedule path is required")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return newFileScheduleStore(path, MemorySeed{})
		}
		return nil, err
	}
	var seed MemorySeed
	if err := json.Unmarshal(data, &seed); err != nil {
		return nil, err
	}
	return newFileScheduleStore(path, seed)
}

func newFileScheduleStore(path string, seed MemorySeed) (*FileScheduleStore, error) {
	if path == "" {
		return nil, fmt.Errorf("calendar schedule path is required")
	}
	return &FileScheduleStore{
		path:   path,
		memory: NewMemoryScheduleStore(seed),
	}, nil
}

func (s *FileScheduleStore) Snapshot() MemorySeed {
	return s.memory.Snapshot()
}

func (s *FileScheduleStore) ListEvents(filter Filter) []Event {
	return s.memory.ListEvents(filter)
}

func (s *FileScheduleStore) SaveEvent(event Event) (Event, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	saved, err := s.memory.SaveEvent(event)
	if err != nil {
		return Event{}, err
	}
	if err := s.persist(); err != nil {
		return Event{}, err
	}
	return saved, nil
}

func (s *FileScheduleStore) ListResources(filter ResourceFilter) []Resource {
	return s.memory.ListResources(filter)
}

func (s *FileScheduleStore) SaveResource(resource Resource) (Resource, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	saved, err := s.memory.SaveResource(resource)
	if err != nil {
		return Resource{}, err
	}
	if err := s.persist(); err != nil {
		return Resource{}, err
	}
	return saved, nil
}

func (s *FileScheduleStore) ListRegistrations(filter RegistrationFilter) []Registration {
	return s.memory.ListRegistrations(filter)
}

func (s *FileScheduleStore) SaveRegistration(registration Registration) (Registration, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	saved, err := s.memory.SaveRegistration(registration)
	if err != nil {
		return Registration{}, err
	}
	if err := s.persist(); err != nil {
		return Registration{}, err
	}
	return saved, nil
}

func (s *FileScheduleStore) persist() error {
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s.memory.Snapshot(), "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(s.path)+".*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, s.path); err != nil {
		return err
	}
	return syncDir(dir)
}

func syncDir(dir string) error {
	f, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer f.Close()
	return f.Sync()
}
