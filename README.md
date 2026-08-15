# forgejo-time-sync

Reads time entries logged against Forgejo issues and pushes matching
timesheets into [Kimai](https://www.kimai.org/). Built for
[migrate-from-gitlab-to-forgejo#46](https://git.ryankes.eu/alrayyes/migrate-from-gitlab-to-forgejo/issues/46):
billing for freelance work needs to attach to something, and Forgejo has no
webhook event for time tracking, so this runs as a scheduled pull rather than
reacting to anything.

**Staging on GitHub, private, for now.** It moves once Kimai has somewhere to
run — tracked in a `vps-docker` ticket — at which point this becomes a
Forgejo repo built and deployed the way `tempus-fugit` is.

## How it works

For each configured `owner/repo`, it calls Forgejo's
`GET /repos/{owner}/{repo}/times` and creates a Kimai timesheet for every
entry that doesn't already have one. Idempotency key: the Forgejo time-entry
ID, embedded in the Kimai timesheet's description. Safe to re-run — a rerun
with nothing new to push creates nothing.

One Kimai project per Forgejo repo is the starting mapping; there's no
per-issue activity mapping yet.

## Running the tests

```sh
python3 -m venv .venv && . .venv/bin/activate
pip install -e ".[dev]"
pytest -v
```

Needs a **Docker daemon** — the suite starts real `forgejo/forgejo` and
`kimai/kimai2` containers rather than mocking either side, matching this
org's `homepage` project's own e2e convention of testing against the real
thing.

## Dependencies

Pinned in `pyproject.toml`; `requirements-lock.txt` is the full resolved set
for `pip install -r requirements-lock.txt`.
