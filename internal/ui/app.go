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
	"github.com/charmbracelet/lipgloss"

	"github.com/zaryob/docker-tui/internal/docker"
)

// App is the root Bubble Tea model.
type App struct {
	client     *docker.Client
	version    docker.Version
	containers []docker.Container
	err        error
	width      int
	height     int
}

// New creates the root model.
func New(client *docker.Client) *App {
	return &App{client: client}
}

type versionMsg struct {
	version docker.Version
	err     error
}

type containersMsg struct {
	items []docker.Container
	err   error
}

func (a *App) loadVersion() tea.Msg {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	v, err := a.client.ServerVersion(ctx)
	return versionMsg{version: v, err: err}
}

func (a *App) loadContainers() tea.Msg {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	items, err := a.client.ListContainers(ctx, true)
	return containersMsg{items: items, err: err}
}

// Init implements tea.Model.
func (a *App) Init() tea.Cmd {
	return tea.Batch(a.loadVersion, a.loadContainers)
}

// Update implements tea.Model.
func (a *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		a.width, a.height = msg.Width, msg.Height
	case versionMsg:
		a.version = msg.version
		if msg.err != nil {
			a.err = msg.err
		}
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

func (a *App) header() string {
	title := titleStyle.Render("docker-tui")
	info := "connecting…"
	if a.version.Version != "" {
		info = fmt.Sprintf("Docker %s · API %s · %s/%s",
			a.version.Version, a.version.APIVersion, a.version.Os, a.version.Arch)
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, title, headerInfoStyle.Render(info))
}

// View implements tea.Model.
func (a *App) View() string {
	var b strings.Builder
	b.WriteString(a.header())
	b.WriteString("\n\n")
	for _, c := range a.containers {
		fmt.Fprintf(&b, "%-12.12s  %-30s  %-10s  %s\n", c.ID, c.Name(), c.State, c.Image)
	}
	b.WriteString("\n")
	if a.err != nil {
		b.WriteString(errorStyle.Render("error: " + a.err.Error()))
	} else {
		b.WriteString(statusStyle.Render(fmt.Sprintf("%d containers", len(a.containers))))
	}
	b.WriteString("\n")
	b.WriteString(statusStyle.Render("r: refresh  q: quit"))
	return b.String()
}
