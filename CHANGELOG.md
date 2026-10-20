# Changelog

All notable changes to this project are documented in this file. The format
is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [Unreleased]

### Added

- Projects tab (7) discovers Compose projects on the selected daemon and runs
  up, confirmed down and rebuild with streamed, cancelable progress. Local
  Compose files can be opened and projects remain accessible after down.
- Real-daemon Compose lifecycle smoke test, required by CI and release gates.

### Fixed

- Real-daemon tests use a known Alpine fixture and the selected daemon context,
  assert log/process/stop/removal behavior, and fail on unavailable prerequisites
  in CI instead of silently skipping integration coverage.

## [0.6.3] - 2026-10-16

### Added

- Opt-in real-daemon integration tests (`go test -race -tags integration ./...`)
  testing ping, resource listing, and isolated container lifecycle with archive transfer.
- Required validation steps (format check, vet, unit tests, integration tests)
  before tagged release publication in CI.

### Fixed

- Cancel stalled SSH interactive handshakes and connection attempts on context
  cancellation without terminating established terminal sessions.
- Documentation in README for six tabs, `1`–`6` shortcut keys, and integration test command.

## [0.6.2] - 2026-10-16

### Fixed

- Container archive extraction is confined to the selected directory, including
  file and directory symlinks. Traversal entries are rejected explicitly.
- The minimum Go version is 1.24.3, which fixes the `os.Root` parent-directory
  escape (CVE-2025-22873).
- Toggling timestamps in build output no longer cancels or starts a build again.
- Logs snapshots report source and scanner failures while retaining usable
  partial results. Log messages have identities distinct from their sources.
- Log selection and scroll position stay attached to the same messages when
  older output is discarded, and Escape closes message details locally.
- The Logs tab no longer offers object removal; copying an ID copies the
  underlying container or build ID.

## [0.6.1] - 2026-10-15

### Added

- Logs tab (Tab 6) with structured Docker log viewer, regex search (`/pattern/`), case sensitivity toggle (`Aa`), row selection, line detail inspection, and clipboard export (`y`/`c`).
- Geometric ASCII Docker whale logo with tail and container blocks in the header, fixed at 12 columns.
- Modern capsule tab bar styling with colored active tab badges and symmetric separators.

## [0.6.0] - 2026-10-15

### Added

- TLS connections to `tcp://` daemons: `DOCKER_TLS_VERIFY`,
  `DOCKER_CERT_PATH` and `DOCKER_TLS` work like in the docker CLI, and
  contexts with TLS material are used with their certificates. The default
  port is 2376 with TLS and 2375 without.
- `ssh://[user@]host[:port]` daemons, through `ssh` and
  `docker system dial-stdio` on the remote host; ssh errors such as a
  denied key are shown as they are.
- Builds and the full-screen shell select the same context as docker-tui,
  so they also reach TLS and ssh daemons.

## [0.5.0] - 2026-10-15

### Added

- Pull images with `u` in the Images tab, following the progress of every
  layer. Like builds, `esc` keeps the pull running in the background and
  `x` cancels it.
- CPU and memory columns in the container list, refreshed every few seconds
  and summed on compose project rows; sorting on them is numeric.
- Copy files and directories between a container and the local machine with
  `f`, using the `docker cp` syntax `name:/path`; `ctrl+s` swaps the
  direction. Works with stopped containers too.

## [0.4.1] - 2026-10-15

### Added

- Scroll the output of a terminal with the mouse wheel. Full-screen programs
  such as `less` receive arrow keys instead. Mouse reporting is only on while
  a terminal is shown, so text can still be selected elsewhere.

### Fixed

- Keys typed while a terminal was connecting, or right as the shell started,
  were lost. They are now queued and sent once the shell is ready.

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
