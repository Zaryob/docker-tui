// Copyright (C) 2026 Suleyman Poyraz
// SPDX-License-Identifier: GPL-3.0-or-later

package ui

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/zaryob/docker-tui/internal/docker"
)

// newTestApp returns an App whose client talks to handler.
func newTestApp(t *testing.T, handler http.Handler) *App {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	client, err := docker.NewClient("tcp://" + strings.TrimPrefix(srv.URL, "http://"))
	if err != nil {
		t.Fatal(err)
	}
	a := New(client, Options{})
	a.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	return a
}

func containersFixture() []docker.Container {
	return []docker.Container{
		{ID: "a1", Names: []string{"/web"}, State: "running", Labels: map[string]string{composeProjectLabel: "shop"}},
		{ID: "b2", Names: []string{"/db"}, State: "running", Labels: map[string]string{composeProjectLabel: "shop"}},
		{ID: "c3", Names: []string{"/cache"}, State: "exited"},
	}
}

func TestBulkActionOnMarkedContainers(t *testing.T) {
	var got []string
	a := newTestApp(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = append(got, r.Method+" "+r.URL.Path)
		if strings.Contains(r.URL.Path, "/b2/") {
			w.WriteHeader(http.StatusConflict)
			w.Write([]byte(`{"message":"cannot stop db"}`))
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	t1 := a.current()
	t1.setItems(containerItems(containersFixture()))
	t1.toggleMark() // web
	t1.table.SetCursor(1)
	t1.toggleMark() // db
	t1.table.SetCursor(2)

	msg := a.handleKey(keyRunes("x"))().(actionMsg)
	if strings.Join(got, ",") != "POST /containers/a1/stop,POST /containers/b2/stop" {
		t.Errorf("requests = %v", got)
	}
	if msg.err == nil || msg.err.Error() != "stopped 1 of 2 containers; stopped db: cannot stop db" {
		t.Errorf("err = %v", msg.err)
	}
	if len(t1.marked) != 0 {
		t.Error("marks not cleared after the action")
	}

	// Without marks the selected container is the target.
	got = nil
	msg = a.handleKey(keyRunes("s"))().(actionMsg)
	if strings.Join(got, ",") != "POST /containers/c3/start" || msg.status != "started cache" || msg.err != nil {
		t.Errorf("requests = %v, msg = %+v", got, msg)
	}
}

func TestBulkRemoveDialog(t *testing.T) {
	a := newTestApp(t, http.NotFoundHandler())
	t1 := a.current()
	t1.setItems(containerItems(containersFixture()))
	t1.markAll()
	d := a.removeDialog()
	if d.title != "Remove 3 containers?" || d.message != "3 containers: web, db, cache" {
		t.Errorf("dialog = %q / %q", d.title, d.message)
	}
	// Only running containers can be killed.
	if d := a.killDialog(); d == nil || !strings.HasPrefix(d.message, "2 containers: web, db") {
		t.Errorf("kill dialog = %+v", d)
	}
}

func TestActionError(t *testing.T) {
	e1, e2 := errors.New("e1"), errors.New("e2")
	if err := actionError("stopped", "containers", 3, nil); err != nil {
		t.Errorf("no errors: %v", err)
	}
	if err := actionError("stopped", "containers", 1, []error{e1}); !errors.Is(err, e1) {
		t.Errorf("single object: %v", err)
	}
	if err := actionError("stopped", "containers", 3, []error{e1, e2}); err.Error() != "stopped 1 of 3 containers; e1 (and 1 more errors)" {
		t.Errorf("several errors: %v", err)
	}
}

func TestDescribeItems(t *testing.T) {
	var items []item
	for _, n := range []string{"a", "b", "c", "d", "e", "f", "g"} {
		items = append(items, item{name: n})
	}
	if got := describeItems(items, "images"); got != "7 images: a, b, c, d, e and 2 more" {
		t.Errorf("describeItems = %q", got)
	}
	if got := describeItems(items[:1], "images"); got != "a" {
		t.Errorf("single item = %q", got)
	}
}

func keyRunes(s string) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)} }
