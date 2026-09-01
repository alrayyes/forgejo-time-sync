package forgejo_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/alrayyes/forgejo-time-sync/internal/forgejo"
	"github.com/stretchr/testify/require"
)

// newTestServer serves the SDK's own version handshake plus whatever
// handler the test provides for the actual call under test.
func newTestServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/version", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"version": "12.0.0"}`))
	})
	mux.HandleFunc("/", handler)

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	return srv
}

func TestNewClientConnects(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})

	_, err := forgejo.NewClient(srv.URL, "s3cr3t")

	require.NoError(t, err)
}

func TestNewClientFailsOnAConnectionError(t *testing.T) {
	t.Parallel()

	_, err := forgejo.NewClient("http://127.0.0.1:1", "s3cr3t")

	require.Error(t, err)
}

// listRepoTimesResult is what TestListRepoTimes's subtests assert against —
// captured here so the arrange/act step isn't repeated per subtest and
// doesn't count against the test function's own length.
type listRepoTimesResult struct {
	path, auth string
	entries    []forgejo.TimeEntry
}

func listRepoTimes(t *testing.T) listRepoTimesResult {
	t.Helper()

	var gotPath, gotAuth string

	srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[
			{"id": 1, "created": "2026-08-17T09:00:00Z", "time": 5400, "issue": {"number": 12}}
		]`))
	})

	c, err := forgejo.NewClient(srv.URL, "s3cr3t")
	require.NoError(t, err)

	entries, err := c.ListRepoTimes(t.Context(), "alrayyes", "forgejo-time-sync")
	require.NoError(t, err)

	return listRepoTimesResult{path: gotPath, auth: gotAuth, entries: entries}
}

func TestListRepoTimes(t *testing.T) {
	t.Parallel()

	r := listRepoTimes(t)
	gotPath, gotAuth, entries := r.path, r.auth, r.entries

	t.Run("hits the repo times endpoint", func(t *testing.T) {
		t.Parallel()

		require.Equal(t, "/api/v1/repos/alrayyes/forgejo-time-sync/times", gotPath)
	})

	t.Run("authenticates with the token header", func(t *testing.T) {
		t.Parallel()

		require.Equal(t, "token s3cr3t", gotAuth)
	})

	t.Run("returns every entry the server sent", func(t *testing.T) {
		t.Parallel()

		require.Len(t, entries, 1)
	})

	e := entries[0]
	t.Run("parses the entry id", func(t *testing.T) {
		t.Parallel()

		require.Equal(t, int64(1), e.ID)
	})

	t.Run("parses the created timestamp", func(t *testing.T) {
		t.Parallel()

		require.Equal(t, time.Date(2026, 8, 17, 9, 0, 0, 0, time.UTC), e.Created)
	})

	t.Run("parses the duration in seconds", func(t *testing.T) {
		t.Parallel()

		require.Equal(t, int64(5400), e.Seconds)
	})

	t.Run("parses the nested issue number", func(t *testing.T) {
		t.Parallel()

		require.Equal(t, int64(12), e.IssueNumber)
	})
}

func TestListRepoTimesErrorStatus(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	})
	c, err := forgejo.NewClient(srv.URL, "bad-token")
	require.NoError(t, err)

	_, err = c.ListRepoTimes(t.Context(), "alrayyes", "forgejo-time-sync")

	require.Error(t, err)
}
