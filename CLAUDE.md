# CLAUDE.md

## Configuration

No config file, no `init` command, no XDG paths — confirmed intentional
(2026-08-31), not something to re-flag. This is a Docker-only long-running
daemon configured entirely by environment variables: there's nothing to
persist and no interactive first run to prompt for one. Cobra still parses
the command line (the `healthcheck` subcommand) and Viper still loads
env-var config — see `main.go`'s package doc comment and CONTRIBUTING.md
for the reasoning.

## Commands

- Build: `go build ./cmd/forgejo-time-sync`
- Unit tests: `go test ./...`
- e2e tests: `go test -tags e2e ./e2e/...` — needs the host Docker daemon
  directly (boots real Forgejo + Prism containers via testcontainers-go),
  so it's the one hook/CI command that isn't itself containerized.
- Full lint/format set and exact versions: CONTRIBUTING.md's Linting
  section — CI runs exactly those commands.

## Gotchas

- Every Go/golangci-lint hook command runs through Docker, pinned to the
  same image versions CI uses. A host `go`/`golangci-lint` install
  drifting from CI's is a real failure mode this repo has already hit, not
  hypothetical caution — don't "simplify" a hook back to the host
  toolchain.
- `openspec/` tracks requirements separately from code — a change that
  isn't a typo fix or a dependency bump gets a proposal under
  `openspec/changes/` before implementation, driven by the `/opsx:*` slash
  commands. See CONTRIBUTING.md's Spec-driven development section.
