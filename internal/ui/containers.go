// Copyright (C) 2026 Suleyman Poyraz
// SPDX-License-Identifier: GPL-3.0-or-later

package ui

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/zaryob/docker-tui/internal/docker"
	"github.com/zaryob/docker-tui/internal/format"
)

const (
	containerStateCol  = 4
	containerStatusCol = 5

	composeProjectLabel = "com.docker.compose.project"
)

// stateStyle returns the color for a container state.
func stateStyle(state string) (lipgloss.Style, bool) {
	switch state {
	case "running":
		return okStyle, true
	case "paused", "restarting":
		return warnStyle, true
	case "exited", "dead":
		return errorStyle, true
	case "created", "removing":
		return statusStyle, true
	}
	return lipgloss.Style{}, false
}

func newContainersTab() *tab {
	t := newTab(tabContainers, "Containers", "containers",
		Column{Title: "CONTAINER ID", Width: 12},
		Column{Title: "NAME", Grow: 3},
		Column{Title: "PROJECT", Width: 14},
		Column{Title: "IMAGE", Grow: 3},
		Column{Title: "STATE", Width: 10},
		Column{Title: "STATUS", Width: 24},
		Column{Title: "PORTS", Grow: 2},
	)
	t.table.SetCellStyle(func(col int, value string) (lipgloss.Style, bool) {
		if col == containerStateCol {
			return stateStyle(value)
		}
		return lipgloss.Style{}, false
	})
	return t
}

func (a *App) loadContainers() tea.Msg {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	list, err := a.client.ListContainers(ctx, !a.onlyRunning)
	return listMsg{kind: tabContainers, items: containerItems(list), err: err}
}

func containerItems(list []docker.Container) []item {
	items := make([]item, len(list))
	for i, c := range list {
		items[i] = item{
			id:   c.ID,
			name: c.Name(),
			cells: []string{
				format.ShortID(c.ID),
				c.Name(),
				c.Labels[composeProjectLabel],
				c.Image,
				c.State,
				c.Status,
				formatPorts(c.Ports),
			},
			// Sort the status column by creation time.
			sortKeys: []string{containerStatusCol: fmt.Sprintf("%020d", c.Created)},
			data:     c,
		}
	}
	return items
}

// formatPorts renders port mappings compactly, e.g. "8080->80/tcp, 443/tcp".
// IPv4 and IPv6 bindings of the same mapping are shown once.
func formatPorts(ports []docker.Port) string {
	seen := make(map[string]bool, len(ports))
	var out []string
	sorted := slices.Clone(ports)
	slices.SortFunc(sorted, func(a, b docker.Port) int {
		if a.PrivatePort != b.PrivatePort {
			return int(a.PrivatePort) - int(b.PrivatePort)
		}
		return strings.Compare(a.Type, b.Type)
	})
	for _, p := range sorted {
		s := fmt.Sprintf("%d/%s", p.PrivatePort, p.Type)
		if p.PublicPort != 0 {
			s = fmt.Sprintf("%d->%s", p.PublicPort, s)
		}
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return strings.Join(out, ", ")
}

// killDialog asks for confirmation before killing the targeted containers
// that are running.
func (a *App) killDialog() *confirmDialog {
	var items []item
	for _, it := range a.targets() {
		if c := it.data.(docker.Container); c.State == "running" || c.State == "restarting" {
			items = append(items, it)
		}
	}
	if len(items) == 0 {
		return nil
	}
	title, what := "Kill container?", items[0].name+" will be"
	if len(items) > 1 {
		title = "Kill " + countNoun(len(items), "containers") + "?"
		what = describeItems(items, "containers") + "\n\nThey will be"
	}
	return &confirmDialog{
		title:   title,
		message: what + " stopped immediately with SIGKILL.",
		onYes: func() tea.Cmd {
			return a.itemsAction("killed", items, a.client.KillContainer)
		},
	}
}

// markProject marks every container of the compose project of the selected
// container, so that the next action applies to the whole project.
func (a *App) markProject() tea.Cmd {
	t := a.current()
	it, ok := t.selected()
	if !ok {
		return nil
	}
	project := it.data.(docker.Container).Labels[composeProjectLabel]
	if project == "" {
		return a.setStatus(it.name+" is not part of a compose project", true)
	}
	n := t.markWhere(func(it item) bool {
		return it.data.(docker.Container).Labels[composeProjectLabel] == project
	})
	return a.setStatus(fmt.Sprintf("marked %s of project %s", countNoun(n, "containers"), project), false)
}

// togglePause pauses the targeted containers, or unpauses them if they are
// all paused.
func (a *App) togglePause() tea.Cmd {
	items := a.targets()
	if len(items) == 0 {
		return nil
	}
	allPaused := true
	for _, it := range items {
		allPaused = allPaused && it.data.(docker.Container).State == "paused"
	}
	if allPaused {
		return a.itemsAction("unpaused", items, a.client.UnpauseContainer)
	}
	var running []item
	for _, it := range items {
		if it.data.(docker.Container).State == "running" {
			running = append(running, it)
		}
	}
	return a.itemsAction("paused", running, a.client.PauseContainer)
}

func (a *App) containerKeys(msg tea.KeyMsg) tea.Cmd {
	switch {
	case key.Matches(msg, keys.ToggleAll):
		a.onlyRunning = !a.onlyRunning
		if a.onlyRunning {
			return tea.Batch(a.setStatus("showing running containers", false), a.loadContainers)
		}
		return tea.Batch(a.setStatus("showing all containers", false), a.loadContainers)
	case key.Matches(msg, keys.Logs):
		return a.openLogs()
	case key.Matches(msg, keys.Exec):
		return a.execShell()
	case key.Matches(msg, keys.Stats):
		return a.openStats()
	case key.Matches(msg, keys.Processes):
		return a.openTop()
	case key.Matches(msg, keys.Start):
		return a.objectAction("started", a.client.StartContainer)
	case key.Matches(msg, keys.Stop):
		return a.objectAction("stopped", a.client.StopContainer)
	case key.Matches(msg, keys.Restart):
		return a.objectAction("restarted", a.client.RestartContainer)
	case key.Matches(msg, keys.MarkProject):
		return a.markProject()
	case key.Matches(msg, keys.Kill):
		a.confirm = a.killDialog()
	case key.Matches(msg, keys.Pause):
		return a.togglePause()
	}
	return nil
}
