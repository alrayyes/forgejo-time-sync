package state_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/alrayyes/forgejo-time-sync/internal/state"
)

func TestLoadMissingFileStartsEmpty(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")

	s, err := state.Load(path)
	require.NoError(t, err)

	t.Run("has no synced ids", func(t *testing.T) {
		require.False(t, s.Has(1))
	})

	t.Run("reports zero length", func(t *testing.T) {
		require.Zero(t, s.Len())
	})
}

func TestAddPersistsAcrossLoad(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	s, err := state.Load(path)
	require.NoError(t, err)
	require.NoError(t, s.Add(42))

	reloaded, err := state.Load(path)
	require.NoError(t, err)

	t.Run("the id added before persisting survives a reload", func(t *testing.T) {
		require.True(t, reloaded.Has(42))
	})

	t.Run("length survives a reload", func(t *testing.T) {
		require.Equal(t, 1, reloaded.Len())
	})
}

func TestAddIsIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	s, err := state.Load(path)
	require.NoError(t, err)

	require.NoError(t, s.Add(1))
	require.NoError(t, s.Add(1))

	require.Equal(t, 1, s.Len(), "adding the same id twice should not grow the state")
}

func TestAddLeavesNoTempFileBehind(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")
	s, err := state.Load(path)
	require.NoError(t, err)
	require.NoError(t, s.Add(7))

	entries, err := os.ReadDir(dir)
	require.NoError(t, err)

	require.Len(t, entries, 1, "an atomic write should not leave a temp file behind")
	require.Equal(t, "state.json", entries[0].Name())
}
