// Copyright (C) 2026 Suleyman Poyraz
// SPDX-License-Identifier: GPL-3.0-or-later

package ui

import (
	"context"
	"sort"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/zaryob/docker-tui/internal/docker"
	"github.com/zaryob/docker-tui/internal/format"
)

func newVolumesTab() *tab {
	return newTab(tabVolumes, "Volumes", "volumes",
		Column{Title: "VOLUME NAME"},
		Column{Title: "DRIVER", Width: 10},
		Column{Title: "SCOPE", Width: 8},
		Column{Title: "CREATED", Width: 18},
		Column{Title: "MOUNTPOINT"},
	)
}

func (a *App) loadVolumes() tea.Msg {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	list, err := a.client.ListVolumes(ctx)
	return listMsg{kind: tabVolumes, items: volumeItems(list, time.Now()), err: err}
}

func volumeItems(list []docker.Volume, now time.Time) []item {
	sort.Slice(list, func(i, j int) bool { return list[i].Name < list[j].Name })
	items := make([]item, len(list))
	for i, v := range list {
		created := ""
		if t, err := time.Parse(time.RFC3339, v.CreatedAt); err == nil {
			created = format.Ago(t, now)
		}
		items[i] = item{
			id:    v.Name,
			name:  v.Name,
			cells: []string{v.Name, v.Driver, v.Scope, created, v.Mountpoint},
			data:  v,
		}
	}
	return items
}
