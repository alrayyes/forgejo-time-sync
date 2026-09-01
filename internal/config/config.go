// Package config reads this tool's environment-variable configuration.
package config

import (
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/spf13/viper"
)

// Config is the fully resolved, validated set of settings this tool needs
// to run — one Forgejo repo synced into one Toggl project.
type Config struct {
	ForgejoBaseURL string
	ForgejoToken   string
	ForgejoOwner   string
	ForgejoRepo    string

	TogglAPIToken string
	// TogglOrganizationID and TogglWorkspaceID together scope every Focus API
	// call — the API has no "list my organizations" endpoint to derive the
	// former from, so it's configured directly (find it in Toggl's
	// organization settings URL).
	TogglOrganizationID int64
	TogglWorkspaceID    int64
	// TogglProjectID is optional: 0 means auto-provision a Toggl client
	// (named after ForgejoOwner) and project (named after ForgejoRepo)
	// instead of syncing into an explicitly chosen one.
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

var requiredKeys = []string{
	"FORGEJO_BASE_URL", "FORGEJO_TOKEN", "FORGEJO_OWNER", "FORGEJO_REPO",
	"TOGGL_API_TOKEN", "TOGGL_ORGANIZATION_ID", "TOGGL_WORKSPACE_ID",
}

// New returns a Viper instance layered over this tool's environment
// variables, with defaults for every optional setting. There is no config
// file layer: this tool is a Docker-only daemon with nothing for a user to
// persist, so env vars are the only real source — Viper here is purely the
// resolution/precedence and validation mechanism rules/go.md's Configuration
// section asks for.
func New() *viper.Viper {
	v := viper.New()
	v.AutomaticEnv()
	v.SetDefault("SYNC_INTERVAL_SECONDS", defaultSyncIntervalSeconds)
	v.SetDefault("STATE_FILE_PATH", defaultStateFilePath)
	v.SetDefault("TOGGL_MAX_REQUESTS_PER_HOUR", defaultTogglMaxRequestsPerHour)

	return v
}

// Load resolves Config from v, then validates it. v is New() in production;
// a test builds its own with viper.New() plus v.Set(key, value) so tests
// need no real environment variables and stay parallel-safe.
func Load(v *viper.Viper) (Config, error) {
	var missing []string

	for _, key := range requiredKeys {
		if v.GetString(key) == "" {
			missing = append(missing, key)
		}
	}

	if len(missing) > 0 {
		return Config{}, fmt.Errorf("%w: %v", errMissingRequiredVars, missing)
	}

	cfg := Config{
		ForgejoBaseURL: v.GetString("FORGEJO_BASE_URL"),
		ForgejoToken:   v.GetString("FORGEJO_TOKEN"),
		ForgejoOwner:   v.GetString("FORGEJO_OWNER"),
		ForgejoRepo:    v.GetString("FORGEJO_REPO"),
		TogglAPIToken:  v.GetString("TOGGL_API_TOKEN"),
		StateFilePath:  v.GetString("STATE_FILE_PATH"),
	}

	if err := cfg.parseNumeric(v); err != nil {
		return Config{}, err
	}

	return cfg, nil
}

var (
	errMissingRequiredVars = errors.New("config: missing required environment variables")
	errInvalidValue        = errors.New("config: invalid value")
)

// parseNumeric fills in every field parsed from a numeric string. Viper's
// own GetInt64/GetInt cast a non-numeric string to zero rather than
// erroring, so — per rules/go.md's "config from the environment can lie,
// validate it as it loads" — these are parsed by hand instead.
func (cfg *Config) parseNumeric(v *viper.Viper) error {
	var err error

	if cfg.TogglOrganizationID, err = strconv.ParseInt(v.GetString("TOGGL_ORGANIZATION_ID"), 10, 64); err != nil {
		return fmt.Errorf("%w: TOGGL_ORGANIZATION_ID: %w", errInvalidValue, err)
	}
	if cfg.TogglWorkspaceID, err = strconv.ParseInt(v.GetString("TOGGL_WORKSPACE_ID"), 10, 64); err != nil {
		return fmt.Errorf("%w: TOGGL_WORKSPACE_ID: %w", errInvalidValue, err)
	}
	if s := v.GetString("TOGGL_PROJECT_ID"); s != "" {
		if cfg.TogglProjectID, err = strconv.ParseInt(s, 10, 64); err != nil {
			return fmt.Errorf("%w: TOGGL_PROJECT_ID: %w", errInvalidValue, err)
		}
	}

	seconds, err := strconv.Atoi(v.GetString("SYNC_INTERVAL_SECONDS"))
	if err != nil {
		return fmt.Errorf("%w: SYNC_INTERVAL_SECONDS: %w", errInvalidValue, err)
	}
	cfg.SyncInterval = time.Duration(seconds) * time.Second

	if cfg.TogglMaxRequestsPerHour, err = strconv.Atoi(v.GetString("TOGGL_MAX_REQUESTS_PER_HOUR")); err != nil {
		return fmt.Errorf("%w: TOGGL_MAX_REQUESTS_PER_HOUR: %w", errInvalidValue, err)
	}

	return nil
}
