// Copyright (C) 2026 Suleyman Poyraz
// SPDX-License-Identifier: GPL-3.0-or-later

package ui

import (
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/zaryob/docker-tui/internal/buildx"
)

func localProject(t *testing.T) buildx.Project {
	t.Helper()
	file := filepath.Join(t.TempDir(), "compose.yaml")
	if err := os.WriteFile(file, []byte("services: {}"), 0600); err != nil {
		t.Fatal(err)
	}
	return buildx.Project{Name: "shop", ConfigFiles: file, Status: "running(1)"}
}

func TestComposeRetainsProjectAfterDown(t *testing.T) {
	a := newTestApp(t, http.NotFoundHandler())
	p := localProject(t)
	a.mergeProjects([]buildx.Project{p}, nil)
	a.mergeProjects(nil, nil)
	if a.projects[p.Name].ConfigFiles != p.ConfigFiles || a.projects[p.Name].Status != "not running" {
		t.Fatal(a.projects)
	}
	a.active = int(tabProjects)
	// Down always asks for confirmation; cancel does not create a stream.
	a.handleKey(keyRunes("x"))
	if a.confirm == nil || a.compose != nil {
		t.Fatal("down did not wait for confirmation")
	}
	a.handleKey(keyRunes("n"))
	if a.confirm != nil || a.compose != nil {
		t.Fatal("canceled down started an operation")
	}
	a.handleKey(keyRunes("u"))
	if a.compose == nil || a.pane != asPane(a.compose) {
		t.Fatal("retained project cannot be started")
	}
	a.compose.close()
}

func TestComposeBackgroundAndCompletion(t *testing.T) {
	a := newTestApp(t, http.NotFoundHandler())
	p := localProject(t)
	a.startCompose(p, "up")
	l := a.compose
	defer l.close()
	a.handleKey(tea.KeyMsg{Type: tea.KeyEsc})
	if a.pane != nil || a.compose != l || a.composeIndicator() == "" {
		t.Fatal("escape lost the running operation")
	}
	a.active = int(tabProjects)
	a.handleKey(keyRunes("u"))
	if a.pane != asPane(l) {
		t.Fatal("cannot reopen progress")
	}
	a.Update(logChunkMsg{gen: l.gen, done: true, err: errors.New("compose failed")})
	if a.compose != nil || a.pane != asPane(l) || l.err == nil {
		t.Fatal("completion lost output or error")
	}
}

func TestComposeMissingFilesAndStaleForms(t *testing.T) {
	a := newTestApp(t, http.NotFoundHandler())
	p := buildx.Project{Name: "remote", ConfigFiles: "/missing/compose.yaml"}
	a.startCompose(p, "up")
	if a.compose != nil || !strings.Contains(a.status, "unavailable locally") {
		t.Fatal("missing remote files accepted")
	}
	old := &composeForm{}
	current := &composeForm{}
	a.pane = current
	a.Update(openComposeMsg{form: old, project: localProject(t)})
	if a.pane != current || len(a.projects) != 0 {
		t.Fatal("stale form changed current screen")
	}
}

func TestOpenedComposeSourceSurvivesDaemonRefresh(t *testing.T) {
	a := newTestApp(t, http.NotFoundHandler())
	p := localProject(t)
	p.LocalSource = true
	a.projects[p.Name] = p
	a.mergeProjects([]buildx.Project{{Name: p.Name, ConfigFiles: "/remote/compose.yaml", WorkingDir: "/remote", Status: "running(1)"}}, nil)
	got := a.projects[p.Name]
	if got.ConfigFiles != p.ConfigFiles || !got.LocalSource || got.Status != "running(1)" {
		t.Fatal("daemon labels replaced the explicitly opened local source")
	}
}
