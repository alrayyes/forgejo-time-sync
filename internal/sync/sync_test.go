package sync_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/alrayyes/forgejo-time-sync/internal/forgejo"
	"github.com/alrayyes/forgejo-time-sync/internal/state"
	"github.com/alrayyes/forgejo-time-sync/internal/sync"
	"github.com/alrayyes/forgejo-time-sync/internal/toggl"
)

type fakeForgejo struct {
	entries []forgejo.TimeEntry
	err     error
}

func (f fakeForgejo) ListRepoTimes(context.Context, string, string) ([]forgejo.TimeEntry, error) {
	return f.entries, f.err
}

type fakeToggl struct {
	created []toggl.Entry
	err     error

	recent    []toggl.Entry
	recentErr error
}

func (f *fakeToggl) CreateTimeEntry(_ context.Context, workspaceID, projectID int64, _ time.Time, duration time.Duration, description string) (toggl.Entry, error) {
	if f.err != nil {
		return toggl.Entry{}, f.err
	}
	e := toggl.Entry{
		ID:          int64(len(f.created) + 1),
		WorkspaceID: workspaceID,
		ProjectID:   projectID,
		Description: description,
		Duration:    int64(duration.Seconds()),
	}
	f.created = append(f.created, e)
	return e, nil
}

func (f *fakeToggl) ListRecentEntries(context.Context, time.Time) ([]toggl.Entry, error) {
	return f.recent, f.recentErr
}

func newState(t *testing.T) *state.State {
	t.Helper()
	s, err := state.Load(filepath.Join(t.TempDir(), "state.json"))
	require.NoError(t, err)
	return s
}

func TestRepoTimesCreatesNewEntries(t *testing.T) {
	fg := fakeForgejo{entries: []forgejo.TimeEntry{
		{ID: 1, Created: time.Date(2026, 8, 17, 9, 0, 0, 0, time.UTC), Seconds: 5400, IssueNumber: 12},
	}}
	tg := &fakeToggl{}
	st := newState(t)

	created, err := sync.RepoTimes(t.Context(), fg, tg, st, "alrayyes", "repo", 100, 200)
	require.NoError(t, err)
	require.Len(t, created, 1)

	t.Run("passes the exact duration through with no rounding", func(t *testing.T) {
		require.Equal(t, int64(5400), created[0].Duration)
	})

	t.Run("embeds the forgejo entry id and issue reference in the description", func(t *testing.T) {
		require.Equal(t, "forgejo-time-entry:1 issue:alrayyes/repo#12", created[0].Description)
	})

	t.Run("records the entry as synced", func(t *testing.T) {
		require.True(t, st.Has(1))
	})
}

func TestRepoTimesSkipsAlreadySyncedEntries(t *testing.T) {
	fg := fakeForgejo{entries: []forgejo.TimeEntry{
		{ID: 1, Created: time.Now(), Seconds: 60, IssueNumber: 1},
		{ID: 2, Created: time.Now(), Seconds: 60, IssueNumber: 1},
	}}
	tg := &fakeToggl{}
	st := newState(t)
	require.NoError(t, st.Add(1))

	created, err := sync.RepoTimes(t.Context(), fg, tg, st, "alrayyes", "repo", 100, 200)
	require.NoError(t, err)

	t.Run("only the unsynced entry is returned", func(t *testing.T) {
		require.Len(t, created, 1)
	})

	t.Run("only the unsynced entry reached Toggl", func(t *testing.T) {
		require.Len(t, tg.created, 1)
	})
}

func TestRepoTimesIsIdempotentOnRerun(t *testing.T) {
	fg := fakeForgejo{entries: []forgejo.TimeEntry{
		{ID: 1, Created: time.Now(), Seconds: 60, IssueNumber: 1},
	}}
	tg := &fakeToggl{}
	st := newState(t)
	_, err := sync.RepoTimes(t.Context(), fg, tg, st, "alrayyes", "repo", 100, 200)
	require.NoError(t, err)

	created, err := sync.RepoTimes(t.Context(), fg, tg, st, "alrayyes", "repo", 100, 200)
	require.NoError(t, err)

	t.Run("the second run creates nothing new", func(t *testing.T) {
		require.Empty(t, created)
	})

	t.Run("Toggl only ever saw one create call across both runs", func(t *testing.T) {
		require.Len(t, tg.created, 1)
	})
}

func TestRepoTimesStopsOnTogglError(t *testing.T) {
	fg := fakeForgejo{entries: []forgejo.TimeEntry{
		{ID: 1, Created: time.Now(), Seconds: 60, IssueNumber: 1},
	}}
	tg := &fakeToggl{err: errors.New("boom")}
	st := newState(t)

	_, err := sync.RepoTimes(t.Context(), fg, tg, st, "alrayyes", "repo", 100, 200)
	require.Error(t, err)

	require.False(t, st.Has(1), "an entry that failed to sync should not be recorded as synced")
}

func TestReconcileSeedsStateFromParseableDescriptions(t *testing.T) {
	tg := &fakeToggl{recent: []toggl.Entry{
		{ID: 10, Description: "forgejo-time-entry:5 issue:alrayyes/repo#1"},
		{ID: 11, Description: "unrelated manual entry"},
	}}
	st := newState(t)

	err := sync.Reconcile(t.Context(), tg, st, time.Now().Add(-24*time.Hour))
	require.NoError(t, err)

	t.Run("the parseable entry's forgejo id is recorded", func(t *testing.T) {
		require.True(t, st.Has(5))
	})

	t.Run("nothing else gets recorded from the unparseable entry", func(t *testing.T) {
		require.Equal(t, 1, st.Len())
	})
}

func TestReconcileIsANoOpWhenStateIsAlreadyPopulated(t *testing.T) {
	tg := &fakeToggl{recent: []toggl.Entry{
		{ID: 10, Description: "forgejo-time-entry:5"},
	}}
	st := newState(t)
	require.NoError(t, st.Add(1))

	err := sync.Reconcile(t.Context(), tg, st, time.Now().Add(-24*time.Hour))
	require.NoError(t, err)

	require.False(t, st.Has(5), "reconcile should only ever seed an empty state, never merge into an existing one")
}
