# Changelog

All notable changes to this project are documented in this file. The format
is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [0.4.0] - 2026-10-15

### Added

- Builds tab with the build history of BuildKit: status, steps, cache use and
  duration; `enter` shows the logs of a build, `i` inspects it, `d` and
  `P` remove records.
- Build images with `b` from the Images or Builds tab, following the output
  live.
- Built-in terminal for running containers with `e`, rendered inside the
  TUI through a terminal emulator; `ctrl+p ctrl+q`, as in docker, or `ctrl+]`
  leaves it. The previous
  full-screen shell moved to `E`.
- Scroll back through the output of a terminal with `shift+↑/↓` and
  `ctrl+pgup/pgdn`; new output does not move the lines in view.
- Attach to the main process of a container with `A`, like `docker attach`;
  `ctrl+p ctrl+q` detaches without stopping it. Containers without a TTY show their
  output read-only.
- Closing the output of a build with `esc` keeps the build running in the
  background, shown in the status line; `b` shows it again and `x` cancels
  it.

## [0.3.0] - 2026-10-15

### Added

- Containers of a compose project are grouped under a project row showing
  how many of them run. Fold or unfold it with `c`; actions, marking and
  logs on the row apply to the whole project.

- Mark objects with `space`, or all visible ones with `ctrl+a`; `esc`
  clears the marks.
- Start, stop, restart, kill, pause/unpause and remove act on all marked
  objects at once, several at a time.
- Mark every container of the selected compose project with `C`.
- The log view follows all marked containers together, prefixing each line
  with the name of its container.

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
