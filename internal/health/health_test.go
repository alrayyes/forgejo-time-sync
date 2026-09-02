package health_test

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/alrayyes/forgejo-time-sync/internal/health"
	"github.com/stretchr/testify/require"
)

func TestCheckFailsWhenHeartbeatFileIsMissing(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "healthcheck")
	hb := health.NewHeartbeat(path)

	err := hb.Check(time.Minute)

	require.Error(t, err)
}

func TestCheckSucceedsRightAfterTouch(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "healthcheck")
	hb := health.NewHeartbeat(path)
	require.NoError(t, hb.Touch())

	err := hb.Check(time.Minute)

	require.NoError(t, err)
}

func TestCheckFailsWhenHeartbeatIsStale(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "healthcheck")
	hb := health.NewHeartbeat(path)
	require.NoError(t, hb.Touch())

	err := hb.Check(-time.Second) // anything already counts as older than "-1s ago"

	require.ErrorContains(t, err, "stale")
}

func TestTouchCanBeCalledRepeatedly(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "healthcheck")
	hb := health.NewHeartbeat(path)

	require.NoError(t, hb.Touch())
	require.NoError(t, hb.Touch())

	require.NoError(t, hb.Check(time.Minute))
}

func TestHeartbeatPathSitsBesideTheStateFile(t *testing.T) {
	t.Parallel()

	require.Equal(t, "/data/healthcheck", health.HeartbeatPath("/data/state.json"))
}

func TestMaxAgeIsThreePollIntervals(t *testing.T) {
	t.Parallel()

	require.Equal(t, 30*time.Second, health.MaxAge(10*time.Second))
}
