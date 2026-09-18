// Copyright (C) 2026 Suleyman Poyraz
// SPDX-License-Identifier: GPL-3.0-or-later

package ui

import (
	"context"
	"fmt"
	"time"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/zaryob/docker-tui/internal/docker"
	"github.com/zaryob/docker-tui/internal/format"
)

func newContainersTab() *tab {
	return newTab(tabContainers, "Containers", "containers",
		Column{Title: "CONTAINER ID", Width: 12},
		Column{Title: "NAME"},
		Column{Title: "IMAGE"},
		Column{Title: "STATE", Width: 10},
		Column{Title: "STATUS", Width: 24},
	)
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
			id:    c.ID,
			name:  c.Name(),
			cells: []string{format.ShortID(c.ID), c.Name(), c.Image, c.State, c.Status},
			// Sort the status column by creation time.
			sortKeys: []string{4: fmt.Sprintf("%020d", c.Created)},
			data:     c,
		}
	}
	return items
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
	case key.Matches(msg, keys.Start):
		return a.objectAction("started", a.client.StartContainer)
	case key.Matches(msg, keys.Stop):
		return a.objectAction("stopped", a.client.StopContainer)
	case key.Matches(msg, keys.Restart):
		return a.objectAction("restarted", a.client.RestartContainer)
	case key.Matches(msg, keys.Pause):
		it, ok := a.current().selected()
		if !ok {
			return nil
		}
		if it.data.(docker.Container).State == "paused" {
			return a.objectAction("unpaused", a.client.UnpauseContainer)
		}
		return a.objectAction("paused", a.client.PauseContainer)
	}
	return nil
}
