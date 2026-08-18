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
	received []toggl.NewTimeEntry
	err      error

	recent    []toggl.Entry
	recentErr error

	clientID        int64
	projectID       int64
	findOrCreateErr error
	clientCalls     []string
	projectCalls    []findOrCreateProjectCall
}

type findOrCreateProjectCall struct {
	clientID int64
	name     string
}

func (f *fakeToggl) FindOrCreateClient(_ context.Context, _ int64, name string) (int64, error) {
	f.clientCalls = append(f.clientCalls, name)
	if f.findOrCreateErr != nil {
		return 0, f.findOrCreateErr
	}
	return f.clientID, nil
}

func (f *fakeToggl) FindOrCreateProject(_ context.Context, _, clientID int64, name string) (int64, error) {
	f.projectCalls = append(f.projectCalls, findOrCreateProjectCall{clientID: clientID, name: name})
	if f.findOrCreateErr != nil {
		return 0, f.findOrCreateErr
	}
	return f.projectID, nil
}

func (f *fakeToggl) CreateTimeEntry(_ context.Context, e toggl.NewTimeEntry) (toggl.Entry, error) {
	if f.err != nil {
		return toggl.Entry{}, f.err
	}
	f.received = append(f.received, e)
	return toggl.Entry{
		ID:          int64(len(f.received)),
		WorkspaceID: e.WorkspaceID,
		ProjectID:   e.ProjectID,
		Description: e.Description,
		Duration:    int64(e.Duration.Seconds()),
	}, nil
}

func (f *fakeToggl) ListRecentEntries(context.Context, int64, time.Time) ([]toggl.Entry, error) {
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

	t.Run("tags the entry with the issue reference as structured metadata", func(t *testing.T) {
		require.Equal(t, []string{"alrayyes/repo#12"}, tg.received[0].Tags)
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
		require.Len(t, tg.received, 1)
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
		require.Len(t, tg.received, 1)
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

	err := sync.Reconcile(t.Context(), tg, st, 100, time.Now().Add(-24*time.Hour))
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

	err := sync.Reconcile(t.Context(), tg, st, 100, time.Now().Add(-24*time.Hour))
	require.NoError(t, err)

	require.False(t, st.Has(5), "reconcile should only ever seed an empty state, never merge into an existing one")
}

func TestResolveProjectPrefersAnExplicitID(t *testing.T) {
	tg := &fakeToggl{clientID: 7, projectID: 21}
	st := newState(t)

	id, err := sync.ResolveProject(t.Context(), tg, st, "alrayyes", "repo", 1, 99)
	require.NoError(t, err)

	t.Run("returns the explicit id unchanged", func(t *testing.T) {
		require.Equal(t, int64(99), id)
	})

	t.Run("never touches toggl at all", func(t *testing.T) {
		require.Empty(t, tg.clientCalls)
		require.Empty(t, tg.projectCalls)
	})
}

func TestResolveProjectAutoProvisionsWhenNoIDIsGiven(t *testing.T) {
	tg := &fakeToggl{clientID: 7, projectID: 21}
	st := newState(t)

	id, err := sync.ResolveProject(t.Context(), tg, st, "alrayyes", "repo", 1, 0)
	require.NoError(t, err)

	t.Run("returns the auto-provisioned project id", func(t *testing.T) {
		require.Equal(t, int64(21), id)
	})

	t.Run("creates the client named after the repo owner", func(t *testing.T) {
		require.Equal(t, []string{"alrayyes"}, tg.clientCalls)
	})

	t.Run("creates the project named after the repo, under that client", func(t *testing.T) {
		require.Equal(t, []findOrCreateProjectCall{{clientID: 7, name: "repo"}}, tg.projectCalls)
	})

	t.Run("caches the resolved id in state", func(t *testing.T) {
		require.Equal(t, int64(21), st.ProjectID())
	})
}

func TestResolveProjectReusesTheCachedIDWithoutTouchingToggl(t *testing.T) {
	tg := &fakeToggl{clientID: 7, projectID: 21}
	st := newState(t)
	require.NoError(t, st.SetProjectID(555))

	id, err := sync.ResolveProject(t.Context(), tg, st, "alrayyes", "repo", 1, 0)
	require.NoError(t, err)

	t.Run("returns the cached id, not a freshly resolved one", func(t *testing.T) {
		require.Equal(t, int64(555), id)
	})

	t.Run("never re-looks-up by name, so a rename in Toggl's UI is left alone", func(t *testing.T) {
		require.Empty(t, tg.clientCalls)
		require.Empty(t, tg.projectCalls)
	})
}

func TestResolveProjectPropagatesErrors(t *testing.T) {
	tg := &fakeToggl{findOrCreateErr: errors.New("boom")}
	st := newState(t)

	_, err := sync.ResolveProject(t.Context(), tg, st, "alrayyes", "repo", 1, 0)
	require.Error(t, err)

	require.Zero(t, st.ProjectID(), "a failed resolution should not cache a zero/partial id")
}
