# Changelog

All notable changes to this project are documented in this file. The format
is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [0.2.0] - 2026-10-15

### Added

- Kill running containers with `K`.
- Process list of a running container with `T`, refreshed every two seconds.
- Search in the log view with `/`, jumping between matches with `n` and `N`.
- Toggle timestamps in the log view with `t`.
- Docker CLI contexts: the current context (`DOCKER_CONTEXT` or
  `docker context use`) is honored, and `-context` selects one explicitly.
- Lists update as soon as the daemon reports a change, in addition to the
  periodic refresh.

## [0.1.0] - 2026-10-10

First release.

### Added

- Tabs for containers, images, volumes and networks.
- Container actions: start, stop, restart, pause/unpause and remove.
- Real-time log viewer that follows container output.
- Live stats view with CPU history, memory, network and block I/O.
- Interactive shell in running containers through the docker CLI.
- Inspect view with highlighted JSON for every object type.
- Incremental filtering, column sorting and copying IDs to the clipboard.
- Pruning of stopped containers, dangling images, unused volumes and networks.
- Container state colors, published ports and compose project columns.
- Periodic refresh of the visible list (`-refresh`).
- Connection over unix sockets and TCP, honoring `DOCKER_HOST` and the
  Docker Desktop, Colima and rootless socket locations.
