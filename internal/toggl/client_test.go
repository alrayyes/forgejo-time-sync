package toggl_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/alrayyes/forgejo-time-sync/internal/toggl"
	"github.com/stretchr/testify/require"
)

// countingWaiter is a fake ratelimit.Pacer: it records how many times Wait
// was called, without any real timing, so client-level tests only assert
// that the client paces itself — the pacing math itself is ratelimit's own
// responsibility and is tested there.
type countingWaiter struct{ calls int }

func (w *countingWaiter) Wait() { w.calls++ }

const testOrganizationID = 42

func newTestClient(t *testing.T, waiter *countingWaiter, handler http.HandlerFunc) *toggl.Client {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	c := toggl.NewClientWithPacer("test-token", testOrganizationID, waiter)
	c.BaseURL = srv.URL

	return c
}

// noTagsHandler wraps handler with a guard that fails the test if the tag
// list/create endpoints are ever hit — for tests that pass no Tags and so
// should never resolve any.
func noTagsHandler(t *testing.T, handler http.HandlerFunc) http.HandlerFunc {
	t.Helper()

	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/workspaces/1/tags" {
			t.Fatalf("unexpected tag request: %s %s", r.Method, r.URL.Path)
		}
		handler(w, r)
	}
}

// createTimeEntryResult is what TestCreateTimeEntry's subtests assert
// against — captured here so the arrange/act step isn't repeated per
// subtest and doesn't count against any one test function's own length.
type createTimeEntryResult struct {
	method, path, auth string
	body               map[string]any
	entry              toggl.Entry
	waiter             *countingWaiter
}

func createTimeEntry(t *testing.T) createTimeEntryResult {
	t.Helper()

	var gotMethod, gotPath, gotAuth string
	var gotBody map[string]any

	waiter := &countingWaiter{}
	c := newTestClient(t, waiter, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/workspaces/1/tags":
			_, _ = w.Write([]byte(`{"data": [{"id": 3, "name": "alrayyes/repo#12"}], "page": 1, "per_page": 50, "total": 1}`))
		case r.Method == http.MethodPost && r.URL.Path == "/organizations/42/workspaces/1/time-entries":
			gotMethod = r.Method
			gotPath = r.URL.Path
			gotAuth = r.Header.Get("Authorization")
			_ = json.NewDecoder(r.Body).Decode(&gotBody)
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"id": 99, "workspace_id": 1, "project_id": 2, "description": "d", "duration": 5400}`))
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
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

	return createTimeEntryResult{
		method: gotMethod, path: gotPath, auth: gotAuth, body: gotBody,
		entry: entry, waiter: waiter,
	}
}

func TestCreateTimeEntry(t *testing.T) {
	r := createTimeEntry(t)

	t.Run("sends a POST", func(t *testing.T) {
		require.Equal(t, http.MethodPost, r.method)
	})

	t.Run("hits the organization/workspace time-entries endpoint", func(t *testing.T) {
		require.Equal(t, "/organizations/42/workspaces/1/time-entries", r.path)
	})

	t.Run("authenticates with a bearer token", func(t *testing.T) {
		require.Equal(t, "Bearer test-token", r.auth)
	})

	t.Run("marks it as a taskless activity entry", func(t *testing.T) {
		require.Equal(t, "activity", r.body["type"])
	})

	t.Run("sends the project id", func(t *testing.T) {
		require.InDelta(t, float64(2), r.body["project_id"], 0)
	})

	t.Run("sends the exact duration in seconds, with no rounding", func(t *testing.T) {
		require.InDelta(t, float64(5400), r.body["duration"], 0)
	})

	t.Run("passes the description through unchanged", func(t *testing.T) {
		require.Equal(t, "forgejo-time-entry:1 issue:alrayyes/repo#12", r.body["description"])
	})

	t.Run("resolves the issue reference to a tag id, not a tag name", func(t *testing.T) {
		require.Equal(t, []any{float64(3)}, r.body["tag_ids"])
	})

	t.Run("paces itself before every request, including the tag lookup", func(t *testing.T) {
		require.Equal(t, 2, r.waiter.calls)
	})

	t.Run("returns the created entry's id", func(t *testing.T) {
		require.Equal(t, int64(99), r.entry.ID)
	})
}

func TestCreateTimeEntryCreatesAMissingTag(t *testing.T) {
	var gotTagBody map[string]any

	c := newTestClient(t, &countingWaiter{}, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/workspaces/1/tags":
			_, _ = w.Write([]byte(`{"data": [], "page": 1, "per_page": 50, "total": 0}`))
		case r.Method == http.MethodPost && r.URL.Path == "/workspaces/1/tags":
			_ = json.NewDecoder(r.Body).Decode(&gotTagBody)
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"id": 5, "name": "alrayyes/repo#12"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/organizations/42/workspaces/1/time-entries":
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"id": 99}`))
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})

	_, err := c.CreateTimeEntry(t.Context(), toggl.NewTimeEntry{
		WorkspaceID: 1, ProjectID: 2, Start: time.Now(), Duration: time.Minute,
		Description: "d", Tags: []string{"alrayyes/repo#12"},
	})
	require.NoError(t, err)

	t.Run("creates the tag with a name and a color, since color is required", func(t *testing.T) {
		require.Equal(t, "alrayyes/repo#12", gotTagBody["name"])
		require.NotEmpty(t, gotTagBody["color"])
	})
}

