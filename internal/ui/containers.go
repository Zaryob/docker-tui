// Copyright (C) 2026 Suleyman Poyraz
// SPDX-License-Identifier: GPL-3.0-or-later

package ui

import (
	"context"
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
	list, err := a.client.ListContainers(ctx, true)
	return listMsg{kind: tabContainers, items: containerItems(list), err: err}
}

func containerItems(list []docker.Container) []item {
	items := make([]item, len(list))
	for i, c := range list {
		items[i] = item{
			id:    c.ID,
			name:  c.Name(),
			cells: []string{format.ShortID(c.ID), c.Name(), c.Image, c.State, c.Status},
			data:  c,
		}
	}
	return items
}

func (a *App) containerKeys(msg tea.KeyMsg) tea.Cmd {
	switch {
	case key.Matches(msg, keys.Start):
		return a.objectAction("started", a.client.StartContainer)
	case key.Matches(msg, keys.Stop):
		return a.objectAction("stopped", a.client.StopContainer)
	case key.Matches(msg, keys.Restart):
		return a.objectAction("restarted", a.client.RestartContainer)
	}
	return nil
}
