// Copyright (C) 2026 Suleyman Poyraz
// SPDX-License-Identifier: GPL-3.0-or-later

package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
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
