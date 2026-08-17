// Package state tracks which Forgejo time entries have already been pushed
// to Toggl, so a poll can skip Toggl entirely when there's nothing new.
package state

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

// State is the set of Forgejo time-entry IDs already synced to Toggl,
// backed by a JSON file on disk.
type State struct {
	path string
	mu   sync.Mutex
	ids  map[int64]struct{}
}

// Load reads the state file at path, or starts empty if it doesn't exist
// yet.
func Load(path string) (*State, error) {
	s := &State{path: path, ids: make(map[int64]struct{})}

	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}

	var ids []int64
	if err := json.Unmarshal(data, &ids); err != nil {
		return nil, err
	}
	for _, id := range ids {
		s.ids[id] = struct{}{}
	}
	return s, nil
}

// Has reports whether id has already been synced.
func (s *State) Has(id int64) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.ids[id]
	return ok
}

// Len returns the number of synced IDs.
func (s *State) Len() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.ids)
}

// Add records id as synced and persists the state file. A no-op, but still
// persisted, if id was already recorded.
func (s *State) Add(id int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.ids[id] = struct{}{}
	return s.saveLocked()
}

func (s *State) saveLocked() error {
	ids := make([]int64, 0, len(s.ids))
	for id := range s.ids {
		ids = append(ids, id)
	}

	data, err := json.Marshal(ids)
	if err != nil {
		return err
	}

	dir := filepath.Dir(s.path)
	tmp, err := os.CreateTemp(dir, ".state-*.json.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}

	return os.Rename(tmpPath, s.path)
}
