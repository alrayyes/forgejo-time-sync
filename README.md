# forgejo-time-sync

[![CI](https://github.com/alrayyes/forgejo-time-sync/actions/workflows/ci.yml/badge.svg)](https://github.com/alrayyes/forgejo-time-sync/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/alrayyes/forgejo-time-sync.svg)](https://pkg.go.dev/github.com/alrayyes/forgejo-time-sync)
[![License: GPL-3.0](https://img.shields.io/badge/license-GPL--3.0-blue.svg)](LICENSE)

Polls a Forgejo repo's tracked time and pushes any entry that isn't already
in Toggl Track into it. Packaged as a container you start once and leave
running — it polls on its own timer, there's nothing to re-invoke.

Forgejo has no webhook event for time tracking, so this has to poll rather
than react. Built for
[migrate-from-gitlab-to-forgejo#46](https://git.higherlearning.eu/alrayyes/migrate-from-gitlab-to-forgejo/issues/46).

## How it works

Every `SYNC_INTERVAL_SECONDS`, it calls Forgejo's
`GET /repos/{owner}/{repo}/times` and creates a Toggl time entry for any
entry not already recorded in local state. That local state — not asking
Toggl what already exists — is what makes a fast poll interval possible at
all: Toggl Track's API caps the free tier at 30 requests/hour (see
[Toggl's rate-limit docs](https://support.toggl.com/api-webhook-limits)), so
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

## Requirements

- A Forgejo instance with an API token that can read the target repo's
  tracked time.
- A Toggl Track account, an API token, and the workspace/project IDs you
  want entries created in.
- Docker (or Go 1.26+ and a Docker daemon, if building from source — the
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
| `TOGGL_WORKSPACE_ID`          | yes      | —                  |
| `TOGGL_PROJECT_ID`            | yes      | —                  |
| `SYNC_INTERVAL_SECONDS`       | no       | `10`               |
| `STATE_FILE_PATH`             | no       | `/data/state.json` |
| `TOGGL_MAX_REQUESTS_PER_HOUR` | no       | `30`               |

## Usage

Once running, it logs a structured JSON line per poll that finds something
new:

```json
{ "level": "INFO", "msg": "synced new time entries", "count": 1 }
```

It shuts down cleanly on `SIGTERM`/`SIGINT` — `docker compose down` or
`docker stop` are safe.

## Testing

```sh
go test ./...              # fast, no Docker needed
go test -tags e2e ./e2e/... # real Forgejo + a Toggl-spec-mocked Prism container
```

The e2e suite doesn't call the real Toggl API — it mocks against Toggl's own
published OpenAPI spec (vendored in `e2e/testdata/`, refreshed by
`scripts/vendor-toggl-spec.sh`) via [Prism](https://stoplight.io/open-source/prism),
so requests are validated against Toggl's real contract without CI
depending on Toggl's uptime or eating into its rate limit on every run. If
that vendored spec ever drifts far enough from reality to matter, the
fallback is testing against a real Toggl test workspace instead — not
needed yet.

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md).

## License

[GPL-3.0](LICENSE).
