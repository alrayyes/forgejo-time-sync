package config_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/alrayyes/forgejo-time-sync/internal/config"
)

func fakeGetenv(vars map[string]string) func(string) string {
	return func(key string) string { return vars[key] }
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
	cfg, err := config.Load(fakeGetenv(validEnv()))
	require.NoError(t, err)

	t.Run("forwards the forgejo settings", func(t *testing.T) {
		require.Equal(t, "https://forgejo.example.com", cfg.ForgejoBaseURL)
		require.Equal(t, "forgejo-token", cfg.ForgejoToken)
		require.Equal(t, "alrayyes", cfg.ForgejoOwner)
		require.Equal(t, "forgejo-time-sync", cfg.ForgejoRepo)
	})

	t.Run("forwards the toggl settings", func(t *testing.T) {
		require.Equal(t, "toggl-token", cfg.TogglAPIToken)
		require.EqualValues(t, 50, cfg.TogglOrganizationID)
		require.EqualValues(t, 111, cfg.TogglWorkspaceID)
	})

	t.Run("defaults the project id to unset, for auto-provisioning", func(t *testing.T) {
		require.Zero(t, cfg.TogglProjectID)
	})

	t.Run("defaults the sync interval to 10 seconds", func(t *testing.T) {
		require.Equal(t, 10*time.Second, cfg.SyncInterval)
	})

	t.Run("defaults the state file path", func(t *testing.T) {
		require.Equal(t, "/data/state.json", cfg.StateFilePath)
	})

	t.Run("defaults the toggl rate budget to the free tier", func(t *testing.T) {
		require.Equal(t, 30, cfg.TogglMaxRequestsPerHour)
	})
}

func TestLoadOverridesOptionalVars(t *testing.T) {
	env := validEnv()
	env["SYNC_INTERVAL_SECONDS"] = "5"
	env["STATE_FILE_PATH"] = "/tmp/custom.json"
	env["TOGGL_MAX_REQUESTS_PER_HOUR"] = "600"
	env["TOGGL_PROJECT_ID"] = "222"

	cfg, err := config.Load(fakeGetenv(env))
	require.NoError(t, err)

	require.Equal(t, 5*time.Second, cfg.SyncInterval)
	require.Equal(t, "/tmp/custom.json", cfg.StateFilePath)
	require.Equal(t, 600, cfg.TogglMaxRequestsPerHour)
	require.EqualValues(t, 222, cfg.TogglProjectID)
}

func TestLoadRejectsNonNumericProjectID(t *testing.T) {
	env := validEnv()
	env["TOGGL_PROJECT_ID"] = "not-a-number"

	_, err := config.Load(fakeGetenv(env))

	require.Error(t, err)
}

func TestLoadRequiresEveryMandatoryVar(t *testing.T) {
	for _, key := range []string{
		"FORGEJO_BASE_URL", "FORGEJO_TOKEN", "FORGEJO_OWNER", "FORGEJO_REPO",
		"TOGGL_API_TOKEN", "TOGGL_ORGANIZATION_ID", "TOGGL_WORKSPACE_ID",
	} {
		t.Run("missing "+key, func(t *testing.T) {
			env := validEnv()
			delete(env, key)

			_, err := config.Load(fakeGetenv(env))

			require.ErrorContains(t, err, key)
		})
	}
}

func TestLoadRejectsNonNumericTogglIDs(t *testing.T) {
	env := validEnv()
	env["TOGGL_WORKSPACE_ID"] = "not-a-number"

	_, err := config.Load(fakeGetenv(env))

	require.Error(t, err)
}

func TestLoadRejectsNonNumericOrganizationID(t *testing.T) {
	env := validEnv()
	env["TOGGL_ORGANIZATION_ID"] = "not-a-number"

	_, err := config.Load(fakeGetenv(env))

	require.Error(t, err)
}
