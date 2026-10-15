// Copyright (C) 2026 Suleyman Poyraz
// SPDX-License-Identifier: GPL-3.0-or-later

package ui

import (
	"bufio"
	"context"
	"io"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/zaryob/docker-tui/internal/docker"
)

func newLogsTab() *tab {
	t := newTab(tabLogs, "Logs", "log entries",
		Column{Title: "[ ]", Width: 3},
		Column{Title: "TIMESTAMP", Width: 19},
		Column{Title: "OBJECT", Width: 16},
		Column{Title: "MESSAGE", Grow: 2},
	)
	t.table.SetCellStyle(func(col int, value string) (lipgloss.Style, bool) {
		switch col {
		case 0:
			if value == "[✓]" {
				return markStyle, true
			}
			return statusStyle, true
		case 1:
			return timeStyle, true
		case 2:
			if strings.Contains(value, "🔧") {
				return lipgloss.NewStyle().Foreground(lipgloss.Color("#5FD7D7")), true
			}
			return lipgloss.NewStyle().Foreground(colorAccent), true
		}
		return lipgloss.Style{}, false
	})
	return t
}

func (a *App) loadLogs() tea.Msg {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var items []item

	// 1. Fetch running containers
	containers, _ := a.client.ListContainers(ctx, false)
	for i, c := range containers {
		if i >= 10 {
			break
		}
		name := c.Name()
		if name == "" {
			name = c.ID[:min(12, len(c.ID))]
		}
		rc, err := a.client.Logs(ctx, c.ID, docker.LogOptions{Follow: false, Tail: 15, Timestamps: true})
		if err == nil {
			sc := bufio.NewScanner(rc)
			for sc.Scan() {
				line := sc.Text()
				entry := parseLogLine(line, name, false, time.Now(), i)
				items = append(items, item{
					id:   c.ID,
					name: name,
					cells: []string{
						"[ ]",
						entry.time,
						"📦 " + name,
						entry.message,
					},
					sortKeys: []string{
						1: entry.time,
					},
					data: entry,
				})
			}
			rc.Close()
		}
	}

	// 2. Fetch build history
	records, _ := a.buildx.History(ctx)
	for i, r := range records {
		if i >= 5 {
			break
		}
		name := r.Name
		if name == "" {
			name = r.ID()
		}
		rc, err := a.buildx.Logs(ctx, r.ID())
		if err == nil {
			sc := bufio.NewScanner(rc)
			linesRead := 0
			for sc.Scan() && linesRead < 15 {
				linesRead++
				line := sc.Text()
				entry := parseLogLine(line, name, true, r.CreatedAt, i)
				items = append(items, item{
					id:   r.ID(),
					name: name,
					cells: []string{
						"[ ]",
						entry.time,
						"🔧 " + name,
						entry.message,
					},
					sortKeys: []string{
						1: entry.time,
					},
					data: entry,
				})
			}
			rc.Close()
		}
	}

	// Sort logs descending by timestamp
	sort.SliceStable(items, func(i, j int) bool {
		return items[i].cells[1] > items[j].cells[1]
	})

	return listMsg{kind: tabLogs, items: items, err: nil}
}

func (a *App) openSelectedLog() tea.Cmd {
	it, ok := a.selectedObject()
	if !ok {
		return nil
	}
	// Check if build or container
	isBuild := strings.Contains(it.cells[2], "🔧")
	if isBuild {
		cli, id := a.buildx, it.id
		l := newLogView(it.name, a.width, a.bodyHeight())
		l.kind, l.ended = "Logs", "complete"
		l.running = "● loading"
		l.client = a.client
		l.sources = []logSource{{id: id, name: it.name, open: func(ctx context.Context, _ bool) (io.ReadCloser, error) {
			return cli.Logs(ctx, id)
		}}}
		a.pane = l
		return l.start()
	}

	sources := []logSource{{id: it.id, name: it.name}}
	l := newLogView(it.name, a.width, a.bodyHeight())
	l.timestamps = true
	l.client, l.sources = a.client, sources
	a.pane = l
	return l.start()
}
