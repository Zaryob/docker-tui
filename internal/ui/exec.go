// Copyright (C) 2026 Suleyman Poyraz
// SPDX-License-Identifier: GPL-3.0-or-later

package ui

import (
	"fmt"
	"os"
	"os/exec"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/zaryob/docker-tui/internal/buildx"
	"github.com/zaryob/docker-tui/internal/docker"
)

// shellScript starts the best shell available in the container.
const shellScript = "if command -v bash >/dev/null 2>&1; then exec bash; else exec sh; fi"

type execDoneMsg struct {
	name string
	err  error
}

// execShell suspends the TUI and opens an interactive shell in the selected
// container using the docker CLI.
func (a *App) execShell() tea.Cmd {
	it, ok := a.selectedObject()
	if !ok {
		return nil
	}
	if c := it.data.(docker.Container); c.State != "running" {
		return a.setStatus(fmt.Sprintf("%s is not running", it.name), true)
	}
	bin, err := exec.LookPath("docker")
	if err != nil {
		return a.setStatus("docker CLI not found in PATH; it is required for exec", true)
	}
	cmd := exec.Command(bin, "exec", "-it", it.id, "sh", "-c", shellScript)
	// Make the CLI talk to the same daemon as we do. DOCKER_HOST already
	// identifies it, so drop a context selection that could conflict.
	cmd.Env = buildx.Env(os.Environ(), a.client.Host())
	name := it.name
	return tea.ExecProcess(cmd, func(err error) tea.Msg {
		return execDoneMsg{name: name, err: err}
	})
}
