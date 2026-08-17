// Copyright (C) 2026 Suleyman Poyraz
// SPDX-License-Identifier: GPL-3.0-or-later

package ui

import (
	"context"
	"strconv"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/zaryob/docker-tui/internal/docker"
	"github.com/zaryob/docker-tui/internal/format"
)

func newImagesTab() *tab {
	return newTab(tabImages, "Images", "images",
		Column{Title: "IMAGE ID", Width: 12},
		Column{Title: "REPOSITORY:TAG"},
		Column{Title: "CREATED", Width: 18},
		Column{Title: "SIZE", Width: 9},
		Column{Title: "USED BY", Width: 7},
	)
}

func (a *App) loadImages() tea.Msg {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	list, err := a.client.ListImages(ctx)
	return listMsg{kind: tabImages, items: imageItems(list, time.Now()), err: err}
}

func imageItems(list []docker.Image, now time.Time) []item {
	items := make([]item, len(list))
	for i, img := range list {
		used := "-"
		if img.Containers > 0 {
			used = strconv.FormatInt(img.Containers, 10)
		}
		items[i] = item{
			id:   img.ID,
			name: img.Tag(),
			cells: []string{
				format.ShortID(img.ID),
				img.Tag(),
				format.Ago(time.Unix(img.Created, 0), now),
				format.Size(img.Size),
				used,
			},
			data: img,
		}
	}
	return items
}
