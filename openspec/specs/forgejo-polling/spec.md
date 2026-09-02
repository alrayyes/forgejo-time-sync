# Forgejo time-entry polling

## Purpose

Polls one Forgejo repository's tracked time on a fixed interval, since
Forgejo has no webhook event for time tracking to react to instead.

## Requirements

### Requirement: Poll a single repo's tracked time

The system SHALL poll one configured Forgejo repository's tracked time on a
fixed interval, since Forgejo has no webhook event for time tracking.

#### Scenario: Poll tick

- **WHEN** the configured `SYNC_INTERVAL_SECONDS` elapses
- **THEN** the system fetches every tracked time entry on
  `FORGEJO_OWNER`/`FORGEJO_REPO` from the Forgejo API in a single
  unpaginated request

#### Scenario: One container instance per repo

- **WHEN** more than one Forgejo repo needs syncing
- **THEN** each repo runs its own container instance, its own config, and
  its own state volume — one instance never polls more than one repo

### Requirement: Tolerate an unrecognized Forgejo version

The system SHALL connect to a Forgejo instance even when the SDK doesn't
recognize its reported version, since Forgejo is a young enough fork that a
strict lower-bound check isn't worth failing startup over.

#### Scenario: Version check fails with ErrUnknownVersion

- **WHEN** connecting to the configured Forgejo instance returns an
  unknown-version error
- **THEN** the connection still succeeds

#### Scenario: Connection genuinely fails

- **WHEN** connecting to the configured Forgejo instance fails for any other
  reason (network error, invalid token, unreachable host)
- **THEN** the system reports the error and does not start the poll loop
