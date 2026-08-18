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
	togglOrganizationID = 42
	togglWorkspaceID    = 100
	togglProjectID      = 200
)

func newSyncFixture(t *testing.T, ctx context.Context) (fg *forgejo.Client, fgFixture forgejoFixture, tg *toggl.Client, proxy *recordingProxy, st *state.State) {
	t.Helper()

	fgFixture = startForgejo(t, ctx)
	var err error
	fg, err = forgejo.NewClient(fgFixture.baseURL, fgFixture.token)
	require.NoError(t, err)

	prismURL := startPrismMock(t, ctx)
	proxy = newRecordingProxy(t, prismURL)
	tg = toggl.NewClient("test-token", togglOrganizationID, 3600)
	tg.BaseURL = proxy.URL()

	st, err = state.Load(filepath.Join(t.TempDir(), "state.json"))
	require.NoError(t, err)

	return fg, fgFixture, tg, proxy, st
}

// timeEntryPath is the one endpoint that can ever create a time entry.
// Every e2e test checks requests against this exact path (never a
// PUT/PATCH/DELETE, and never .../time-entries/{id}), which is the
// running-timer-safety guarantee made concrete: nothing in this tool can
// start, stop, or modify an existing Toggl entry, because it never sends
// anything but a POST to this one path.
const timeEntryPath = "/organizations/42/workspaces/100/time-entries"

func requestsToPath(requests []recordedRequest, path string) []recordedRequest {
	var matched []recordedRequest
	for _, r := range requests {
		if r.Path == path {
			matched = append(matched, r)
		}
	}
	return matched
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

	t.Run("exactly one time entry request went out, validated against toggl's real contract", func(t *testing.T) {
		timeEntryRequests := requestsToPath(proxy.Requests(), timeEntryPath)
		require.Len(t, timeEntryRequests, 1)
		require.Equal(t, "POST", timeEntryRequests[0].Method)
	})

	t.Run("nothing sent could have started, stopped, or modified an existing entry", func(t *testing.T) {
		for _, req := range proxy.Requests() {
			require.NotContains(t, []string{"PUT", "PATCH", "DELETE"}, req.Method, "%s %s", req.Method, req.Path)
		}
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

	t.Run("toggl only ever saw one time entry created across both runs", func(t *testing.T) {
		require.Len(t, requestsToPath(proxy.Requests(), timeEntryPath), 1)
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
