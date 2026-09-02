# Toggl project resolution

## Purpose

Settles which Toggl project a repo's synced time lands in, auto-provisioning
one the first time so a user doesn't have to create it by hand first.

## Requirements

### Requirement: Resolve the Toggl project to sync into

The system SHALL resolve the Toggl project ID to sync into, in priority
order: an explicitly configured `TOGGL_PROJECT_ID`, a project ID already
cached in state, or a freshly auto-provisioned project.

#### Scenario: TOGGL_PROJECT_ID is set

- **WHEN** `TOGGL_PROJECT_ID` is configured
- **THEN** that ID is used directly, with no Toggl lookup and nothing cached
  beyond it

#### Scenario: No explicit ID, nothing cached yet

- **WHEN** `TOGGL_PROJECT_ID` is unset and the state file has no cached
  project ID
- **THEN** the system auto-provisions a Toggl client named after
  `FORGEJO_OWNER` and a project named after `FORGEJO_REPO` under it,
  creating either that doesn't already exist by that name, then caches the
  resulting project ID in state

#### Scenario: No explicit ID, already cached

- **WHEN** `TOGGL_PROJECT_ID` is unset and the state file already has a
  cached project ID
- **THEN** the cached ID is used, with no Toggl lookup at all
