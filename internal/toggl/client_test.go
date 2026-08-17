package toggl_test

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/alrayyes/forgejo-time-sync/internal/toggl"
)

// countingWaiter is a fake ratelimit.Pacer: it records how many times Wait
// was called, without any real timing, so client-level tests only assert
// that the client paces itself — the pacing math itself is ratelimit's own
// responsibility and is tested there.
type countingWaiter struct{ calls int }

func (w *countingWaiter) Wait() { w.calls++ }

func newTestClient(t *testing.T, waiter *countingWaiter, handler http.HandlerFunc) *toggl.Client {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	c := toggl.NewClientWithPacer("test-token", waiter)
	c.BaseURL = srv.URL
	return c
}

func TestCreateTimeEntry(t *testing.T) {
	var gotMethod, gotPath, gotAuth string
	var gotBody map[string]any

	waiter := &countingWaiter{}
	c := newTestClient(t, waiter, func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		_ = json.NewDecoder(r.Body).Decode(&gotBody)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id": 99, "workspace_id": 1, "project_id": 2, "description": "d", "duration": 5400}`))
	})

	start := time.Date(2026, 8, 17, 9, 0, 0, 0, time.UTC)
	entry, err := c.CreateTimeEntry(t.Context(), toggl.NewTimeEntry{
		WorkspaceID: 1,
		ProjectID:   2,
		Start:       start,
		Duration:    90 * time.Minute,
		Description: "forgejo-time-entry:1 issue:alrayyes/repo#12",
		Tags:        []string{"alrayyes/repo#12"},
	})
	require.NoError(t, err)

	t.Run("sends a POST", func(t *testing.T) {
		require.Equal(t, http.MethodPost, gotMethod)
	})

	t.Run("hits the workspace time_entries endpoint", func(t *testing.T) {
		require.Equal(t, "/api/v9/workspaces/1/time_entries", gotPath)
	})

	t.Run("authenticates with the token as the basic-auth username", func(t *testing.T) {
		wantAuth := "Basic " + base64.StdEncoding.EncodeToString([]byte("test-token:api_token"))
		require.Equal(t, wantAuth, gotAuth)
	})

	t.Run("sends the workspace and project ids", func(t *testing.T) {
		require.Equal(t, float64(1), gotBody["workspace_id"])
		require.Equal(t, float64(2), gotBody["project_id"])
	})

	t.Run("sends the exact duration in seconds, with no rounding", func(t *testing.T) {
		require.Equal(t, float64(5400), gotBody["duration"])
	})

	t.Run("tags the request as coming from this tool", func(t *testing.T) {
		require.Equal(t, "forgejo-time-sync", gotBody["created_with"])
	})

	t.Run("passes the description through unchanged", func(t *testing.T) {
		require.Equal(t, "forgejo-time-entry:1 issue:alrayyes/repo#12", gotBody["description"])
	})

	t.Run("sends the issue reference as a tag, not just in the description", func(t *testing.T) {
		require.Equal(t, []any{"alrayyes/repo#12"}, gotBody["tags"])
	})

	t.Run("paces itself before the request", func(t *testing.T) {
		require.Equal(t, 1, waiter.calls)
	})

	t.Run("returns the created entry's id", func(t *testing.T) {
		require.Equal(t, int64(99), entry.ID)
	})
}

func TestCreateTimeEntryPropagatesHardErrors(t *testing.T) {
	c := newTestClient(t, &countingWaiter{}, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
	})

	_, err := c.CreateTimeEntry(t.Context(), toggl.NewTimeEntry{WorkspaceID: 1, ProjectID: 2, Start: time.Now(), Duration: time.Minute, Description: "d"})

	require.Error(t, err)
}

