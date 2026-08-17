package forgejo_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/alrayyes/forgejo-time-sync/internal/forgejo"
)

func TestListRepoTimes(t *testing.T) {
	var gotPath, gotAuth string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[
			{"id": 1, "created": "2026-08-17T09:00:00Z", "time": 5400, "issue": {"number": 12}}
		]`))
	}))
	defer srv.Close()

	c := forgejo.Client{BaseURL: srv.URL, Token: "s3cr3t"}
	entries, err := c.ListRepoTimes(t.Context(), "alrayyes", "forgejo-time-sync")
	require.NoError(t, err)

	t.Run("hits the repo times endpoint", func(t *testing.T) {
		require.Equal(t, "/api/v1/repos/alrayyes/forgejo-time-sync/times", gotPath)
	})

	t.Run("authenticates with the token header", func(t *testing.T) {
		require.Equal(t, "token s3cr3t", gotAuth)
	})

	t.Run("returns every entry the server sent", func(t *testing.T) {
		require.Len(t, entries, 1)
	})

	e := entries[0]
	t.Run("parses the entry id", func(t *testing.T) {
		require.Equal(t, int64(1), e.ID)
	})

	t.Run("parses the created timestamp", func(t *testing.T) {
		require.Equal(t, time.Date(2026, 8, 17, 9, 0, 0, 0, time.UTC), e.Created)
	})

	t.Run("parses the duration in seconds", func(t *testing.T) {
		require.Equal(t, int64(5400), e.Seconds)
	})

	t.Run("parses the nested issue number", func(t *testing.T) {
		require.Equal(t, int64(12), e.IssueNumber)
	})
}

func TestListRepoTimesErrorStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	c := forgejo.Client{BaseURL: srv.URL, Token: "bad-token"}
	_, err := c.ListRepoTimes(t.Context(), "alrayyes", "forgejo-time-sync")

	require.Error(t, err)
}
