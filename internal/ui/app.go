// Copyright (C) 2026 Suleyman Poyraz
// SPDX-License-Identifier: GPL-3.0-or-later

// Package ui contains the Bubble Tea models that make up docker-tui.
package ui

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/zaryob/docker-tui/internal/docker"
)

// App is the root Bubble Tea model.
type App struct {
	client     *docker.Client
	containers []docker.Container
	err        error
	width      int
	height     int
}

// New creates the root model.
func New(client *docker.Client) *App {
	return &App{client: client}
}

type containersMsg struct {
	items []docker.Container
	err   error
}

func (a *App) loadContainers() tea.Msg {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	items, err := a.client.ListContainers(ctx, true)
	return containersMsg{items: items, err: err}
}

// Init implements tea.Model.
func (a *App) Init() tea.Cmd {
	return a.loadContainers
}

// Update implements tea.Model.
func (a *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		a.width, a.height = msg.Width, msg.Height
	case containersMsg:
		a.containers, a.err = msg.items, msg.err
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return a, tea.Quit
		case "r":
			return a, a.loadContainers
		}
	}
	return a, nil
}

// View implements tea.Model.
func (a *App) View() string {
	var b strings.Builder
	b.WriteString("docker-tui\n\n")
	if a.err != nil {
		fmt.Fprintf(&b, "error: %v\n", a.err)
	}
	for _, c := range a.containers {
		fmt.Fprintf(&b, "%-12.12s  %-30s  %-10s  %s\n", c.ID, c.Name(), c.State, c.Image)
	}
	b.WriteString("\nr: refresh  q: quit\n")
	return b.String()
}
