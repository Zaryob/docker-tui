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
	containerNameCol   = 1
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
		switch col {
		case containerStateCol:
			return stateStyle(value)
		case containerNameCol:
			if strings.HasPrefix(value, groupOpen) || strings.HasPrefix(value, groupClosed) {
				return groupStyle, true
			}
		}
		return lipgloss.Style{}, false
	})
	t.group = &grouping{
		key:        containerProject,
		nameCol:    containerNameCol,
		memberName: composeMemberName,
		row:        projectRow,
	}
	return t
}

// Markers in front of the name of a compose project row.
const (
	groupOpen   = "▾ "
	groupClosed = "▸ "
)

var groupStyle = lipgloss.NewStyle().Bold(true)

// containerProject returns the compose project of a container or of a
// project row, or "" for a standalone container.
func containerProject(it item) string {
	if it.isGroup() {
		return strings.TrimPrefix(it.id, groupIDPrefix)
	}
	c, _ := it.data.(docker.Container)
	return c.Labels[composeProjectLabel]
}

// composeMemberName shortens the name of a container inside its project row,
// e.g. "shop-web-1" becomes "web-1" like in Docker Desktop.
func composeMemberName(it item, project string) string {
	if short := strings.TrimPrefix(it.name, project+"-"); short != "" {
		return short
	}
	return it.name
}

// projectRow summarizes the containers of a compose project: it is shown as
// running while any of them runs, with the number of running containers as
// status.
func projectRow(project string, members []item, collapsed bool) item {
	running, state := 0, ""
	var ports []string
	for _, m := range members {
		c := m.data.(docker.Container)
		if c.State == "running" {
			running++
		}
		if state == "" || c.State == "running" {
			state = c.State
		}
		if p := m.cells[len(m.cells)-1]; p != "" {
			ports = append(ports, p)
		}
	}
	marker := groupOpen
	if collapsed {
		marker = groupClosed
	}
	return item{
		name: project,
		cells: []string{
			"",
			marker + project,
			project,
			"",
			state,
			fmt.Sprintf("%d/%d running", running, len(members)),
			strings.Join(ports, ", "),
		},
		members: members,
	}
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
	project := containerProject(it)
	if project == "" {
		return a.setStatus(it.name+" is not part of a compose project", true)
	}
	n := t.markWhere(func(it item) bool {
		return containerProject(it) == project
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
		return a.openTerminal()
	case key.Matches(msg, keys.ExecCLI):
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
	case key.Matches(msg, keys.Collapse):
		a.current().toggleCollapse()
	case key.Matches(msg, keys.MarkProject):
		return a.markProject()
	case key.Matches(msg, keys.Kill):
		a.confirm = a.killDialog()
	case key.Matches(msg, keys.Pause):
		return a.togglePause()
	}
	return nil
}
