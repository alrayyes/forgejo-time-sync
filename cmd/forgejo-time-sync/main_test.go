package main

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestHeartbeatPathSitsBesideTheStateFile(t *testing.T) {
	t.Parallel()

	require.Equal(t, "/data/healthcheck", heartbeatPath("/data/state.json"))
}

func TestHeartbeatMaxAgeIsThreePollIntervals(t *testing.T) {
	t.Parallel()

	require.Equal(t, 30*time.Second, heartbeatMaxAge(10*time.Second))
}
