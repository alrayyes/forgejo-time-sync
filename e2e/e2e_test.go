//go:build e2e

// Package e2e is the container/integration layer: a real Forgejo
// (testcontainers) and a Prism mock server serving Toggl's own vendored
// OpenAPI spec, so requests get validated against Toggl's real contract
// without depending on Toggl's uptime or rate limit in CI. Run with:
//
//	go test -tags e2e ./e2e/...
package e2e

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/alrayyes/forgejo-time-sync/internal/forgejo"
	"github.com/alrayyes/forgejo-time-sync/internal/state"
	"github.com/alrayyes/forgejo-time-sync/internal/sync"
	"github.com/alrayyes/forgejo-time-sync/internal/toggl"
)

const (
	togglWorkspaceID = 100
	togglProjectID   = 200
)

func newSyncFixture(t *testing.T, ctx context.Context) (fg *forgejo.Client, fgFixture forgejoFixture, tg *toggl.Client, proxy *recordingProxy, st *state.State) {
	t.Helper()

	fgFixture = startForgejo(t, ctx)
	var err error
	fg, err = forgejo.NewClient(fgFixture.baseURL, fgFixture.token)
	require.NoError(t, err)

	prismURL := startPrismMock(t, ctx)
	proxy = newRecordingProxy(t, prismURL)
	tg = toggl.NewClient("test-token", 3600)
	tg.BaseURL = proxy.URL()

	st, err = state.Load(filepath.Join(t.TempDir(), "state.json"))
	require.NoError(t, err)

	return fg, fgFixture, tg, proxy, st
}

func TestSyncPushesForgejoTimeIntoToggl(t *testing.T) {
	ctx := t.Context()
	fg, fgFixture, tg, proxy, st := newSyncFixture(t, ctx)

	fgFixture.createRepoWithTrackedTime(t, "e2e-repo", 5400)

	created, err := sync.RepoTimes(ctx, fg, tg, st, forgejoAdmin, "e2e-repo", togglWorkspaceID, togglProjectID)
	require.NoError(t, err)

	t.Run("one entry gets created", func(t *testing.T) {
		require.Len(t, created, 1)
	})

	t.Run("state records the forgejo entry as synced", func(t *testing.T) {
		require.Equal(t, 1, st.Len())
	})

	t.Run("prism validated exactly one request against the real toggl contract", func(t *testing.T) {
		require.Len(t, proxy.Requests(), 1)
	})

	t.Run("the only request made was a POST to create a time entry", func(t *testing.T) {
		// This is the running-timer-safety guarantee made concrete: nothing
		// in this tool can start, stop, or modify an existing Toggl entry,
		// because it never sends anything but this one request shape.
		req := proxy.Requests()[0]
		require.Equal(t, "POST", req.Method)
		require.Equal(t, "/api/v9/workspaces/100/time_entries", req.Path)
	})
}

func TestSyncIsIdempotentOnRerun(t *testing.T) {
	ctx := t.Context()
	fg, fgFixture, tg, proxy, st := newSyncFixture(t, ctx)

	fgFixture.createRepoWithTrackedTime(t, "e2e-repo", 3600)

	_, err := sync.RepoTimes(ctx, fg, tg, st, forgejoAdmin, "e2e-repo", togglWorkspaceID, togglProjectID)
	require.NoError(t, err)

	created, err := sync.RepoTimes(ctx, fg, tg, st, forgejoAdmin, "e2e-repo", togglWorkspaceID, togglProjectID)
	require.NoError(t, err)

	t.Run("the rerun creates nothing new", func(t *testing.T) {
		require.Empty(t, created)
	})

	t.Run("toggl only ever saw one create request across both runs", func(t *testing.T) {
		require.Len(t, proxy.Requests(), 1)
	})
}

func TestSyncAutoProvisionsTogglProjectWhenNoneConfigured(t *testing.T) {
	ctx := t.Context()
	_, _, tg, proxy, st := newSyncFixture(t, ctx)

	projectID, err := sync.ResolveProject(ctx, tg, st, "alrayyes", "e2e-repo", togglWorkspaceID, 0)
	require.NoError(t, err)

	t.Run("resolves to some project id, validated against toggl's real client/project contract", func(t *testing.T) {
		require.NotZero(t, projectID)
	})

	t.Run("caches the resolved id in state", func(t *testing.T) {
		require.Equal(t, projectID, st.ProjectID())
	})

	firstPassRequests := len(proxy.Requests())

	t.Run("resolving again reuses the cached id without touching toggl again", func(t *testing.T) {
		again, err := sync.ResolveProject(ctx, tg, st, "alrayyes", "e2e-repo", togglWorkspaceID, 0)
		require.NoError(t, err)
		require.Equal(t, projectID, again)
		require.Len(t, proxy.Requests(), firstPassRequests, "no new requests should have been made")
	})
}
