# forgejo-time-sync

[![CI](https://github.com/alrayyes/forgejo-time-sync/actions/workflows/ci.yml/badge.svg)](https://github.com/alrayyes/forgejo-time-sync/actions/workflows/ci.yml)
[![codecov](https://codecov.io/gh/alrayyes/forgejo-time-sync/graph/badge.svg)](https://codecov.io/gh/alrayyes/forgejo-time-sync)
[![Go Reference](https://pkg.go.dev/badge/github.com/alrayyes/forgejo-time-sync.svg)](https://pkg.go.dev/github.com/alrayyes/forgejo-time-sync)
[![License: GPL-3.0](https://img.shields.io/badge/license-GPL--3.0-blue.svg)](LICENSE)

Polls a Forgejo repo's tracked time and pushes any entry that isn't already
in Toggl into it. Packaged as a container you start once and leave running —
it polls on its own timer, there's nothing to re-invoke.

This targets Toggl's 2.0 (Focus) API, not the older Track v9 API — they're
separate products with different auth. `TOGGL_API_TOKEN` needs a Focus
personal API key (`toggl_sk_...`, from your Toggl account settings), not a
classic Track API token; the two aren't interchangeable and Toggl won't tell
you which one it got, it'll just 403.

Forgejo has no webhook event for time tracking, so this has to poll rather
than react.

## How it works

Every `SYNC_INTERVAL_SECONDS`, it calls Forgejo's
`GET /repos/{owner}/{repo}/times` and creates a Toggl time entry for any
entry not already recorded in local state. Each entry's description reads
`forgejo-time-entry:<id> issue:<owner>/<repo>#<number>`, and it's also
tagged with `<owner>/<repo>#<number>` — the same reference, but as
structured metadata you can filter and search by in Toggl, not just read
in the description text.

That local state — not asking
Toggl what already exists — is what makes a fast poll interval possible at
all: Toggl's API caps the free tier at 30 requests/hour, per user, per
organization (see the rate-limit section of
[Toggl's Focus API docs](https://engineering.toggl.com/docs/focus/)), so
re-querying Toggl on every poll would make anything faster than roughly one
poll every two minutes impossible. Instead, Toggl only ever gets a request
when there's an actual new Forgejo entry to push, throttled client-side to
stay under `TOGGL_MAX_REQUESTS_PER_HOUR` (default 30 — raise it if you're on
a paid Toggl plan). The state file lives on a Docker volume, so it survives
container restarts.

If that state file is ever lost (a fresh volume, or one that got deleted),
the tool does a one-time reconciliation pass against Toggl's own history on
startup before it starts polling, so a lost volume costs one extra Toggl
call rather than duplicating every entry that had already been synced.

The tool only ever _creates_ time entries — nothing in it can start, stop,
or modify one that already exists. That's deliberate: it means a timer you
started by hand in the Toggl app is never touched, however often this
polls. See `internal/toggl`'s package doc for how that's enforced at the
type level, not just by convention.

One Forgejo repo maps to one Toggl project, and one container instance
handles one repo. Multiple repos means multiple container instances, each
with its own config and state volume.

If you don't already have a Toggl project to sync into, leave
`TOGGL_PROJECT_ID` unset and it auto-provisions one on first run: a Toggl
client named after `FORGEJO_OWNER`, and a project under it named after
`FORGEJO_REPO`. The resolved project ID is then cached in the state file
and used as-is from then on — it's never looked up by name again, so
renaming the project in Toggl's UI later doesn't get fought or duplicated.

## Requirements

- A Forgejo instance with an API token that can read the target repo's
  tracked time.
- A Toggl account and a Focus (2.0) API personal key — from your Toggl
  account settings, starts with `toggl_sk_`. You don't need to create a
  project up front — see above.
- Docker (or Go 1.27+ and a Docker daemon, if building from source — the
  test suite's container/integration layer needs one either way).

## Installation

Pull the published image — every merge to `main` releases a new version to
`ghcr.io`, tagged with both the version and `latest`:

```sh
mkdir forgejo-time-sync && cd forgejo-time-sync
curl -O https://raw.githubusercontent.com/alrayyes/forgejo-time-sync/main/docker-compose.yml
curl -O https://raw.githubusercontent.com/alrayyes/forgejo-time-sync/main/.env.example
cp .env.example .env # fill in your Forgejo and Toggl details
docker run -d --restart unless-stopped \
  --env-file .env \
  -v forgejo-time-sync-state:/data \
  ghcr.io/alrayyes/forgejo-time-sync:latest
```

Or build from source:

```sh
git clone https://github.com/alrayyes/forgejo-time-sync.git
cd forgejo-time-sync
cp .env.example .env # fill in your Forgejo and Toggl details
docker compose up -d
```

## Configuration

All configuration is environment variables — see `.env.example`.

| Variable                      | Required | Default            |
| ----------------------------- | -------- | ------------------ |
| `FORGEJO_BASE_URL`            | yes      | —                  |
| `FORGEJO_TOKEN`               | yes      | —                  |
| `FORGEJO_OWNER`               | yes      | —                  |
| `FORGEJO_REPO`                | yes      | —                  |
| `TOGGL_API_TOKEN`             | yes      | —                  |
| `TOGGL_ORGANIZATION_ID`       | yes      | —                  |
| `TOGGL_WORKSPACE_ID`          | yes      | —                  |
| `TOGGL_PROJECT_ID`            | no       | auto-provisioned   |
| `SYNC_INTERVAL_SECONDS`       | no       | `10`               |
| `STATE_FILE_PATH`             | no       | `/data/state.json` |
| `TOGGL_MAX_REQUESTS_PER_HOUR` | no       | `30`               |

The Focus API has no "list my organizations" endpoint to derive
`TOGGL_ORGANIZATION_ID` from, so find both ids by hand in Toggl's own UI —
organization settings and workspace settings each show their id.

## Usage

Once running, it logs a structured JSON line per poll that finds something
new:

```json
{ "level": "INFO", "msg": "synced new time entries", "count": 1 }
```

It shuts down cleanly on `SIGTERM`/`SIGINT` — `docker compose down` or
`docker stop` are safe.

The image has a `HEALTHCHECK`: it touches a heartbeat file next to
`state.json` after every poll cycle and fails if that file goes stale for
more than 3 poll intervals. It tracks whether the loop is still cycling,
not whether Forgejo/Toggl are reachable — a real outage on either side
should keep retrying forever, not get "fixed" by Docker restarting a
container that was never actually stuck. `docker ps` and `docker inspect`
show the status the usual way.

## Testing

```sh
go test ./...              # fast, no Docker needed
go test -tags e2e ./e2e/... # real Forgejo + a Toggl-spec-mocked Prism container
```

The e2e suite doesn't call the real Toggl API — it mocks against Toggl's own
published Focus (2.0) API OpenAPI spec (vendored in `e2e/testdata/`,
refreshed by `scripts/vendor-focus-spec.sh`) via
[Prism](https://stoplight.io/open-source/prism),
so requests are validated against Toggl's real contract without CI
depending on Toggl's uptime or eating into its rate limit on every run. If
that vendored spec ever drifts far enough from reality to matter, the
fallback is testing against a real Toggl test workspace instead — not
needed yet.

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md).

## License

[GPL-3.0](LICENSE).
