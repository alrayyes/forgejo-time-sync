package config_test

import (
	"testing"
	"time"

	"github.com/alrayyes/forgejo-time-sync/internal/config"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
)

func testViper(t *testing.T, overrides map[string]string) *viper.Viper {
	t.Helper()

	v := config.New()
	for key, value := range validEnv() {
		v.Set(key, value)
	}
	for key, value := range overrides {
		v.Set(key, value)
	}

	return v
}

func validEnv() map[string]string {
	return map[string]string{
		"FORGEJO_BASE_URL":      "https://forgejo.example.com",
		"FORGEJO_TOKEN":         "forgejo-token",
		"FORGEJO_OWNER":         "alrayyes",
		"FORGEJO_REPO":          "forgejo-time-sync",
		"TOGGL_API_TOKEN":       "toggl-token",
		"TOGGL_ORGANIZATION_ID": "50",
		"TOGGL_WORKSPACE_ID":    "111",
	}
}

func TestLoadWithAllRequiredVars(t *testing.T) {
	t.Parallel()

	cfg, err := config.Load(testViper(t, nil))
	require.NoError(t, err)

	t.Run("forwards the forgejo settings", func(t *testing.T) {
		t.Parallel()
		require.Equal(t, "https://forgejo.example.com", cfg.ForgejoBaseURL)
		require.Equal(t, "forgejo-token", cfg.ForgejoToken)
		require.Equal(t, "alrayyes", cfg.ForgejoOwner)
		require.Equal(t, "forgejo-time-sync", cfg.ForgejoRepo)
	})

	t.Run("forwards the toggl settings", func(t *testing.T) {
		t.Parallel()
		require.Equal(t, "toggl-token", cfg.TogglAPIToken)
		require.EqualValues(t, 50, cfg.TogglOrganizationID)
		require.EqualValues(t, 111, cfg.TogglWorkspaceID)
	})

	t.Run("defaults the project id to unset, for auto-provisioning", func(t *testing.T) {
		t.Parallel()
		require.Zero(t, cfg.TogglProjectID)
	})

	t.Run("defaults the sync interval to 10 seconds", func(t *testing.T) {
		t.Parallel()
		require.Equal(t, 10*time.Second, cfg.SyncInterval)
	})

	t.Run("defaults the state file path", func(t *testing.T) {
		t.Parallel()
		require.Equal(t, "/data/state.json", cfg.StateFilePath)
	})

	t.Run("defaults the toggl rate budget to the free tier", func(t *testing.T) {
		t.Parallel()
		require.Equal(t, 30, cfg.TogglMaxRequestsPerHour)
	})
}

func TestLoadOverridesOptionalVars(t *testing.T) {
	t.Parallel()

	cfg, err := config.Load(testViper(t, map[string]string{
		"SYNC_INTERVAL_SECONDS":       "5",
		"STATE_FILE_PATH":             "/tmp/custom.json",
		"TOGGL_MAX_REQUESTS_PER_HOUR": "600",
		"TOGGL_PROJECT_ID":            "222",
	}))
	require.NoError(t, err)

	require.Equal(t, 5*time.Second, cfg.SyncInterval)
	require.Equal(t, "/tmp/custom.json", cfg.StateFilePath)
	require.Equal(t, 600, cfg.TogglMaxRequestsPerHour)
	require.EqualValues(t, 222, cfg.TogglProjectID)
}

func TestLoadRejectsNonNumericProjectID(t *testing.T) {
	t.Parallel()

	_, err := config.Load(testViper(t, map[string]string{"TOGGL_PROJECT_ID": "not-a-number"}))

	require.Error(t, err)
}

func TestLoadRequiresEveryMandatoryVar(t *testing.T) {
	t.Parallel()

	for _, key := range []string{
		"FORGEJO_BASE_URL", "FORGEJO_TOKEN", "FORGEJO_OWNER", "FORGEJO_REPO",
		"TOGGL_API_TOKEN", "TOGGL_ORGANIZATION_ID", "TOGGL_WORKSPACE_ID",
	} {
		t.Run("missing "+key, func(t *testing.T) {
			t.Parallel()

			v := config.New()
			for envKey, value := range validEnv() {
				if envKey != key {
					v.Set(envKey, value)
				}
			}

			_, err := config.Load(v)

			require.ErrorContains(t, err, key)
		})
	}
}

func TestLoadRejectsNonNumericTogglIDs(t *testing.T) {
	t.Parallel()

	_, err := config.Load(testViper(t, map[string]string{"TOGGL_WORKSPACE_ID": "not-a-number"}))

	require.Error(t, err)
}

func TestLoadRejectsNonNumericOrganizationID(t *testing.T) {
	t.Parallel()

	_, err := config.Load(testViper(t, map[string]string{"TOGGL_ORGANIZATION_ID": "not-a-number"}))

	require.Error(t, err)
}
