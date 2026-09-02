# Startup reconciliation

## Purpose

Recovers from a lost or fresh state volume by seeding state from Toggl's own
history, so it costs one extra API call instead of duplicating every entry
already synced.

## Requirements

### Requirement: Seed lost state from Toggl's own history

The system SHALL, on startup only, seed an empty state file by searching
Toggl's own history for entries this tool already created, so a lost or
fresh state volume costs one extra Toggl call rather than duplicating every
entry already synced.

#### Scenario: State is empty on startup

- **WHEN** the system starts and the loaded state has no recorded entries
- **THEN** it searches the last 90 days of Toggl entries in the configured
  workspace, and for each one whose description matches the
  `forgejo-time-entry:<id>` format this tool writes, records that ID as
  already synced before the poll loop starts

#### Scenario: State is not empty on startup

- **WHEN** the system starts and the loaded state already has at least one
  recorded entry
- **THEN** no reconciliation search runs — this is a cold-start recovery
  path, never part of the steady-state poll

#### Scenario: Reconciliation search fails

- **WHEN** the Toggl history search during startup reconciliation fails
- **THEN** the system logs a warning and continues starting the poll loop
  with empty state, rather than refusing to start

#### Scenario: A Toggl entry doesn't match this tool's format

- **WHEN** a Toggl entry in the reconciliation window has a description that
  doesn't match the `forgejo-time-entry:<id>` format
- **THEN** it's ignored — only entries this tool itself created are ever
  recorded during reconciliation
