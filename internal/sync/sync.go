// Package sync orchestrates pulling Forgejo time entries into Toggl.
package sync

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"time"

	"github.com/alrayyes/forgejo-time-sync/internal/forgejo"
	"github.com/alrayyes/forgejo-time-sync/internal/state"
	"github.com/alrayyes/forgejo-time-sync/internal/toggl"
)

// ForgejoTimes is the one Forgejo capability this package needs.
type ForgejoTimes interface {
	ListRepoTimes(ctx context.Context, owner, repo string) ([]forgejo.TimeEntry, error)
}

// TogglEntries is the one Toggl capability RepoTimes needs. Note there is
// no update/delete here — RepoTimes can only ever create new entries.
type TogglEntries interface {
	CreateTimeEntry(ctx context.Context, e toggl.NewTimeEntry) (toggl.Entry, error)
}

// TogglRecentEntries is the one Toggl capability Reconcile needs.
type TogglRecentEntries interface {
	ListRecentEntries(ctx context.Context, workspaceID int64, since time.Time) ([]toggl.Entry, error)
}

// TogglProjects is the one Toggl capability ResolveProject needs.
type TogglProjects interface {
	FindOrCreateClient(ctx context.Context, workspaceID int64, name string) (int64, error)
	FindOrCreateProject(ctx context.Context, workspaceID, clientID int64, name string) (int64, error)
}

// ResolveProject returns the Toggl project ID to sync owner/repo's time
// entries into, in priority order: an explicit projectID if one was
// configured, the ID already cached in state from a previous resolution,
// or a freshly auto-provisioned project (a client named owner, a project
// named repo under it) — cached in state afterward.
//
// The cache is what makes this safe to call on every startup: once
// resolved, the ID is used as-is and never re-derived by name, so renaming
// the project later in Toggl's UI is left alone rather than fought or
// duplicated.
func ResolveProject(ctx context.Context, tg TogglProjects, st *state.State, owner, repo string, workspaceID, explicitProjectID int64) (int64, error) {
	if explicitProjectID > 0 {
		return explicitProjectID, nil
	}
	if id := st.ProjectID(); id > 0 {
		return id, nil
	}

	clientID, err := tg.FindOrCreateClient(ctx, workspaceID, owner)
	if err != nil {
		return 0, fmt.Errorf("resolving toggl client for %s: %w", owner, err)
	}
	projectID, err := tg.FindOrCreateProject(ctx, workspaceID, clientID, repo)
	if err != nil {
		return 0, fmt.Errorf("resolving toggl project for %s/%s: %w", owner, repo, err)
	}
	if err := st.SetProjectID(projectID); err != nil {
		return 0, fmt.Errorf("caching resolved toggl project id: %w", err)
	}
	return projectID, nil
}

// RepoTimes pulls owner/repo's Forgejo time entries and creates a matching
// Toggl entry for each one not already recorded in state. Safe to call
// repeatedly: an entry already in state is skipped without touching Toggl.
func RepoTimes(ctx context.Context, fg ForgejoTimes, tg TogglEntries, st *state.State, owner, repo string, togglWorkspaceID, togglProjectID int64) ([]toggl.Entry, error) {
	entries, err := fg.ListRepoTimes(ctx, owner, repo)
	if err != nil {
		return nil, fmt.Errorf("listing forgejo time entries for %s/%s: %w", owner, repo, err)
	}

	var created []toggl.Entry
	for _, e := range entries {
		if st.Has(e.ID) {
			continue
		}

		entry, err := tg.CreateTimeEntry(ctx, toggl.NewTimeEntry{
			WorkspaceID: togglWorkspaceID,
			ProjectID:   togglProjectID,
			Start:       e.Created,
			Duration:    time.Duration(e.Seconds) * time.Second,
			Description: formatDescription(e.ID, owner, repo, e.IssueNumber),
			Tags:        []string{issueRef(owner, repo, e.IssueNumber)},
		})
		if err != nil {
			return created, fmt.Errorf("syncing forgejo time entry %d: %w", e.ID, err)
		}
		if err := st.Add(e.ID); err != nil {
			return created, fmt.Errorf("recording forgejo time entry %d as synced: %w", e.ID, err)
		}
		created = append(created, entry)
	}
	return created, nil
}

// Reconcile seeds state from Toggl's own history when state is empty — a
// fresh volume, or one that was lost. It's a one-time recovery path, never
// part of the steady-state poll: without it, losing the state file would
// cause every already-synced Forgejo entry to be recreated in Toggl.
func Reconcile(ctx context.Context, tg TogglRecentEntries, st *state.State, workspaceID int64, since time.Time) error {
	if st.Len() > 0 {
		return nil
	}

	entries, err := tg.ListRecentEntries(ctx, workspaceID, since)
	if err != nil {
		return fmt.Errorf("reconciling sync state from toggl: %w", err)
	}

	for _, e := range entries {
		id, ok := parseSyncedID(e.Description)
		if !ok {
			continue
		}
		if err := st.Add(id); err != nil {
			return fmt.Errorf("recording reconciled entry %d: %w", id, err)
		}
	}
	return nil
}

const descriptionPrefix = "forgejo-time-entry:"

var syncedIDPattern = regexp.MustCompile(`^forgejo-time-entry:(\d+)`)

func formatDescription(id int64, owner, repo string, issueNumber int64) string {
	return fmt.Sprintf("%s%d issue:%s", descriptionPrefix, id, issueRef(owner, repo, issueNumber))
}

func issueRef(owner, repo string, issueNumber int64) string {
	return fmt.Sprintf("%s/%s#%d", owner, repo, issueNumber)
}

func parseSyncedID(description string) (int64, bool) {
	match := syncedIDPattern.FindStringSubmatch(description)
	if match == nil {
		return 0, false
	}
	id, err := strconv.ParseInt(match[1], 10, 64)
	if err != nil {
		return 0, false
	}
	return id, true
}
