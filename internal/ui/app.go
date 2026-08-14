// Copyright (C) 2026 Suleyman Poyraz
// SPDX-License-Identifier: GPL-3.0-or-later

// Package ui contains the Bubble Tea models that make up docker-tui.
package ui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/zaryob/docker-tui/internal/docker"
)

// App is the root Bubble Tea model.
type App struct {
	client     *docker.Client
	version    docker.Version
	containers []docker.Container
	table      *Table
	help       help.Model
	err        error
	width      int
	height     int
}

// New creates the root model.
func New(client *docker.Client) *App {
	return &App{
		client: client,
		help:   help.New(),
		table: NewTable(
			Column{Title: "CONTAINER ID", Width: 12},
			Column{Title: "NAME"},
			Column{Title: "IMAGE"},
			Column{Title: "STATE", Width: 10},
			Column{Title: "STATUS", Width: 24},
		),
	}
}

type actionMsg struct {
	status string
	err    error
}

type actionFunc func(ctx context.Context, id string) error

// containerAction runs fn against the selected container and reports the
// outcome as an actionMsg.
func (a *App) containerAction(verb string, fn actionFunc) tea.Cmd {
	c, ok := a.selectedContainer()
	if !ok {
		return nil
	}
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := fn(ctx, c.ID); err != nil {
			return actionMsg{err: fmt.Errorf("%s %s: %w", verb, c.Name(), err)}
		}
		return actionMsg{status: fmt.Sprintf("%s %s", verb, c.Name())}
	}
}

func (a *App) selectedContainer() (docker.Container, bool) {
	i := a.table.Cursor()
	if i < 0 || i >= len(a.containers) {
		return docker.Container{}, false
	}
	return a.containers[i], true
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
		a.help.Width = msg.Width
		a.layout()
	case versionMsg:
		a.version = msg.version
		if msg.err != nil {
			a.err = msg.err
		}
	case containersMsg:
		a.containers, a.err = msg.items, msg.err
		a.table.SetRows(containerRows(a.containers))
	case actionMsg:
		a.err = msg.err
		return a, a.loadContainers
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, keys.Quit):
			return a, tea.Quit
		case key.Matches(msg, keys.Help):
			a.help.ShowAll = !a.help.ShowAll
			a.layout()
		case key.Matches(msg, keys.Refresh):
			return a, a.loadContainers
		case key.Matches(msg, keys.Start):
			return a, a.containerAction("started", a.client.StartContainer)
		case key.Matches(msg, keys.Stop):
			return a, a.containerAction("stopped", a.client.StopContainer)
		case key.Matches(msg, keys.Restart):
			return a, a.containerAction("restarted", a.client.RestartContainer)
		case key.Matches(msg, keys.Up):
			a.table.Move(-1)
		case key.Matches(msg, keys.Down):
			a.table.Move(1)
		case key.Matches(msg, keys.PageUp):
			a.table.Move(-a.table.PageSize())
		case key.Matches(msg, keys.PageDown):
			a.table.Move(a.table.PageSize())
		case key.Matches(msg, keys.Top):
			a.table.Top()
		case key.Matches(msg, keys.Bottom):
			a.table.Bottom()
		}
	}
	return a, nil
}

// layout recomputes the size of the main table from the window size.
func (a *App) layout() {
	helpHeight := lipgloss.Height(a.help.View(keys))
	// header + status line + help
	a.table.SetSize(a.width, a.height-2-helpHeight)
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
	b.WriteString("\n")
	b.WriteString(a.table.View())
	b.WriteString("\n")
	if a.err != nil {
		b.WriteString(errorStyle.Render("error: " + a.err.Error()))
	} else {
		b.WriteString(statusStyle.Render(fmt.Sprintf("%d containers", len(a.containers))))
	}
	b.WriteString("\n")
	b.WriteString(a.help.View(keys))
	return b.String()
}

func containerRows(items []docker.Container) [][]string {
	rows := make([][]string, len(items))
	for i, c := range items {
		id := c.ID
		if len(id) > 12 {
			id = id[:12]
		}
		rows[i] = []string{id, c.Name(), c.Image, c.State, c.Status}
	}
	return rows
}
