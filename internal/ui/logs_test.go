// Copyright (C) 2026 Suleyman Poyraz
// SPDX-License-Identifier: GPL-3.0-or-later

package ui

import (
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
	drain(l.toggleTimestamps())
	if !l.timestamps || l.gen == oldGen {
		t.Errorf("timestamps = %v, gen %d -> %d", l.timestamps, oldGen, l.gen)
	}
	if len(l.lines) != 1 {
		t.Errorf("lines = %q, want only the lines of the new stream", l.lines)
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
