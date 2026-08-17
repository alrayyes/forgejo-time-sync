// Package config reads this tool's environment-variable configuration.
package config

import (
	"fmt"
	"strconv"
	"time"
)

// Config is the fully resolved, validated set of settings this tool needs
// to run — one Forgejo repo synced into one Toggl project.
type Config struct {
	ForgejoBaseURL string
	ForgejoToken   string
	ForgejoOwner   string
	ForgejoRepo    string

	TogglAPIToken           string
	TogglWorkspaceID        int64
	TogglProjectID          int64
	TogglMaxRequestsPerHour int

	SyncInterval  time.Duration
	StateFilePath string
}

const (
	defaultSyncIntervalSeconds     = 10
	defaultStateFilePath           = "/data/state.json"
	defaultTogglMaxRequestsPerHour = 30
)

// Load reads Config from environment variables via getenv (os.Getenv in
// production; a fake map in tests).
func Load(getenv func(string) string) (Config, error) {
	var cfg Config
	var missing []string

	required := func(key string) string {
		v := getenv(key)
		if v == "" {
			missing = append(missing, key)
		}
		return v
	}

	cfg.ForgejoBaseURL = required("FORGEJO_BASE_URL")
	cfg.ForgejoToken = required("FORGEJO_TOKEN")
	cfg.ForgejoOwner = required("FORGEJO_OWNER")
	cfg.ForgejoRepo = required("FORGEJO_REPO")
	cfg.TogglAPIToken = required("TOGGL_API_TOKEN")
	workspaceID := required("TOGGL_WORKSPACE_ID")
	projectID := required("TOGGL_PROJECT_ID")

	if len(missing) > 0 {
		return Config{}, fmt.Errorf("config: missing required environment variables: %v", missing)
	}

	var err error
	if cfg.TogglWorkspaceID, err = strconv.ParseInt(workspaceID, 10, 64); err != nil {
		return Config{}, fmt.Errorf("config: TOGGL_WORKSPACE_ID: %w", err)
	}
	if cfg.TogglProjectID, err = strconv.ParseInt(projectID, 10, 64); err != nil {
		return Config{}, fmt.Errorf("config: TOGGL_PROJECT_ID: %w", err)
	}

	cfg.SyncInterval = defaultSyncIntervalSeconds * time.Second
	if v := getenv("SYNC_INTERVAL_SECONDS"); v != "" {
		seconds, err := strconv.Atoi(v)
		if err != nil {
			return Config{}, fmt.Errorf("config: SYNC_INTERVAL_SECONDS: %w", err)
		}
		cfg.SyncInterval = time.Duration(seconds) * time.Second
	}

	cfg.StateFilePath = defaultStateFilePath
	if v := getenv("STATE_FILE_PATH"); v != "" {
		cfg.StateFilePath = v
	}

	cfg.TogglMaxRequestsPerHour = defaultTogglMaxRequestsPerHour
	if v := getenv("TOGGL_MAX_REQUESTS_PER_HOUR"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			return Config{}, fmt.Errorf("config: TOGGL_MAX_REQUESTS_PER_HOUR: %w", err)
		}
		cfg.TogglMaxRequestsPerHour = n
	}

	return cfg, nil
}
