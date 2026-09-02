// Package health is a liveness heartbeat: the poll loop touches a file
// every cycle, and a healthcheck reads it back and fails if it's gone
// stale. It tracks whether the loop is still actively cycling, not whether
// a poll's Forgejo/Toggl calls are succeeding — a real outage on either
// side should keep retrying forever, not get treated as unhealthy and
// restarted.
package health

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// errStaleHeartbeat is wrapped with the specific age/threshold that tripped
// it, so callers can errors.Is against a stable sentinel instead of
// matching on the formatted message.
var errStaleHeartbeat = errors.New("health: heartbeat is stale")

// Heartbeat is a liveness marker backed by a file's modification time.
type Heartbeat struct {
	path string
}

// HeartbeatPath keeps the heartbeat file on the same volume as the state
// file at stateFilePath, so it needs no config of its own.
func HeartbeatPath(stateFilePath string) string {
	return filepath.Join(filepath.Dir(stateFilePath), "healthcheck")
}

// MaxAge allows a couple of missed ticks — a single slow poll (a slow
// Forgejo/Toggl response, not a hang) shouldn't flap the container
// unhealthy — while still catching a genuinely stuck loop.
func MaxAge(interval time.Duration) time.Duration {
	return 3 * interval
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
	f, err := os.OpenFile(h.path, os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("health: creating heartbeat: %w", err)
	}

	if err := f.Close(); err != nil {
		return fmt.Errorf("health: closing heartbeat: %w", err)
	}

	return nil
}

// Check reports an error if the heartbeat file is missing or hasn't been
// touched within maxAge.
func (h Heartbeat) Check(maxAge time.Duration) error {
	info, err := os.Stat(h.path)
	if err != nil {
		return fmt.Errorf("health: reading heartbeat: %w", err)
	}
	if age := time.Since(info.ModTime()); age > maxAge {
		return fmt.Errorf("%w (last touched %s ago, want under %s)", errStaleHeartbeat, age.Round(time.Second), maxAge)
	}

	return nil
}
