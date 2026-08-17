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