func TestCreateTimeEntryRetriesOn402ThenSucceeds(t *testing.T) {
	attempts := 0
	waiter := &countingWaiter{}
	c := newTestClient(t, waiter, func(w http.ResponseWriter, _ *http.Request) {
		attempts++
		if attempts < 3 {
			w.WriteHeader(http.StatusPaymentRequired)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id": 1}`))
	})

	entry, err := c.CreateTimeEntry(t.Context(), toggl.NewTimeEntry{WorkspaceID: 1, ProjectID: 2, Start: time.Now(), Duration: time.Minute, Description: "d"})
	require.NoError(t, err)

	t.Run("eventually returns the entry created on the successful attempt", func(t *testing.T) {
		require.Equal(t, int64(1), entry.ID)
	})

	t.Run("retries until the rate limit clears", func(t *testing.T) {
		require.Equal(t, 3, attempts)
	})

	t.Run("paces itself before every attempt, not just the first", func(t *testing.T) {
		require.Equal(t, 3, waiter.calls)
	})
}

func TestCreateTimeEntryGivesUpAfterRepeated402(t *testing.T) {
	c := newTestClient(t, &countingWaiter{}, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusPaymentRequired)
	})

	_, err := c.CreateTimeEntry(t.Context(), toggl.NewTimeEntry{WorkspaceID: 1, ProjectID: 2, Start: time.Now(), Duration: time.Minute, Description: "d"})

	require.Error(t, err)
}

func TestFindOrCreateClientFindsAnExistingClientByName(t *testing.T) {
	var postCount int

	c := newTestClient(t, &countingWaiter{}, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			postCount++
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"id": 7, "name": "alrayyes"}, {"id": 8, "name": "someone-else"}]`))
	})

	id, err := c.FindOrCreateClient(t.Context(), 1, "alrayyes")
	require.NoError(t, err)

	t.Run("returns the matching client's id", func(t *testing.T) {
		require.Equal(t, int64(7), id)
	})

	t.Run("never creates a duplicate", func(t *testing.T) {
		require.Zero(t, postCount)
	})
}

func TestFindOrCreateClientCreatesWhenNoneMatch(t *testing.T) {
	var gotMethod, gotPath string
	var gotBody map[string]any

	c := newTestClient(t, &countingWaiter{}, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet {
			_, _ = w.Write([]byte(`[]`))
			return
		}
		gotMethod = r.Method
		gotPath = r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		_, _ = w.Write([]byte(`{"id": 9, "name": "alrayyes"}`))
	})

	id, err := c.FindOrCreateClient(t.Context(), 1, "alrayyes")
	require.NoError(t, err)

	t.Run("returns the newly created client's id", func(t *testing.T) {
		require.Equal(t, int64(9), id)
	})

	t.Run("creates it with a POST to the clients endpoint", func(t *testing.T) {
		require.Equal(t, http.MethodPost, gotMethod)
		require.Equal(t, "/api/v9/workspaces/1/clients", gotPath)
	})

	t.Run("sends the requested name", func(t *testing.T) {
		require.Equal(t, "alrayyes", gotBody["name"])
	})
}

func TestFindOrCreateProjectFindsAnExistingProjectByNameAndClient(t *testing.T) {
	var postCount int

	c := newTestClient(t, &countingWaiter{}, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			postCount++
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[
			{"id": 20, "name": "repo", "client_id": 999},
			{"id": 21, "name": "repo", "client_id": 7}
		]`))
	})

	id, err := c.FindOrCreateProject(t.Context(), 1, 7, "repo")
	require.NoError(t, err)

	t.Run("returns the project matching both name and client", func(t *testing.T) {
		require.Equal(t, int64(21), id)
	})

	t.Run("never creates a duplicate", func(t *testing.T) {
		require.Zero(t, postCount)
	})
}

