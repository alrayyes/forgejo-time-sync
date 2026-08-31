## [1.4.1](https://github.com/alrayyes/forgejo-time-sync/compare/v1.4.0...v1.4.1) (2026-08-31)


### Bug Fixes

* **lint:** adopt the canonical .golangci.yml from rules/go-lint.md ([#31](https://github.com/alrayyes/forgejo-time-sync/issues/31)) ([1fa8ab3](https://github.com/alrayyes/forgejo-time-sync/commit/1fa8ab33e1692f14d890e945096a8fdbfed8ff4b))

# [1.4.0](https://github.com/alrayyes/forgejo-time-sync/compare/v1.3.4...v1.4.0) (2026-08-31)


### Features

* **cli:** parse the command line with Cobra, load config through Viper ([#30](https://github.com/alrayyes/forgejo-time-sync/issues/30)) ([901d606](https://github.com/alrayyes/forgejo-time-sync/commit/901d606ea92fcfc42340aa8f98ab3e8ba362b540))

## [1.3.4](https://github.com/alrayyes/forgejo-time-sync/compare/v1.3.3...v1.3.4) (2026-08-31)


### Bug Fixes

* **deps:** align go toolchain version across go.mod, CI, and docs ([#27](https://github.com/alrayyes/forgejo-time-sync/issues/27)) ([eb815b2](https://github.com/alrayyes/forgejo-time-sync/commit/eb815b218fe499d934630eda45a201dd53bea629))

## [1.3.3](https://github.com/alrayyes/forgejo-time-sync/compare/v1.3.2...v1.3.3) (2026-08-31)


### Bug Fixes

* **deps:** bump golang from `65b6f28` to `4013ae0` ([#26](https://github.com/alrayyes/forgejo-time-sync/issues/26)) ([07e8a2f](https://github.com/alrayyes/forgejo-time-sync/commit/07e8a2f334fb007a0f745bb1b4b26798f6acee3e))
* **deps:** bump golang from 1.26.6 to 1.27.0 ([#24](https://github.com/alrayyes/forgejo-time-sync/issues/24)) ([86c4d5a](https://github.com/alrayyes/forgejo-time-sync/commit/86c4d5a0d359d1c0731b27be5bea94c10483e160))

## [1.3.2](https://github.com/alrayyes/forgejo-time-sync/compare/v1.3.1...v1.3.2) (2026-08-21)


### Bug Fixes

* **ci:** skip codecov upload on dependabot-triggered runs ([#22](https://github.com/alrayyes/forgejo-time-sync/issues/22)) ([53cb040](https://github.com/alrayyes/forgejo-time-sync/commit/53cb0401ec4f5ba3ba0384281c4d6f8df4ebea41))
* **deps:** bump the go-dependencies group with 2 updates ([#16](https://github.com/alrayyes/forgejo-time-sync/issues/16)) ([7697073](https://github.com/alrayyes/forgejo-time-sync/commit/76970731b18d886c904014ca1600de8b7a0d888b))

## [1.3.1](https://github.com/alrayyes/forgejo-time-sync/compare/v1.3.0...v1.3.1) (2026-08-18)


### Bug Fixes

* **toggl:** migrate from Track v9 to the 2.0 (Focus) API ([96b5cdc](https://github.com/alrayyes/forgejo-time-sync/commit/96b5cdc520529182edc489597e6235d50ae7215a))

# [1.3.0](https://github.com/alrayyes/forgejo-time-sync/compare/v1.2.1...v1.3.0) (2026-08-17)


### Features

* add a Docker HEALTHCHECK ([395c07f](https://github.com/alrayyes/forgejo-time-sync/commit/395c07f7d5a700a83b5b4a539fcade3235fb6f37))

## [1.2.1](https://github.com/alrayyes/forgejo-time-sync/compare/v1.2.0...v1.2.1) (2026-08-17)


### Bug Fixes

* **docker:** fix /data volume ownership so state actually persists ([c1f158b](https://github.com/alrayyes/forgejo-time-sync/commit/c1f158bae39963517d4db93862492c6fdb5dcc07))

# [1.2.0](https://github.com/alrayyes/forgejo-time-sync/compare/v1.1.0...v1.2.0) (2026-08-17)


### Features

* **toggl:** auto-provision a client/project when none is configured ([0b3c064](https://github.com/alrayyes/forgejo-time-sync/commit/0b3c064914c84cf2676a9344babc2d5163af9f85))

# [1.1.0](https://github.com/alrayyes/forgejo-time-sync/compare/v1.0.0...v1.1.0) (2026-08-17)


### Features

* **toggl:** tag entries with the issue reference, not just the description ([36b88ec](https://github.com/alrayyes/forgejo-time-sync/commit/36b88ec4494fe671446f390b94eb101f890cdd48)), closes [owner/repo#N](https://github.com/owner/repo/issues/N)

# 1.0.0 (2026-08-17)


### Bug Fixes

* pin go, container images and CI actions to exact versions ([b8d9db4](https://github.com/alrayyes/forgejo-time-sync/commit/b8d9db41c2615c560638d85d628b2dcf192d7f41))
* target go 1.26.6, which fixes real stdlib CVEs ([e4b6b59](https://github.com/alrayyes/forgejo-time-sync/commit/e4b6b59ac9838e9b626f90fb746022290ca1db25))


### Features

* **cmd:** poll daemon entrypoint ([48fadc7](https://github.com/alrayyes/forgejo-time-sync/commit/48fadc73e1256c869b3240b12d71e80c6171a1e5))
* **sync:** forgejo and toggl clients with rate-limited, state-backed sync ([8822f08](https://github.com/alrayyes/forgejo-time-sync/commit/8822f080910eff767b6a33149f4361dd8a140bc7))
* **sync:** pull Forgejo time entries into Kimai timesheets ([40de3f1](https://github.com/alrayyes/forgejo-time-sync/commit/40de3f1b230ee50654504ca7aa608bba3985d76b))
