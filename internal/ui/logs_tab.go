// Copyright (C) 2026 Suleyman Poyraz
// SPDX-License-Identifier: GPL-3.0-or-later

package ui

import (
	"bufio"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/zaryob/docker-tui/internal/docker"
)

// logTabEntry keeps a row's source separate from its unique message ID.
type logTabEntry struct {
	sourceID      string
	isBuildSource bool
	logEntry
}

// objectID returns the Docker or build object behind a table row.
func (it item) objectID() string {
	if entry, ok := it.data.(logTabEntry); ok {
		return entry.sourceID
	}
	return it.id
}

func logTabItems(rc io.Reader, sourceID, name string, isBuild bool, fallback time.Time, colorIndex, limit int) ([]item, error) {
	sc := bufio.NewScanner(rc)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	items := make([]item, 0, limit)
	occurrences := make(map[[32]byte]int)
	kind, icon := "container", "📦 "
	if isBuild {
		kind, icon = "build", "🔧 "
	}
	for len(items) < limit && sc.Scan() {
		line := sc.Text()
		entry := parseLogLine(line, name, isBuild, fallback, colorIndex)
		digest := sha256.Sum256([]byte(line))
		occurrences[digest]++
		items = append(items, item{
			id:       fmt.Sprintf("%s:%s:%x:%d", kind, sourceID, digest, occurrences[digest]),
			name:     name,
			cells:    []string{"[ ]", entry.time, icon + name, entry.message},
			sortKeys: []string{1: entry.time},
			data:     logTabEntry{sourceID: sourceID, isBuildSource: isBuild, logEntry: entry},
		})
	}
	return items, sc.Err()
}

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
	var errs []string
	report := func(source string, err error) {
		if err != nil {
			errs = append(errs, source+": "+err.Error())
		}
	}

	// 1. Fetch running containers
	containers, err := a.client.ListContainers(ctx, false)
	report("containers", err)
	for i, c := range containers {
		if i >= 10 {
			break
		}
		name := c.Name()
		if name == "" {
			name = c.ID[:min(12, len(c.ID))]
		}
		rc, err := a.client.Logs(ctx, c.ID, docker.LogOptions{Follow: false, Tail: 15, Timestamps: true})
		if err != nil {
			report("container "+name, err)
			continue
		}
		rows, err := logTabItems(rc, c.ID, name, false, time.Now(), i, 15)
		rc.Close()
		items = append(items, rows...)
		report("container "+name, err)
	}

	// 2. Fetch build history
	records, err := a.buildx.History(ctx)
	report("build history", err)
	for i, r := range records {
		if i >= 5 {
			break
		}
		name := r.Name
		if name == "" {
			name = r.ID()
		}
		rc, err := a.buildx.Logs(ctx, r.ID())
		if err != nil {
			report("build "+name, err)
			continue
		}
		rows, err := logTabItems(rc, r.ID(), name, true, r.CreatedAt, i, 15)
		rc.Close()
		items = append(items, rows...)
		report("build "+name, err)
	}

	// Sort logs descending by timestamp
	sort.SliceStable(items, func(i, j int) bool {
		return items[i].cells[1] > items[j].cells[1]
	})

	var loadErr error
	if len(errs) > 0 {
		loadErr = fmt.Errorf("logs: %s", strings.Join(errs, "; "))
	}
	return listMsg{kind: tabLogs, items: items, err: loadErr, partial: loadErr != nil && len(items) > 0}
}

func (a *App) openSelectedLog() tea.Cmd {
	it, ok := a.selectedObject()
	if !ok {
		return nil
	}
	entry, ok := it.data.(logTabEntry)
	if !ok {
		return nil
	}
	if entry.isBuildSource {
		cli, id := a.buildx, entry.sourceID
		l := newLogView(it.name, a.width, a.bodyHeight())
		l.kind, l.ended = "Build logs", "complete"
		l.running = "● loading"
		l.client = a.client
		l.sources = []logSource{{id: id, name: it.name, open: func(ctx context.Context, _ bool) (io.ReadCloser, error) {
			return cli.Logs(ctx, id)
		}}}
		a.pane = l
		return l.start()
	}

	sources := []logSource{{id: entry.sourceID, name: it.name}}
	l := newLogView(it.name, a.width, a.bodyHeight())
	l.timestamps = true
	l.client, l.sources = a.client, sources
	a.pane = l
	return l.start()
}
