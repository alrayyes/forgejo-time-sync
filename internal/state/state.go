// Package state tracks which Forgejo time entries have already been pushed
// to Toggl, so a poll can skip Toggl entirely when there's nothing new. It
// also caches the Toggl project a repo resolved to, once auto-provisioned,
// so a later rename in Toggl's UI never triggers a re-lookup or a
// duplicate.
package state

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// State is a Forgejo repo's sync state, backed by a JSON file on disk.
type State struct {
	path string
	mu   sync.Mutex
	ids  map[int64]struct{}

	projectID int64
}

// fileFormat is the on-disk shape of the state file.
type fileFormat struct {
	IDs       []int64 `json:"ids"`
	ProjectID int64   `json:"project_id,omitempty"`
}

// Load reads the state file at path, or starts empty if it doesn't exist
// yet.
func Load(path string) (*State, error) {
	s := &State{path: path, ids: make(map[int64]struct{})}

	data, err := os.ReadFile(path) //nolint:gosec // path is operator-provided config (STATE_FILE_PATH), not untrusted input
	if os.IsNotExist(err) {
		return s, nil
	}
	if err != nil {
		return nil, fmt.Errorf("state: reading %s: %w", path, err)
	}

	var f fileFormat
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("state: parsing %s: %w", path, err)
	}
	for _, id := range f.IDs {
		s.ids[id] = struct{}{}
	}
	s.projectID = f.ProjectID

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

// ProjectID returns the cached Toggl project ID this repo resolved to, or 0
// if none has been resolved yet.
func (s *State) ProjectID() int64 {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.projectID
}

// SetProjectID caches the Toggl project ID this repo resolved to and
// persists it. Once set, it's used as-is — never re-derived by name — so a
// project renamed later in Toggl's UI is left alone rather than fought or
// duplicated.
func (s *State) SetProjectID(id int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.projectID = id

	return s.saveLocked()
}

func (s *State) saveLocked() error {
	f := fileFormat{IDs: make([]int64, 0, len(s.ids)), ProjectID: s.projectID}
	for id := range s.ids {
		f.IDs = append(f.IDs, id)
	}

	data, err := json.Marshal(f)
	if err != nil {
		return fmt.Errorf("state: encoding: %w", err)
	}

	dir := filepath.Dir(s.path)
	tmp, err := os.CreateTemp(dir, ".state-*.json.tmp")
	if err != nil {
		return fmt.Errorf("state: creating temp file: %w", err)
	}
	tmpPath := tmp.Name()

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)

		return fmt.Errorf("state: writing temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)

		return fmt.Errorf("state: closing temp file: %w", err)
	}

	if err := os.Rename(tmpPath, s.path); err != nil {
		return fmt.Errorf("state: replacing %s: %w", s.path, err)
	}

	return nil
}
