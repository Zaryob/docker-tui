// Copyright (C) 2026 Suleyman Poyraz
// SPDX-License-Identifier: GPL-3.0-or-later

package ui

import (
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

// runCmd executes cmd, batches included, and feeds the resulting messages
// to the app once; the commands those produce are not run.
func runCmd(a *App, cmd tea.Cmd) {
	if cmd == nil {
		return
	}
	switch msg := cmd().(type) {
	case tea.BatchMsg:
		for _, c := range msg {
			runCmd(a, c)
		}
	case nil:
	default:
		a.Update(msg)
	}
}

func TestStaleListDoesNotReplaceNewer(t *testing.T) {
	a := newTestApp(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "[]")
	}))
	tb := a.tabs[tabContainers]
	first, second := a.load(tabContainers), a.load(tabContainers)
	if tb.loadSeq != 2 {
		t.Fatalf("loadSeq = %d, want 2", tb.loadSeq)
	}
	newer := containerItems(containersFixture())
	a.Update(listMsg{kind: tabContainers, seq: 2, items: newer})
	a.Update(listMsg{kind: tabContainers, seq: 1, items: nil})
	if len(tb.all) != len(newer) {
		t.Fatalf("stale list replaced the newer one: %d items", len(tb.all))
	}
	// Loads stamp their results with the sequence they were issued with.
	if msg, ok := first().(listMsg); !ok || msg.seq != 1 {
		t.Errorf("first load message = %+v", msg)
	}
	if msg, ok := second().(listMsg); !ok || msg.seq != 2 {
		t.Errorf("second load message = %+v", msg)
	}
	// Unordered messages, as sent by tests and older code, still apply.
	a.Update(listMsg{kind: tabContainers, items: nil})
	if len(tb.all) != 0 {
		t.Errorf("unordered list ignored")
	}
}

func TestToggleStoppedCapturesVisibilityWhenIssued(t *testing.T) {
	var (
		mu   sync.Mutex
		alls []string
	)
	a := newTestApp(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		alls = append(alls, r.URL.Query().Get("all"))
		mu.Unlock()
		io.WriteString(w, "[]")
	}))
	cmd := a.handleKey(keyRunes("a"))
	if !a.onlyRunning {
		t.Fatal("a did not hide stopped containers")
	}
	// Flipping the flag after the command was built must not affect it.
	a.onlyRunning = false
	runCmd(a, cmd)
	mu.Lock()
	defer mu.Unlock()
	if len(alls) != 1 || alls[0] != "" {
		t.Errorf("all query = %q, want one request without all=1", alls)
	}
}

func TestLogPrefixParsedWhenColored(t *testing.T) {
	for _, p := range logPrefixes([]logSource{{name: "web"}, {name: "db-main"}}) {
		if !strings.HasSuffix(p, " | ") {
			t.Errorf("prefix %q does not end with the plain separator", p)
		}
	}
	raw := "\x1b[38;2;36;150;237mweb    \x1b[0m | 2024-05-01T10:00:00.000000000Z GET /"
	e := parseLogLine(raw, "2 containers", false, time.Now(), 0)
	if e.object != "web" {
		t.Errorf("object = %q, want web", e.object)
	}
	if e.message != "GET /" {
		t.Errorf("message = %q, want GET /", e.message)
	}
	if !strings.HasPrefix(e.time, "2024-05-01") {
		t.Errorf("time = %q, want the line's timestamp", e.time)
	}
}

func TestFitWidthKeepsEscapes(t *testing.T) {
	s := fitWidth("\x1b[31mhello\x1b[0m world and more", 9)
	if w := ansi.StringWidth(s); w != 9 {
		t.Errorf("width = %d, want 9: %q", w, s)
	}
	if !strings.Contains(s, "\x1b[31m") || !strings.HasSuffix(ansi.Strip(s), "…") {
		t.Errorf("fitWidth = %q", s)
	}
	if got := fitWidth("ab", 4); got != "ab  " {
		t.Errorf("padded = %q", got)
	}
}

