package health_test

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/alrayyes/forgejo-time-sync/internal/health"
)

func TestCheckFailsWhenHeartbeatFileIsMissing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "healthcheck")
	hb := health.NewHeartbeat(path)

	err := hb.Check(time.Minute)

	require.Error(t, err)
}

func TestCheckSucceedsRightAfterTouch(t *testing.T) {
	path := filepath.Join(t.TempDir(), "healthcheck")
	hb := health.NewHeartbeat(path)
	require.NoError(t, hb.Touch())

	err := hb.Check(time.Minute)

	require.NoError(t, err)
}

func TestCheckFailsWhenHeartbeatIsStale(t *testing.T) {
	path := filepath.Join(t.TempDir(), "healthcheck")
	hb := health.NewHeartbeat(path)
	require.NoError(t, hb.Touch())

	err := hb.Check(-time.Second) // anything already counts as older than "-1s ago"

	require.ErrorContains(t, err, "stale")
}

func TestTouchCanBeCalledRepeatedly(t *testing.T) {
	path := filepath.Join(t.TempDir(), "healthcheck")
	hb := health.NewHeartbeat(path)

	require.NoError(t, hb.Touch())
	require.NoError(t, hb.Touch())

	require.NoError(t, hb.Check(time.Minute))
}
