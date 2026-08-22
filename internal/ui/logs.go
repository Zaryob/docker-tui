// Copyright (C) 2026 Suleyman Poyraz
// SPDX-License-Identifier: GPL-3.0-or-later

package ui

import (
	"bufio"
	"context"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/zaryob/docker-tui/internal/docker"
)

const logTail = 1000

var paneTitleStyle = lipgloss.NewStyle().Bold(true).Foreground(colorAccent)

// logView shows the output of a container.
type logView struct {
	id      string
	name    string
	vp      viewport.Model
	lines   []string
	loading bool
	err     error
}

type logsMsg struct {
	id    string
	lines []string
	err   error
}

func newLogView(id, name string, width, height int) *logView {
	l := &logView{id: id, name: name, loading: true, vp: viewport.New(0, 0)}
	l.setSize(width, height)
	return l
}

func (l *logView) setSize(width, height int) {
	l.vp.Width = width
	l.vp.Height = max(height-1, 1)
}

// openLogs opens the log view for the selected container.
func (a *App) openLogs() tea.Cmd {
	it, ok := a.current().selected()
	if !ok {
		return nil
	}
	a.logs = newLogView(it.id, it.name, a.width, a.bodyHeight())
	client, id := a.client, it.id
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		lines, err := readLogs(ctx, client, id)
		return logsMsg{id: id, lines: lines, err: err}
	}
}

func readLogs(ctx context.Context, client *docker.Client, id string) ([]string, error) {
	rc, err := client.Logs(ctx, id, docker.LogOptions{Tail: logTail})
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	var lines []string
	sc := bufio.NewScanner(rc)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() {
		lines = append(lines, cleanLogLine(sc.Text()))
	}
	return lines, sc.Err()
}

func cleanLogLine(s string) string {
	s = strings.TrimRight(s, "\r")
	return strings.ReplaceAll(s, "\t", "    ")
}

func (l *logView) update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case logsMsg:
		if msg.id != l.id {
			return nil
		}
		l.loading = false
		l.err = msg.err
		l.lines = msg.lines
		l.vp.SetContent(strings.Join(l.lines, "\n"))
		l.vp.GotoBottom()
		return nil
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, keys.Top):
			l.vp.GotoTop()
			return nil
		case key.Matches(msg, keys.Bottom):
			l.vp.GotoBottom()
			return nil
		}
	}
	var cmd tea.Cmd
	l.vp, cmd = l.vp.Update(msg)
	return cmd
}

func (l *logView) view() string {
	title := paneTitleStyle.Render("Logs: " + l.name)
	var info string
	switch {
	case l.loading:
		info = "loading…"
	case l.err != nil:
		info = errorStyle.Render(l.err.Error())
	default:
		info = statusStyle.Render(countNoun(len(l.lines), "lines"))
	}
	header := title + "  " + info
	body := l.vp.View()
	if !l.loading && l.err == nil && len(l.lines) == 0 {
		body = statusStyle.Render("no output")
	}
	return header + "\n" + body
}
