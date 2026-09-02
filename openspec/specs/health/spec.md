# Health and liveness

## Purpose

Tracks whether the poll loop is still actively cycling, and backs the
`healthcheck` subcommand Docker's `HEALTHCHECK` calls.

## Requirements

### Requirement: Liveness heartbeat

The system SHALL maintain a heartbeat file, on the same volume as the state
file, recording whether the poll loop is still actively cycling.

#### Scenario: Loop starts

- **WHEN** the poll loop starts
- **THEN** the heartbeat file is touched immediately, before waiting out the
  first interval

#### Scenario: Poll tick completes

- **WHEN** a poll tick completes, whether or not it synced any entries or
  returned an error
- **THEN** the heartbeat file is touched again

### Requirement: Heartbeat-backed healthcheck

The `healthcheck` subcommand SHALL report failure when the heartbeat is
missing or older than three poll intervals, and success otherwise.

#### Scenario: Heartbeat missing

- **WHEN** the heartbeat file doesn't exist
- **THEN** `healthcheck` exits non-zero

#### Scenario: Heartbeat stale

- **WHEN** the heartbeat file's last modification is older than three times
  `SYNC_INTERVAL_SECONDS`
- **THEN** `healthcheck` exits non-zero

#### Scenario: Heartbeat fresh

- **WHEN** the heartbeat file was touched within three poll intervals
- **THEN** `healthcheck` exits 0

#### Scenario: Forgejo or Toggl unreachable

- **WHEN** a poll tick fails because Forgejo or Toggl is unreachable, but the
  loop itself is still cycling and touching the heartbeat
- **THEN** `healthcheck` still reports healthy — the heartbeat tracks the
  loop's liveness, not the last poll's success, so a real outage keeps
  retrying instead of getting the container restarted