func TestLogViewResizeRebuildsRows(t *testing.T) {
	l := newLogView("web", 120, 20)
	l.lines = []string{strings.Repeat("m", 200)}
	l.render()
	l.setSize(60, 20)
	for _, row := range strings.Split(l.vp.View(), "\n") {
		if w := ansi.StringWidth(row); w > 60 {
			t.Fatalf("row is %d cells wide after resizing to 60: %q", w, ansi.Strip(row))
		}
	}
	if !strings.Contains(ansi.Strip(l.vp.View()), "…") {
		t.Error("row was not cut to the new width")
	}
}

func TestPaneSizedWithOwnHelp(t *testing.T) {
	a := newTestApp(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "{}")
	}))
	a.help.ShowAll = true
	a.current().setItems(containerItems(containersFixture()))
	selectID(t, a.current(), "a1")
	a.handleKey(keyRunes("i"))
	v, ok := a.pane.(*inspectView)
	if !ok {
		t.Fatalf("pane = %T", a.pane)
	}
	if want := a.bodyHeight(); v.vp.Height != want-1 && v.vp.Height != want {
		t.Errorf("inspect height = %d, body height with the pane's help = %d", v.vp.Height, want)
	}
}

func TestTerminalSessionClosedWhenPaneReplaced(t *testing.T) {
	a := newTestApp(t, http.NotFoundHandler())
	a.pane = newLogView("web", 80, 20)
	ours, theirs := net.Pipe()
	a.Update(termStartedMsg{gen: 42, sess: &termSession{conn: theirs}})
	ours.SetReadDeadline(time.Now().Add(time.Second))
	if _, err := ours.Read(make([]byte, 1)); !errors.Is(err, io.EOF) {
		t.Errorf("read after the session was dropped = %v, want EOF", err)
	}
}

func TestFooterShowsActionsOfTheTab(t *testing.T) {
	a := newTestApp(t, http.NotFoundHandler())
	helpText := func(kind tabKind) string {
		a.active = int(kind)
		return a.help.View(a.helpKeys())
	}
	if h := helpText(tabLogs); strings.Contains(h, "remove") || strings.Contains(h, "start") || !strings.Contains(h, "open log") {
		t.Errorf("logs footer = %q", h)
	}
	if h := helpText(tabContainers); !strings.Contains(h, "remove") || !strings.Contains(h, "stop") {
		t.Errorf("containers footer = %q", h)
	}
	if h := helpText(tabVolumes); strings.Contains(h, "logs") || strings.Contains(h, "stop") || !strings.Contains(h, "prune") {
		t.Errorf("volumes footer = %q", h)
	}
	if h := helpText(tabBuilds); !strings.Contains(h, "build logs") || strings.Contains(h, "stop") {
		t.Errorf("builds footer = %q", h)
	}
	a.help.ShowAll = true
	if h := helpText(tabImages); !strings.Contains(h, "registry login") || strings.Contains(h, "stats") || strings.Contains(h, "fold project") {
		t.Errorf("images full help = %q", h)
	}
	if h := helpText(tabLogs); strings.Contains(h, "copy ID") || !strings.Contains(h, "mark") {
		t.Errorf("logs full help = %q", h)
	}
}

func TestLogObjectColorsAreStable(t *testing.T) {
	l := newLogView("2 containers", 80, 20)
	l.lines = []string{"web | a", "db | b", "web | c", "db | d", "web | e"}
	l.ensureEntries()
	for i, e := range l.entries {
		want := l.entries[i%2].color
		if e.color != want {
			t.Errorf("line %d (%s) color = %v, want the color of its container %v", i, e.object, e.color, want)
		}
	}
	if l.entries[0].color == l.entries[1].color {
		t.Error("different containers share a color")
	}
}
