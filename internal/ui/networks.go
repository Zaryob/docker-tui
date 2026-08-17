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

func newNetworksTab() *tab {
	return newTab(tabNetworks, "Networks", "networks",
		Column{Title: "NETWORK ID", Width: 12},
		Column{Title: "NAME"},
		Column{Title: "DRIVER", Width: 10},
		Column{Title: "SCOPE", Width: 8},
		Column{Title: "SUBNET", Width: 18},
	)
}

func (a *App) loadNetworks() tea.Msg {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	list, err := a.client.ListNetworks(ctx)
	return listMsg{kind: tabNetworks, items: networkItems(list), err: err}
}

func networkItems(list []docker.Network) []item {
	sort.Slice(list, func(i, j int) bool { return list[i].Name < list[j].Name })
	items := make([]item, len(list))
	for i, n := range list {
		items[i] = item{
			id:    n.ID,
			name:  n.Name,
			cells: []string{format.ShortID(n.ID), n.Name, n.Driver, n.Scope, n.Subnet()},
			data:  n,
		}
	}
	return items
}
