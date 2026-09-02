# Sync state

## Purpose

Tracks which Forgejo time entries are already synced to Toggl, and caches
the resolved Toggl project, so a restart never re-derives or re-syncs
either from scratch.

## Requirements

### Requirement: Synced-entry tracking

The system SHALL persist, on disk, which Forgejo time-entry IDs have already
been pushed to Toggl, so a restart doesn't lose track of what's synced.

#### Scenario: State file doesn't exist yet

- **WHEN** the configured state file path has no file on it
- **THEN** the system starts with empty state rather than failing

#### Scenario: Entry recorded

- **WHEN** a Forgejo time entry is synced to Toggl
- **THEN** its ID is recorded in the state file before the next poll runs

#### Scenario: Entry already recorded

- **WHEN** a poll finds a Forgejo time entry whose ID is already recorded
- **THEN** it's skipped without any Toggl API call

### Requirement: Cached Toggl project

The system SHALL cache the resolved Toggl project ID in the state file once
resolved, and use the cached value on every later run instead of re-deriving
it by name.

#### Scenario: Project resolved for the first time

- **WHEN** the Toggl project is resolved (explicitly configured or
  auto-provisioned) and no ID is yet cached
- **THEN** the resolved ID is persisted to the state file

#### Scenario: Project already cached

- **WHEN** the state file already has a cached project ID
- **THEN** it's used as-is, with no lookup against Toggl by name — so
  renaming the project later in Toggl's UI is never fought or duplicated

### Requirement: Crash-safe writes

The system SHALL write the state file so that a crash or power loss during
a save can never leave a corrupt or partially-written file in its place.

#### Scenario: Write interrupted

- **WHEN** the process is killed while a state save is in progress
- **THEN** the state file on disk is either the previous complete version or
  the new complete version, never a partial one — writes go to a temp file
  in the same directory, then rename over the real path