func TestCreateTimeEntryPropagatesHardErrors(t *testing.T) {
	c := newTestClient(t, &countingWaiter{}, noTagsHandler(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
	}))

	_, err := c.CreateTimeEntry(t.Context(), toggl.NewTimeEntry{WorkspaceID: 1, ProjectID: 2, Start: time.Now(), Duration: time.Minute, Description: "d"})

	require.Error(t, err)
}

func TestCreateTimeEntryRetriesOn402ThenSucceeds(t *testing.T) {
	attempts := 0
	waiter := &countingWaiter{}
	c := newTestClient(t, waiter, noTagsHandler(t, func(w http.ResponseWriter, _ *http.Request) {
		attempts++
		if attempts < 3 {
			w.WriteHeader(http.StatusPaymentRequired)

			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id": 1}`))
	}))

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
	c := newTestClient(t, &countingWaiter{}, noTagsHandler(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusPaymentRequired)
	}))

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
		_, _ = w.Write([]byte(`{"data": [{"id": 7, "name": "alrayyes"}, {"id": 8, "name": "someone-else"}], "page": 1, "per_page": 50}`))
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
			_, _ = w.Write([]byte(`{"data": [], "page": 1, "per_page": 50}`))

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

	t.Run("creates it with a POST to the clients endpoint, ungrouped by organization", func(t *testing.T) {
		require.Equal(t, http.MethodPost, gotMethod)
		require.Equal(t, "/workspaces/1/clients", gotPath)
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
		_, _ = w.Write([]byte(`{"data": [
			{"id": 20, "name": "repo", "client_id": 999},
			{"id": 21, "name": "repo", "client_id": 7}
		], "page": 1, "per_page": 50, "total": 2}`))
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
	var gotPath string
	var gotBody map[string]any

	c := newTestClient(t, &countingWaiter{}, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet {
			_, _ = w.Write([]byte(`{"data": [], "page": 1, "per_page": 50, "total": 0}`))

			return
		}
		gotPath = r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		_, _ = w.Write([]byte(`{"id": 22, "name": "repo", "client_id": 7}`))
	})

	id, err := c.FindOrCreateProject(t.Context(), 1, 7, "repo")
	require.NoError(t, err)

	t.Run("returns the newly created project's id", func(t *testing.T) {
		require.Equal(t, int64(22), id)
	})

	t.Run("creates it under the organization/workspace scope and the given client", func(t *testing.T) {
		require.Equal(t, "/organizations/42/workspaces/1/projects", gotPath)
		require.Equal(t, "repo", gotBody["name"])
		require.InDelta(t, float64(7), gotBody["client_id"], 0)
	})
}

func TestListRecentEntries(t *testing.T) {
	var gotPath string

	c := newTestClient(t, &countingWaiter{}, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.String()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data": [
			{"id": 1, "description": "forgejo-time-entry:5", "duration": 60},
			{"id": 2, "description": "unrelated", "duration": 120}
		], "page": 1, "per_page": 200}`))
	})

	since := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	entries, err := c.ListRecentEntries(t.Context(), 1, since)
	require.NoError(t, err)

	t.Run("hits the organization/workspace time-entries endpoint", func(t *testing.T) {
		require.Contains(t, gotPath, "/organizations/42/workspaces/1/time-entries?")
	})

	t.Run("scopes the lookup to entries since the given time", func(t *testing.T) {
		require.Contains(t, gotPath, "date_from=2026-08-01T00%3A00%3A00Z")
	})

	t.Run("includes taskless entries, since every entry this tool creates is taskless", func(t *testing.T) {
		require.Contains(t, gotPath, "include_taskless=true")
	})

	t.Run("returns every entry the server sent", func(t *testing.T) {
		require.Len(t, entries, 2)
	})
}
