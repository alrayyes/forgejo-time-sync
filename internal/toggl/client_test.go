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
	entry, err := c.CreateTimeEntry(t.Context(), 1, 2, start, 90*time.Minute, "forgejo-time-entry:1 issue:alrayyes/repo#12")
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

	_, err := c.CreateTimeEntry(t.Context(), 1, 2, time.Now(), time.Minute, "d")

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

	entry, err := c.CreateTimeEntry(t.Context(), 1, 2, time.Now(), time.Minute, "d")
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

	_, err := c.CreateTimeEntry(t.Context(), 1, 2, time.Now(), time.Minute, "d")

	require.Error(t, err)
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
