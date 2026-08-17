# Contributing

## Toolchain

- Go 1.26+
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
Toggl's own vendored OpenAPI spec — see `e2e/testdata/toggl-openapi.json`
and `scripts/vendor-toggl-spec.sh` for how to refresh it. It does not call
the real Toggl API.

## Linting

```sh
golangci-lint run ./...
golangci-lint fmt ./...   # gofumpt + goimports
go run golang.org/x/vuln/cmd/govulncheck@v1.7.0 ./...
docker run --rm -i hadolint/hadolint:v2.15.1@sha256:32dac94127fd60b7b7e3fbfc65e1383b9b5e25c9bfd7b8536de7a539fe68a12d < Dockerfile
bun run format:check      # prettier, markdown and yaml
bun run lint:md           # markdownlint
```

CI runs exactly these commands — see `.github/workflows/ci.yml`. The git
hooks in `lefthook.yml` run the fast subset on commit and the rest on push,
so a red pipeline should never be a surprise.

## Commits and branches

[Conventional Commits](https://www.conventionalcommits.org/), enforced by
commitlint on every commit message. One logical change per commit; one
feature per pull request. Work lands through a pull request — nothing gets
pushed straight to `main`.

## Releases

This ships as a container image (`docker build .`), not a distributed
binary, so there's no goreleaser/release-please pipeline here — pull the
image built off whatever commit you want to run.
