// Package health is a liveness heartbeat: the poll loop touches a file
// every cycle, and a healthcheck reads it back and fails if it's gone
// stale. It tracks whether the loop is still actively cycling, not whether
// a poll's Forgejo/Toggl calls are succeeding — a real outage on either
// side should keep retrying forever, not get treated as unhealthy and
// restarted.
package health

import (
	"fmt"
	"os"
	"time"
)

// Heartbeat is a liveness marker backed by a file's modification time.
type Heartbeat struct {
	path string
}

// NewHeartbeat returns a Heartbeat backed by the file at path.
func NewHeartbeat(path string) Heartbeat {
	return Heartbeat{path: path}
}

// Touch records that the caller is alive right now.
func (h Heartbeat) Touch() error {
	now := time.Now()
	if err := os.Chtimes(h.path, now, now); err == nil {
		return nil
	}
	f, err := os.OpenFile(h.path, os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	return f.Close()
}

// Check reports an error if the heartbeat file is missing or hasn't been
// touched within maxAge.
func (h Heartbeat) Check(maxAge time.Duration) error {
	info, err := os.Stat(h.path)
	if err != nil {
		return fmt.Errorf("health: reading heartbeat: %w", err)
	}
	if age := time.Since(info.ModTime()); age > maxAge {
		return fmt.Errorf("health: heartbeat is stale (last touched %s ago, want under %s)", age.Round(time.Second), maxAge)
	}
	return nil
}
