// Copyright (C) 2026 Suleyman Poyraz
// SPDX-License-Identifier: GPL-3.0-or-later

package ui

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/zaryob/docker-tui/internal/docker"
)

func TestBuildTimestampTogglePreservesOperation(t *testing.T) {
	for _, done := range []bool{false, true} {
		t.Run(fmt.Sprintf("done=%v", done), func(t *testing.T) {
			l := newLogView("web", 80, 10)
			l.kind, l.done = "Build", done
			l.lines = []string{"build output"}
			l.ensureEntries()
			l.marked[0] = true
			canceled, opened := false, false
			l.cancel = func() { canceled = true }
			l.sources = []logSource{{open: func(context.Context, bool) (io.ReadCloser, error) {
				opened = true
				return io.NopCloser(strings.NewReader("repeated build")), nil
			}}}
			gen := l.gen
			cmd := l.update(keyRunes("t"))
			if cmd != nil || canceled || opened || l.gen != gen || l.done != done || l.timestamps {
				t.Fatalf("timestamp toggle changed build state: cmd %v, canceled %v, opened %v, gen %d, done %v", cmd != nil, canceled, opened, l.gen, l.done)
			}
			if len(l.entries) != 1 || l.entries[0].message != "build output" || !l.marked[0] {
				t.Fatal("timestamp toggle discarded build output or selection")
			}
		})
	}
}

func TestLogRetentionPreservesSelectedMessages(t *testing.T) {
	l := newLogView("web", 80, 20)
	l.follow = false
	l.lines = make([]string, logMaxLines)
	for i := range l.lines {
		l.lines[i] = fmt.Sprintf("line-%d", i)
	}
	l.render()
	l.cursor = 10
	l.marked = map[int]bool{0: true, 10: true, logMaxLines - 1: true}
	l.vp.SetYOffset(8)
	l.update(logChunkMsg{gen: l.gen, lines: []string{"new-1", "new-2"}, done: true})
	if l.cursor != 8 || l.entries[l.cursor].message != "line-10" || l.vp.YOffset != 6 {
		t.Fatalf("selected message moved: cursor %d, message %q, offset %d", l.cursor, l.entries[l.cursor].message, l.vp.YOffset)
	}
	if len(l.marked) != 2 || !l.marked[8] || !l.marked[logMaxLines-3] {
		t.Fatalf("marks changed messages after eviction: %v", l.marked)
	}
	if len(l.lines) != logMaxLines || len(l.entries) != logMaxLines || l.entries[0].message != "line-2" {
		t.Fatalf("retained buffer: lines %d, entries %d, first %q", len(l.lines), len(l.entries), l.entries[0].message)
	}
	// Inspecting a message that has left the buffer must not show another
	// message in its place under the same detail view.
	l.cursor, l.inspecting = 0, true
	l.update(logChunkMsg{gen: l.gen, lines: []string{"new-3"}, done: true})
	if l.inspecting || l.cursor != 0 {
		t.Fatal("evicted message remained open in the detail view")
	}
}

func TestLogMarkToggleRemovesUnselectedRows(t *testing.T) {
	l := newLogView("web", 80, 10)
	l.lines = []string{"one", "two"}
	l.ensureEntries()
	l.update(keyRunes(" "))
	l.update(keyRunes(" "))
	if len(l.marked) != 0 {
		t.Fatalf("unmark left stale selections: %v", l.marked)
	}
	l.update(tea.KeyMsg{Type: tea.KeyCtrlA})
	if len(l.marked) != 2 || !l.marked[0] || !l.marked[1] {
		t.Fatalf("mark all = %v", l.marked)
	}
	l.update(tea.KeyMsg{Type: tea.KeyCtrlA})
	if len(l.marked) != 0 {
		t.Fatalf("unmark all = %v", l.marked)
	}
}

func TestLogDetailCloseKeepsLogPane(t *testing.T) {
	for _, closeKey := range []tea.KeyMsg{{Type: tea.KeyEsc}, keyRunes("q"), {Type: tea.KeyEnter}} {
		t.Run(closeKey.String(), func(t *testing.T) {
			a := newTestApp(t, http.NotFoundHandler())
			l := newLogView("web", 80, 10)
			l.lines = []string{"one", "two"}
			l.render()
			a.pane = l
			a.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
			if !l.inspecting {
				t.Fatal("detail view was not opened")
			}
			a.handleKey(tea.KeyMsg{Type: tea.KeyDown})
			if l.cursor != 0 {
				t.Fatal("navigation changed the message behind its detail view")
			}
			a.handleKey(closeKey)
			if a.pane != l || l.inspecting {
				t.Fatal("closing message details also closed the log pane")
			}
		})
	}
}

