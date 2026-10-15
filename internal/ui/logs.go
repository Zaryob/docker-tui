// Copyright (C) 2026 Suleyman Poyraz
// SPDX-License-Identifier: GPL-3.0-or-later

package ui

import (
	"bufio"
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/zaryob/docker-tui/internal/docker"
)

const (
	logTail     = 1000
	logMaxLines = 10000
	logBatch    = 500
)

var (
	paneTitleStyle = lipgloss.NewStyle().Bold(true).Foreground(colorAccent)
	matchStyle     = lipgloss.NewStyle().Background(colorWarn).Foreground(lipgloss.Color("#000000"))
)

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

	// searching is true while the search input has focus.
	searching bool
	search    textinput.Model
	// query is the active search; matches holds the indices of the lines
	// containing it and match the index of the current one in matches.
	query   string
	matches []int
	match   int
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
	si := textinput.New()
	si.Prompt = "search: "
	si.PromptStyle = lipgloss.NewStyle().Foreground(colorAccent)
	l := &logView{gen: logGen, name: name, vp: viewport.New(0, 0), search: si}
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
	l := newLogView(it.name, a.width, a.bodyHeight())
	a.pane = l
	return l.start(a.client, it.id)
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

// capturesInput reports whether the view wants every key, including the
// ones that would otherwise close it.
func (l *logView) capturesInput() bool { return l.searching }

// render rebuilds the viewport content, highlighting search matches.
func (l *logView) render() {
	l.matches = l.matches[:0]
	if l.query == "" {
		l.vp.SetContent(strings.Join(l.lines, "\n"))
		return
	}
	var b strings.Builder
	for i, line := range l.lines {
		if i > 0 {
			b.WriteByte('\n')
		}
		hl, ok := highlightMatches(line, l.query)
		if ok {
			l.matches = append(l.matches, i)
		}
		b.WriteString(hl)
	}
	l.vp.SetContent(b.String())
}

// highlightMatches returns line with every case-insensitive occurrence of
// query highlighted, and whether there was any. Escape sequences are
// removed from matching lines so that the highlight is not disturbed.
func highlightMatches(line, query string) (string, bool) {
	if query == "" {
		return line, false
	}
	plain := ansi.Strip(line)
	i := indexFold(plain, query)
	if i < 0 {
		return line, false
	}
	var b strings.Builder
	for i >= 0 {
		b.WriteString(plain[:i])
		b.WriteString(matchStyle.Render(plain[i : i+len(query)]))
		plain = plain[i+len(query):]
		i = indexFold(plain, query)
	}
	b.WriteString(plain)
	return b.String(), true
}

// indexFold returns the byte index of the first occurrence of substr in s
// under Unicode case folding, or -1. Matches always have the byte length of
// substr, so the result can be used to slice s.
func indexFold(s, substr string) int {
	n := len(substr)
	for i := range s {
		if len(s)-i < n {
			break
		}
		if strings.EqualFold(s[i:i+n], substr) {
			return i
		}
	}
	return -1
}

// setQuery starts a new search and jumps to the last match, which is the
// most recent output.
func (l *logView) setQuery(q string) {
	l.query = q
	l.render()
	l.match = len(l.matches) - 1
	l.showMatch()
}

// nextMatch moves to the next (delta 1) or previous (delta -1) match,
// wrapping around.
func (l *logView) nextMatch(delta int) {
	if len(l.matches) == 0 {
		return
	}
	l.match = (l.match + delta + len(l.matches)) % len(l.matches)
	l.showMatch()
}

// showMatch scrolls the current match to the middle of the viewport.
func (l *logView) showMatch() {
	if l.match < 0 || l.match >= len(l.matches) {
		return
	}
	l.vp.SetYOffset(l.matches[l.match] - l.vp.Height/2)
}

func (l *logView) handleSearchKey(msg tea.KeyMsg) tea.Cmd {
	switch msg.String() {
	case "esc":
		l.searching = false
		l.search.Blur()
		l.setQuery("")
		return nil
	case "enter":
		l.searching = false
		l.search.Blur()
		return nil
	}
	var cmd tea.Cmd
	l.search, cmd = l.search.Update(msg)
	l.setQuery(l.search.Value())
	return cmd
}

func (l *logView) update(msg tea.Msg) tea.Cmd {
	if km, ok := msg.(tea.KeyMsg); ok && l.searching {
		return l.handleSearchKey(km)
	}
	switch msg := msg.(type) {
	case logChunkMsg:
		if msg.gen != l.gen {
			return nil
		}
		if msg.err != nil {
			l.err = msg.err
		}
		if len(msg.lines) > 0 {
			// Only stick to the bottom if the user has not scrolled up.
			atBottom := l.vp.AtBottom() || len(l.lines) == 0
			l.lines = append(l.lines, msg.lines...)
			dropped := 0
			if over := len(l.lines) - logMaxLines; over > 0 {
				l.lines = l.lines[over:]
				dropped = over
			}
			// Keep the current match selected while lines are added or
			// dropped from the top.
			var cur int
			hasCur := l.match >= 0 && l.match < len(l.matches)
			if hasCur {
				cur = l.matches[l.match] - dropped
			}
			l.render()
			if hasCur {
				l.match = min(sort.SearchInts(l.matches, cur), max(len(l.matches)-1, 0))
			} else {
				l.match = len(l.matches) - 1
			}
			if atBottom {
				l.vp.GotoBottom()
			}
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
		case key.Matches(msg, logKeys.Search):
			l.searching = true
			l.search.SetValue(l.query)
			l.search.CursorEnd()
			return l.search.Focus()
		case key.Matches(msg, logKeys.NextMatch):
			l.nextMatch(1)
			return nil
		case key.Matches(msg, logKeys.PrevMatch):
			l.nextMatch(-1)
			return nil
		}
	}
	var cmd tea.Cmd
	l.vp, cmd = l.vp.Update(msg)
	return cmd
}

// logKeyMap holds the keys specific to the log view.
type logKeyMap struct {
	Search    key.Binding
	NextMatch key.Binding
	PrevMatch key.Binding
}

var logKeys = logKeyMap{
	Search:    key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "search")),
	NextMatch: key.NewBinding(key.WithKeys("n"), key.WithHelp("n/N", "next/prev match")),
	PrevMatch: key.NewBinding(key.WithKeys("N")),
}

func (k logKeyMap) ShortHelp() []key.Binding {
	return []key.Binding{keys.Up, keys.Down, keys.PageUp, keys.PageDown, keys.Top, keys.Bottom, k.Search, k.NextMatch, keys.Back}
}

func (k logKeyMap) FullHelp() [][]key.Binding { return [][]key.Binding{k.ShortHelp()} }

// keyMap implements keyMapper.
func (l *logView) keyMap() help.KeyMap { return logKeys }

// searchInfo describes the state of the search for the header.
func (l *logView) searchInfo() string {
	switch {
	case l.query == "":
		return ""
	case len(l.matches) == 0:
		return errorStyle.Render(fmt.Sprintf("no match for %q", l.query))
	default:
		return statusStyle.Render(fmt.Sprintf("match %d/%d for %q", l.match+1, len(l.matches), l.query))
	}
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
	if si := l.searchInfo(); si != "" {
		header += statusStyle.Render(" · ") + si
	}
	if l.searching {
		header = l.search.View()
	}
	body := l.vp.View()
	if len(l.lines) == 0 {
		body = statusStyle.Render("no output yet")
	}
	return header + "\n" + body
}
