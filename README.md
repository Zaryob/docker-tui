# docker-tui

A fast, keyboard-driven terminal user interface for Docker, written in Go.

docker-tui talks to the Docker Engine API directly over its socket, so it is a
single static binary with no dependency on the Docker SDK.

## Features

- Browse **containers**, **images**, **volumes** and **networks** in tabs
- Start, stop, restart, kill, pause/unpause and remove containers
- Follow container **logs** in real time and search them
- Live **stats**: CPU (with history), memory, network and block I/O
- **Process list** of running containers, refreshed live
- Open an interactive **shell** inside a running container
- **Inspect** any object as pretty-printed JSON
- Incremental **filtering** and column **sorting**
- **Prune** stopped containers, dangling images, unused volumes and networks
- Automatic refresh of the visible list
- Works with local sockets, Docker Desktop, Colima, rootless Docker and
  remote daemons over TCP

## Installation

With Go 1.24 or newer:

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
docker-tui -context colima         # use the endpoint of a docker CLI context
docker-tui -refresh 10s          # refresh lists every 10 seconds (0 disables)
docker-tui -version
```

The daemon address is taken from, in order: the `-H`/`-host` flag, the
`-context` flag, the `DOCKER_HOST` environment variable, the docker CLI
context selected with `DOCKER_CONTEXT` or `docker context use`,
`/var/run/docker.sock`, and finally the per-user sockets of Docker Desktop
(`~/.docker/run/docker.sock`), Colima and rootless Docker. Opening a shell requires the `docker` CLI to be installed.

## Key bindings

| Key               | Action                                   |
| ----------------- | ---------------------------------------- |
| `tab` / `shift+tab`, `1`–`4` | Switch between tabs           |
| `↑`/`k`, `↓`/`j`  | Move the cursor                          |
| `pgup`, `pgdn`, `g`, `G` | Page up/down, jump to top/bottom  |
| `/`               | Filter the list (`enter` keeps, `esc` clears) |
| `o` / `O`         | Cycle sort column / reverse sort order   |
| `r`               | Refresh                                  |
| `enter`           | Show logs of the selected container      |
| `t`               | Show live stats of the selected container |
| `T`               | List the processes of the selected container |
| `e`               | Open a shell in the selected container   |
| `i`               | Inspect the selected object              |
| `y`               | Copy the ID of the selected object       |
| `s` / `x` / `R`   | Start / stop / restart a container       |
| `K`               | Kill a running container                 |
| `p`               | Pause or unpause a container             |
| `a`               | Toggle showing stopped containers        |
| `d`               | Remove the selected object               |
| `P`               | Prune unused objects of the current tab  |
| `/`, `n` / `N`    | In logs: search, next / previous match   |
| `t`               | In logs: toggle timestamps               |
| `esc`             | Close logs, stats or inspect             |
| `?`               | Toggle full help                         |
| `q`               | Quit                                     |

## Development

```sh
make test   # run the tests with the race detector
make vet
make run
```

## License

Copyright (C) 2026 Suleyman Poyraz

docker-tui is free software: you can redistribute it and/or modify it under
the terms of the GNU General Public License as published by the Free Software
Foundation, either version 3 of the License, or (at your option) any later
version.

This program is distributed in the hope that it will be useful, but WITHOUT
ANY WARRANTY; without even the implied warranty of MERCHANTABILITY or FITNESS
FOR A PARTICULAR PURPOSE. See [LICENSE](LICENSE) for the full text.
