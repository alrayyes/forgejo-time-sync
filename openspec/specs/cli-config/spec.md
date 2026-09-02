# CLI and configuration

## Purpose

Parses the command line and loads every setting this tool needs from the
environment, with no config file to author or persist.

## Requirements

### Requirement: Command-line surface

The system SHALL parse its command line with Cobra, exposing a default run
command and a `healthcheck` subcommand.

#### Scenario: No subcommand given

- **WHEN** the binary is invoked with no arguments
- **THEN** it starts the poll loop (see `forgejo-polling` and `toggl-sync`)

#### Scenario: healthcheck subcommand

- **WHEN** the binary is invoked as `forgejo-time-sync healthcheck`
- **THEN** it checks the heartbeat file and exits 0 if fresh, non-zero if
  stale or missing, without starting the poll loop

### Requirement: Environment-variable configuration

The system SHALL load all configuration from environment variables through
Viper, with no config file, `init` command, or XDG config path — it is a
Docker-only daemon with nothing for a user to persist to disk.

#### Scenario: Required variable missing

- **WHEN** any of `FORGEJO_BASE_URL`, `FORGEJO_TOKEN`, `FORGEJO_OWNER`,
  `FORGEJO_REPO`, `TOGGL_API_TOKEN`, `TOGGL_ORGANIZATION_ID`, or
  `TOGGL_WORKSPACE_ID` is unset or empty
- **THEN** the process fails to start with an error naming every missing
  variable, rather than starting with a partial configuration

#### Scenario: Numeric setting is not a number

- **WHEN** `TOGGL_ORGANIZATION_ID`, `TOGGL_WORKSPACE_ID`,
  `TOGGL_PROJECT_ID`, `SYNC_INTERVAL_SECONDS`, or
  `TOGGL_MAX_REQUESTS_PER_HOUR` is set to a non-numeric string
- **THEN** the process fails to start with an error naming that variable,
  rather than silently treating it as zero

#### Scenario: Optional settings default

- **WHEN** `SYNC_INTERVAL_SECONDS`, `STATE_FILE_PATH`, or
  `TOGGL_MAX_REQUESTS_PER_HOUR` is unset
- **THEN** it defaults to 10 seconds, `/data/state.json`, and 30
  requests/hour respectively

#### Scenario: TOGGL_PROJECT_ID unset

- **WHEN** `TOGGL_PROJECT_ID` is unset
- **THEN** the Toggl project is auto-provisioned instead of pinned to an
  explicit ID (see `toggl-project-resolution`)