func TestFindOrCreateProjectCreatesWhenNoneMatch(t *testing.T) {
	var gotBody map[string]any

	c := newTestClient(t, &countingWaiter{}, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet {
			_, _ = w.Write([]byte(`[]`))
			return
		}
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		_, _ = w.Write([]byte(`{"id": 22, "name": "repo", "client_id": 7}`))
	})

	id, err := c.FindOrCreateProject(t.Context(), 1, 7, "repo")
	require.NoError(t, err)

	t.Run("returns the newly created project's id", func(t *testing.T) {
		require.Equal(t, int64(22), id)
	})

	t.Run("creates it under the given client", func(t *testing.T) {
		require.Equal(t, "repo", gotBody["name"])
		require.Equal(t, float64(7), gotBody["client_id"])
	})
}

func TestStartTimer(t *testing.T) {
	var gotMethod, gotPath string
	var gotBody map[string]any

	c := newTestClient(t, &countingWaiter{}, func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&gotBody)

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id": 55, "workspace_id": 1, "project_id": 2}`))
	})

	start := time.Date(2026, 8, 17, 9, 0, 0, 0, time.UTC)
	entry, err := c.StartTimer(t.Context(), toggl.NewRunningTimer{
		WorkspaceID: 1,
		ProjectID:   2,
		Start:       start,
		Description: "d",
		Tags:        []string{"alrayyes/repo#12"},
	})
	require.NoError(t, err)

	t.Run("sends a POST to create a time entry", func(t *testing.T) {
		require.Equal(t, http.MethodPost, gotMethod)
		require.Equal(t, "/api/v9/workspaces/1/time_entries", gotPath)
	})

	t.Run("sends a negative duration, marking it as still running", func(t *testing.T) {
		require.Equal(t, float64(-1), gotBody["duration"])
	})

	t.Run("sends the start time and tags", func(t *testing.T) {
		require.Equal(t, "2026-08-17T09:00:00Z", gotBody["start"])
		require.Equal(t, []any{"alrayyes/repo#12"}, gotBody["tags"])
	})

	t.Run("returns the created entry's id, for a later StopTimer call", func(t *testing.T) {
		require.Equal(t, int64(55), entry.ID)
	})
}

func TestStopTimer(t *testing.T) {
	var gotMethod, gotPath string
	var gotBody map[string]any

	c := newTestClient(t, &countingWaiter{}, func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&gotBody)

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id": 55, "workspace_id": 1, "project_id": 2, "duration": 5400}`))
	})

	start := time.Date(2026, 8, 17, 9, 0, 0, 0, time.UTC)
	stop := start.Add(90 * time.Minute)
	entry, err := c.StopTimer(t.Context(), 1, 55, start, stop)
	require.NoError(t, err)

	t.Run("sends a PUT to the specific entry it was told to stop", func(t *testing.T) {
		require.Equal(t, http.MethodPut, gotMethod)
		require.Equal(t, "/api/v9/workspaces/1/time_entries/55", gotPath)
	})

	t.Run("sends the exact elapsed duration and stop time, consistent with start", func(t *testing.T) {
		require.Equal(t, float64(5400), gotBody["duration"])
		require.Equal(t, "2026-08-17T09:00:00Z", gotBody["start"])
		require.Equal(t, "2026-08-17T10:30:00Z", gotBody["stop"])
	})

	t.Run("returns the finalized entry", func(t *testing.T) {
		require.Equal(t, int64(5400), entry.Duration)
	})
}

func TestListRecentEntries(t *testing.T) {
	var gotPath string

	c := newTestClient(t, &countingWaiter{}, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.String()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[
			{"id": 1, "description": "forgejo-time-entry:5", "duration": 60},
			{"id": 2, "description": "unrelated", "duration": 120}
		]`))
	})

	since := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	entries, err := c.ListRecentEntries(t.Context(), since)
	require.NoError(t, err)

	t.Run("hits the me/time_entries endpoint", func(t *testing.T) {
		require.Contains(t, gotPath, "/api/v9/me/time_entries?")
	})

	t.Run("returns every entry the server sent", func(t *testing.T) {
		require.Len(t, entries, 2)
	})
}
