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

	"github.com/zaryob/docker-tui/internal/docker"
)

const topInterval = 2 * time.Second

var topGen int

// topView lists the processes running in a container and refreshes the list
// periodically.
type topView struct {
	gen     int
	id      string
	name    string
	client  *docker.Client
	table   *Table
	titles  []string
	count   int
	loading bool
	err     error
	width   int
	height  int
	ctx     context.Context
	cancel  context.CancelFunc
}

type topMsg struct {
	gen   int
	procs docker.Processes
	err   error
}

type topTickMsg struct{ gen int }

// openTop opens the process list of the selected container.
func (a *App) openTop() tea.Cmd {
	it, ok := a.selectedObject()
	if !ok {
		return nil
	}
	if c := it.data.(docker.Container); c.State != "running" && c.State != "paused" {
		return a.setStatus(fmt.Sprintf("%s is not running", it.name), true)
	}
	topGen++
	ctx, cancel := context.WithCancel(context.Background())
	v := &topView{
		gen: topGen, id: it.id, name: it.name, client: a.client,
		table: NewTable(), loading: true, ctx: ctx, cancel: cancel,
	}
	v.setSize(a.width, a.bodyHeight())
	a.pane = v
	return v.fetch()
}

func (v *topView) fetch() tea.Cmd {
	ctx, gen, id, client := v.ctx, v.gen, v.id, v.client
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		p, err := client.Top(ctx, id)
		return topMsg{gen: gen, procs: p, err: err}
	}
}

func (v *topView) setSize(width, height int) {
	v.width, v.height = width, height
	v.table.SetSize(width, max(height-1, 1))
}

func (v *topView) close() { v.cancel() }

// topColumns builds table columns for the titles reported by ps. The command
// column takes the remaining width; the others are sized to their content.
func topColumns(titles []string, rows [][]string) []Column {
	cols := make([]Column, len(titles))
	for i, title := range titles {
		if isCommandTitle(title) {
			cols[i] = Column{Title: title}
			continue
		}
		w := len(title)
		for _, r := range rows {
			if i < len(r) {
				w = max(w, len(r[i]))
			}
		}
		cols[i] = Column{Title: title, Width: min(w, 24)}
	}
	return cols
}

func isCommandTitle(title string) bool {
	switch strings.ToUpper(title) {
	case "CMD", "COMMAND", "ARGS":
		return true
	}
	return false
}

func (v *topView) update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case topMsg:
		if msg.gen != v.gen {
			return nil
		}
		v.loading = false
		v.err = msg.err
		if msg.err == nil {
			if !slices.Equal(v.titles, msg.procs.Titles) {
				v.titles = msg.procs.Titles
				cursor := v.table.Cursor()
				v.table = NewTable(topColumns(msg.procs.Titles, msg.procs.Processes)...)
				v.table.SetSize(v.width, max(v.height-1, 1))
				v.table.SetCursor(cursor)
			}
			v.count = len(msg.procs.Processes)
			v.table.SetRows(msg.procs.Processes)
		}
		gen := v.gen
		return tea.Tick(topInterval, func(time.Time) tea.Msg { return topTickMsg{gen: gen} })
	case topTickMsg:
		if msg.gen != v.gen {
			return nil
		}
		return v.fetch()
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, keys.Up):
			v.table.Move(-1)
		case key.Matches(msg, keys.Down):
			v.table.Move(1)
		case key.Matches(msg, keys.PageUp):
			v.table.Move(-v.table.PageSize())
		case key.Matches(msg, keys.PageDown):
			v.table.Move(v.table.PageSize())
		case key.Matches(msg, keys.Top):
			v.table.Top()
		case key.Matches(msg, keys.Bottom):
			v.table.Bottom()
		}
	}
	return nil
}

func (v *topView) view() string {
	header := paneTitleStyle.Render("Processes: " + v.name)
	switch {
	case v.loading:
		return header + "  " + statusStyle.Render("loading…")
	case v.err != nil:
		return header + "  " + errorStyle.Render(v.err.Error())
	}
	return header + "  " + statusStyle.Render(processCount(v.count)) + "\n" + v.table.View()
}

func processCount(n int) string {
	if n == 1 {
		return "1 process"
	}
	return fmt.Sprintf("%d processes", n)
}
