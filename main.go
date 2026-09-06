// Copyright (C) 2026 Suleyman Poyraz
// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"context"
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/zaryob/docker-tui/internal/docker"
	"github.com/zaryob/docker-tui/internal/ui"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "docker-tui:", err)
		os.Exit(1)
	}
}

func run() error {
	client, err := docker.NewClient("")
	if err != nil {
		return err
	}
	if err := client.Ping(context.Background()); err != nil {
		return fmt.Errorf("cannot reach docker daemon at %s: %w", client.Host(), err)
	}
	p := tea.NewProgram(ui.New(client), tea.WithAltScreen())
	_, err = p.Run()
	return err
}
