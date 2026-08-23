// Copyright (C) 2026 Suleyman Poyraz
// SPDX-License-Identifier: GPL-3.0-or-later

package ui

import (
	"bufio"
	"context"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/zaryob/docker-tui/internal/docker"
)

const (
	logTail     = 1000
	logMaxLines = 10000
	logBatch    = 500
)

var paneTitleStyle = lipgloss.NewStyle().Bold(true).Foreground(colorAccent)

// logGen identifies a log view so that messages from a stream belonging to a
// previously closed view are ignored.
var logGen int

// logView streams the output of a container.
type logView struct {
	gen    int
	name   string
	vp     viewport.Model
	lines  []string
	ch     chan logEvent
	cancel context.CancelFunc
	done   bool
	err    error
}

type logEvent struct {
	line string
	err  error
}

type logChunkMsg struct {
	gen   int
	lines []string
	done  bool
	err   error
}

func newLogView(name string, width, height int) *logView {
	logGen++
	l := &logView{gen: logGen, name: name, vp: viewport.New(0, 0)}
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
	a.logs = newLogView(it.name, a.width, a.bodyHeight())
	return a.logs.start(a.client, it.id)
}

// start begins following the logs of the container in the background.
func (l *logView) start(client *docker.Client, id string) tea.Cmd {
	ctx, cancel := context.WithCancel(context.Background())
	l.cancel = cancel
	ch := make(chan logEvent, logBatch)
	l.ch = ch
	go func() {
		defer close(ch)
		rc, err := client.Logs(ctx, id, docker.LogOptions{Follow: true, Tail: logTail})
		if err != nil {
			ch <- logEvent{err: err}
			return
		}
		defer rc.Close()
		sc := bufio.NewScanner(rc)
		sc.Buffer(make([]byte, 64*1024), 1024*1024)
		for sc.Scan() {
			select {
			case ch <- logEvent{line: cleanLogLine(sc.Text())}:
			case <-ctx.Done():
				return
			}
		}
		if err := sc.Err(); err != nil && ctx.Err() == nil {
			ch <- logEvent{err: err}
		}
	}()
	return l.wait()
}

// wait returns a command that delivers the next batch of log lines.
func (l *logView) wait() tea.Cmd {
	ch, gen := l.ch, l.gen
	return func() tea.Msg {
		msg := logChunkMsg{gen: gen}
		ev, ok := <-ch
		for {
			if !ok {
				msg.done = true
				return msg
			}
			if ev.err != nil {
				msg.err = ev.err
			} else {
				msg.lines = append(msg.lines, ev.line)
			}
			if len(msg.lines) >= logBatch {
				return msg
			}
			select {
			case ev, ok = <-ch:
			default:
				return msg
			}
		}
	}
}

// close stops the log stream.
func (l *logView) close() {
	if l.cancel != nil {
		l.cancel()
	}
}

func cleanLogLine(s string) string {
	s = strings.TrimRight(s, "\r")
	return strings.ReplaceAll(s, "\t", "    ")
}

func (l *logView) update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case logChunkMsg:
		if msg.gen != l.gen {
			return nil
		}
		if msg.err != nil {
			l.err = msg.err
		}
		if len(msg.lines) > 0 {
			l.lines = append(l.lines, msg.lines...)
			if over := len(l.lines) - logMaxLines; over > 0 {
				l.lines = l.lines[over:]
			}
			l.vp.SetContent(strings.Join(l.lines, "\n"))
			l.vp.GotoBottom()
		}
		if msg.done {
			l.done = true
			return nil
		}
		return l.wait()
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
	case l.err != nil:
		info = errorStyle.Render(l.err.Error())
	case l.done:
		info = statusStyle.Render(countNoun(len(l.lines), "lines") + " · stream ended")
	default:
		info = okStyle.Render("● following") + statusStyle.Render(" · "+countNoun(len(l.lines), "lines"))
	}
	header := title + "  " + info
	body := l.vp.View()
	if len(l.lines) == 0 {
		body = statusStyle.Render("no output yet")
	}
	return header + "\n" + body
}
