# Contributing

## Toolchain

- Go 1.27+
- Docker, for the container/integration test layer (`testcontainers-go`)
  and for building the image
- [Bun](https://bun.sh) 1.3.14, for the documentation tooling (Prettier,
  markdownlint, commitlint) — nothing here is a JavaScript project

```sh
go mod download
bun install
bun run prepare # installs the git hooks (lefthook)
```

## Building

```sh
go build ./cmd/forgejo-time-sync
docker build -t forgejo-time-sync .
```

## Testing

```sh
go test ./...               # unit tests, no Docker needed
go test -tags e2e ./e2e/...  # container/integration layer, needs Docker
go test -race -coverprofile=coverage.out ./... && go tool cover -html=coverage.out
```

The e2e suite boots a real Forgejo container and a
[Prism](https://stoplight.io/open-source/prism) mock server loaded with
Toggl's own vendored Focus (2.0) API OpenAPI spec — see
`e2e/testdata/focus-openapi.json` and `scripts/vendor-focus-spec.sh` for how
to refresh it. It does not call the real Toggl API.

## Linting

```sh
golangci-lint run ./...
golangci-lint fmt ./...   # gofumpt + goimports
go run golang.org/x/vuln/cmd/govulncheck@v1.7.0 ./...
docker run --rm -i hadolint/hadolint:v2.15.1@sha256:32dac94127fd60b7b7e3fbfc65e1383b9b5e25c9bfd7b8536de7a539fe68a12d < Dockerfile
docker build .             # hadolint reads the Dockerfile as text, this proves it builds
bun run format:check      # prettier, markdown and yaml
bun run lint:md           # markdownlint
bun audit                 # JS tooling dependency vulnerabilities
```

CI runs exactly these commands — see `.github/workflows/ci.yml`. The git
hooks in `lefthook.yml` run the fast subset on commit and the rest on push,
so a red pipeline should never be a surprise.

## Spec-driven development

[OpenSpec](https://github.com/Fission-AI/OpenSpec) tracks requirements
separately from code, under `openspec/`. `openspec/specs/` is the current
baseline — what the system does today. A change that isn't a typo fix or a
dependency bump starts as a proposal under `openspec/changes/<name>/`
(`proposal.md`, spec deltas, `tasks.md`), gets implemented against it, then
archives into `openspec/specs/` once it ships.

Claude Code sessions in this repo get the workflow as `/opsx:*` slash
commands (`.claude/commands/opsx/`); anyone else drives it with the
`openspec` CLI directly (`bunx @fission-ai/openspec@1.11.0 ...`, or
`bunx openspec ...` once installed as a devDependency).

## Commits and branches

[Conventional Commits](https://www.conventionalcommits.org/), enforced by
commitlint on every commit message. One logical change per commit; one
feature per pull request. Work lands through a pull request — nothing gets
pushed straight to `main`.

## Releases

[semantic-release](https://semantic-release.gitbook.io/) decides the
version from the Conventional Commits on `main`: `feat:` cuts a minor,
`fix:` a patch, a `BREAKING CHANGE:` footer a major, and a push of only
`docs:`/`chore:` releases nothing. Nobody picks a version by hand.

Merging to `main` is what triggers it — see `.github/workflows/release.yml`.
It tags the release, writes `CHANGELOG.md`, creates the GitHub release, and
on a successful release builds and pushes the image to
`ghcr.io/alrayyes/forgejo-time-sync`, tagged with both the version and
`latest`. There's no goreleaser here since there's no binary artifact for
it to build — the image is the release artifact.