func TestIndexFold(t *testing.T) {
	tests := []struct {
		s, sub string
		want   int
	}{
		{"GET /health 200", "health", 5},
		{"GET /Health 200", "HEALTH", 5},
		{"çalışıyor ERROR", "error", len("çalışıyor ")},
		{"no match here", "error", -1},
		{"short", "longer than s", -1},
	}
	for _, tt := range tests {
		if got := indexFold(tt.s, tt.sub); got != tt.want {
			t.Errorf("indexFold(%q, %q) = %d, want %d", tt.s, tt.sub, got, tt.want)
		}
	}
}

func TestHighlightMatches(t *testing.T) {
	if _, ok := highlightMatches("all good", "error"); ok {
		t.Error("unexpected match")
	}
	line := "\x1b[31mError\x1b[0m: disk error"
	got, ok := highlightMatches(line, "error")
	if !ok {
		t.Fatal("expected a match")
	}
	if plain := ansi.Strip(got); plain != "Error: disk error" {
		t.Errorf("text changed: %q", plain)
	}
	if n := strings.Count(got, matchStyle.Render("Error")) + strings.Count(got, matchStyle.Render("error")); n != 2 {
		t.Errorf("got %d highlighted matches in %q, want 2", n, got)
	}
}

func TestLogTimestampsRestartStream(t *testing.T) {
	var gotTimestamps []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/containers/x/json":
			io.WriteString(w, `{"Config":{"Tty":true}}`)
		case "/containers/x/logs":
			gotTimestamps = append(gotTimestamps, r.URL.Query().Get("timestamps"))
			io.WriteString(w, "line\n")
		}
	}))
	defer srv.Close()
	client, err := docker.NewClient("tcp://" + strings.TrimPrefix(srv.URL, "http://"))
	if err != nil {
		t.Fatal(err)
	}

	l := newLogView("web", 80, 10)
	l.client, l.sources = client, []logSource{{id: "x", name: "web"}}
	drain := func(cmd tea.Cmd) {
		for cmd != nil {
			cmd = l.update(cmd())
		}
	}
	drain(l.start())
	oldGen := l.gen
	l.cursor, l.inspecting = 5, true
	l.marked[0] = true
	drain(l.toggleTimestamps())
	if !l.timestamps || l.gen == oldGen {
		t.Errorf("timestamps = %v, gen %d -> %d", l.timestamps, oldGen, l.gen)
	}
	if len(l.lines) != 1 {
		t.Errorf("lines = %q, want only the lines of the new stream", l.lines)
	}
	if len(l.marked) != 0 || l.cursor != 0 || l.inspecting {
		t.Errorf("stream restart kept stale selection: marks %v, cursor %d, inspecting %v", l.marked, l.cursor, l.inspecting)
	}
	if strings.Join(gotTimestamps, ",") != ",1" {
		t.Errorf("timestamps query = %q, want \"\" then \"1\"", gotTimestamps)
	}
}

func TestLogsOfSeveralContainers(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/containers/a1/json", "/containers/b2/json":
			io.WriteString(w, `{"Config":{"Tty":true}}`)
		case "/containers/a1/logs":
			io.WriteString(w, "GET /\n")
		case "/containers/b2/logs":
			io.WriteString(w, "ready\n")
		}
	}))
	defer srv.Close()
	client, err := docker.NewClient("tcp://" + strings.TrimPrefix(srv.URL, "http://"))
	if err != nil {
		t.Fatal(err)
	}
	l := newLogView("2 containers", 80, 10)
	l.client = client
	l.sources = []logSource{{id: "a1", name: "web"}, {id: "b2", name: "db-main"}}
	for cmd := l.start(); cmd != nil; {
		cmd = l.update(cmd())
	}
	got := make([]string, len(l.lines))
	for i, line := range l.lines {
		got[i] = ansi.Strip(line)
	}
	sort.Strings(got)
	if want := "db-main | ready,web     | GET /"; strings.Join(got, ",") != want {
		t.Errorf("lines = %q, want %q", strings.Join(got, ","), want)
	}
	if !l.done {
		t.Error("view not done after all streams ended")
	}
}

func TestLogSearchNavigation(t *testing.T) {
	l := newLogView("web", 80, 10)
	l.lines = []string{"start", "error 1", "ok", "error 2", "ok", "error 3"}
	l.setQuery("ERROR")
	if want := []int{1, 3, 5}; len(l.matches) != 3 || l.matches[0] != want[0] || l.matches[2] != want[2] {
		t.Fatalf("matches = %v, want %v", l.matches, want)
	}
	if l.match != 2 {
		t.Errorf("search starts at match %d, want the last one", l.match)
	}
	l.nextMatch(1)
	if l.match != 0 {
		t.Errorf("next match wraps to %d, want 0", l.match)
	}
	l.nextMatch(-1)
	if l.match != 2 {
		t.Errorf("previous match wraps to %d, want 2", l.match)
	}
	l.setQuery("")
	if len(l.matches) != 0 || l.searchInfo() != "" {
		t.Errorf("clearing the query left matches %v", l.matches)
	}
}
