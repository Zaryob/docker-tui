# docker-tui

A fast, keyboard-driven terminal user interface for Docker, written in Go.

docker-tui talks to the Docker Engine API directly over its socket, so it is a
single static binary with no dependency on the Docker SDK.

## Features

- Browse **containers**, **images**, **volumes**, **networks**, **builds** and
  **logs** and **Compose projects** in seven tabs
- Start, stop, restart, kill, pause/unpause and remove containers, one at a
  time or in bulk
- Containers of a **compose project** are grouped under a foldable project
  row, like in Docker Desktop; actions on the row apply to the whole project
- Run Compose project **up**, **down** and **rebuild**, with live progress and
  cancellation; open local Compose files and retain projects after `down`
- Follow container and build **logs** in real time, search with regex, toggle
  case sensitivity, inspect message details, and copy log lines or IDs
- Live **stats**: CPU (with history), memory, network and block I/O
- **Process list** of running containers, refreshed live
- **CPU and memory** of every running container in the list, summed per
  compose project
- **Copy files** to and from containers, like `docker cp`
- **Pull** images with live progress per layer
- **Build** images with BuildKit and browse the **build history** with its
  logs, like the Builds view of Docker Desktop
- Built-in **terminal** for running containers, like the Exec tab of Docker
  Desktop, with scrollback; **attach** to the main process of a container;
  or a full-screen shell through the docker CLI
- **Inspect** any object as pretty-printed JSON
- Incremental **filtering** and column **sorting**
- **Prune** stopped containers, dangling images, unused volumes and networks
- Lists update instantly on daemon events, plus a periodic refresh
- Works with local sockets, Docker Desktop, Colima, rootless Docker and
  remote daemons over TCP

## Installation

With Go 1.24.3 or newer:

```sh
go install github.com/zaryob/docker-tui@latest
```

Or build from source:

```sh
git clone https://github.com/zaryob/docker-tui.git
cd docker-tui
make build
sudo make install   # installs to /usr/local/bin
```

## Usage

```sh
docker-tui                       # connect using $DOCKER_HOST, the current context or the default socket
docker-tui -H tcp://10.0.0.5:2375
docker-tui -H ssh://deploy@build-server
docker-tui -context colima         # use the endpoint of a docker CLI context
docker-tui -refresh 10s          # refresh lists every 10 seconds (0 disables)
docker-tui -version
```

The daemon address is taken from, in order: the `-H`/`-host` flag, the
`-context` flag, the `DOCKER_HOST` environment variable, the docker CLI
context selected with `DOCKER_CONTEXT` or `docker context use`,
`/var/run/docker.sock`, and finally the per-user sockets of Docker Desktop
(`~/.docker/run/docker.sock`), Colima and rootless Docker.

Remote daemons are supported the same way as in the docker CLI:

- `tcp://` addresses use TLS when `DOCKER_TLS_VERIFY` is set, with `ca.pem`,
  `cert.pem` and `key.pem` from `DOCKER_CERT_PATH` (default `~/.docker`), or
  when the selected context has TLS material.
- `ssh://[user@]host[:port]` connects through `ssh` and runs
  `docker system dial-stdio` on the remote host, so your ssh keys, agent and
  `~/.ssh/config` apply. The remote host needs the docker CLI.

The built-in terminal talks to the daemon directly. The full-screen shell,
building images and the build history require the `docker` CLI (with
buildx) to be installed.

## Key bindings

| Key               | Action                                   |
| ----------------- | ---------------------------------------- |
| `tab` / `shift+tab`, `1`–`7` | Switch between tabs           |
| `↑`/`k`, `↓`/`j`  | Move the cursor                          |
| `pgup`, `pgdn`, `g`, `G` | Page up/down, jump to top/bottom  |
| `/`               | Filter the list (`enter` keeps, `esc` clears) |
| `space` / `ctrl+a` | Mark the selected / all visible objects (`esc` clears) |
| `C`               | Mark all containers of the selected compose project |
| `c`               | Fold or unfold the selected compose project |
| `o` / `O`         | Cycle sort column / reverse sort order   |
| `r`               | Refresh                                  |
| `enter`           | Show logs of the selected or marked containers |
| `t`               | Show live stats of the selected container |
| `T`               | List the processes of the selected container |
| `e`               | Open a terminal in the selected container (`ctrl+p ctrl+q` or `ctrl+]` leaves) |
| `f`               | Copy files between the selected container and this machine, like `docker cp` |
| `A`               | Attach to the main process of the selected container (`ctrl+p ctrl+q` or `ctrl+]` detaches) |
| `E`               | Open a full-screen shell with the docker CLI |
| `shift+↑/↓`, `ctrl+pgup/pgdn`, mouse wheel | In a terminal: scroll back through the output |
| `b`               | In images and builds: build an image, or show the running build |
| `u`               | In images: pull an image, or show the running pull (`x` cancels) |
| `u` / `enter`, `x`, `b` | In Projects: up, confirm down, rebuild |
| `f`               | In Projects: open a local Compose file |
| `x`               | In the build output: cancel the build (`esc` keeps it running) |
| `enter`           | In builds: show the logs of the build    |
| `i`               | Inspect the selected object              |
| `y`               | Copy the ID of the selected object       |
| `s` / `x` / `R`   | Start / stop / restart a container       |
| `K`               | Kill a running container                 |
| `p`               | Pause or unpause a container             |
| `a`               | Toggle showing stopped containers        |
| `d`               | Remove the selected object               |
| `P`               | Prune unused objects of the current tab  |
| `/`, `n` / `N`    | In logs: search, next / previous match   |
| `t`               | In container logs: toggle timestamps     |
| `esc`             | Close logs, stats or inspect             |
| `?`               | Toggle full help                         |
| `q`               | Quit                                     |

Actions such as start, stop, restart, kill, pause and remove apply to all
marked objects, or to the selected one when nothing is marked.

The Projects tab (7) discovers projects on the selected daemon. Operations need
Compose files on this machine; `f` opens a local file when daemon paths are
unavailable. Escape keeps an operation running; `u` reopens its output and `x`
cancels it. `down` preserves volumes and retains the project in the current
session so it can be started again. Compose requires the Docker CLI plugin.

## Development

```sh
make test                                 # run unit and race tests
go test -race -tags integration ./...    # run real-daemon integration tests
make vet
make run
```

Integration tests skip an unavailable daemon locally. CI sets
`DOCKER_TUI_INTEGRATION_REQUIRED=1` so missing prerequisites fail the run.
The lifecycle fixture always uses `alpine:3.21` and cleans up its own container.

## License

Copyright (C) 2026 Suleyman Poyraz

docker-tui is free software: you can redistribute it and/or modify it under
the terms of the GNU General Public License as published by the Free Software
Foundation, either version 3 of the License, or (at your option) any later
version.

This program is distributed in the hope that it will be useful, but WITHOUT
ANY WARRANTY; without even the implied warranty of MERCHANTABILITY or FITNESS
FOR A PARTICULAR PURPOSE. See [LICENSE](LICENSE) for the full text.
