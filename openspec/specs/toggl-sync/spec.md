# Toggl sync

## Purpose

Pushes new Forgejo time entries into Toggl as completed entries, within
Toggl's own rate limit, without ever touching a time entry that already
exists.

## Requirements

### Requirement: Create a Toggl entry per new Forgejo time entry

The system SHALL create one already-completed Toggl time entry for every
Forgejo tracked-time entry not already recorded in state, and SHALL record
it as synced immediately after.

#### Scenario: New Forgejo entry found

- **WHEN** a poll finds a Forgejo time entry whose ID isn't in state
- **THEN** a Toggl time entry is created with the entry's start time and
  duration, described as `forgejo-time-entry:<id> issue:<owner>/<repo>#<n>`,
  and tagged `<owner>/<repo>#<n>`

#### Scenario: One entry in a batch fails to sync

- **WHEN** creating a Toggl entry for one Forgejo entry in a poll's batch
  fails
- **THEN** the entries already created earlier in that batch stay recorded
  as synced, the failing entry is not recorded, and the poll stops rather
  than continuing past the failure

### Requirement: Never touch an existing Toggl time entry

The system SHALL only ever create new Toggl time entries — never start,
stop, or modify one that already exists — so a timer started by hand in the
Toggl app is never touched no matter how often this polls.

#### Scenario: A running timer exists in Toggl

- **WHEN** a user has a timer running in the Toggl app while a poll happens
- **THEN** the poll neither reads, stops, nor otherwise affects that running
  timer — the Toggl client this system uses has no method capable of it

### Requirement: Throttle Toggl API calls to a configured budget

The system SHALL throttle its own requests to Toggl to stay within
`TOGGL_MAX_REQUESTS_PER_HOUR` (default 30, Toggl Free tier's own cap),
waiting between calls rather than making them and handling a rejection.

#### Scenario: Nothing new to sync

- **WHEN** a poll finds no new Forgejo time entries
- **THEN** no Toggl API call is made at all — the local state file is what
  makes the poll interval independent of Toggl's rate limit

#### Scenario: Calls made faster than the configured budget

- **WHEN** the system would otherwise make two Toggl API calls closer
  together than `time.Hour / TOGGL_MAX_REQUESTS_PER_HOUR`
- **THEN** the second call blocks until that spacing has elapsed

### Requirement: Retry on Toggl's rate-limit response

The system SHALL retry a Toggl API call that's rejected for being rate
limited, up to 5 attempts, rather than failing the poll immediately.

#### Scenario: Toggl responds 402 Payment Required

- **WHEN** a Toggl API call receives Toggl's rate-limit status
  (`402 Payment Required`, not the usual `429`)
- **THEN** the system paces and retries the same call, up to 5 attempts
  total, before giving up and reporting an error
