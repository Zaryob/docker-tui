// Copyright (C) 2026 Suleyman Poyraz
// SPDX-License-Identifier: GPL-3.0-or-later

package ui

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync"
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
	var (
		mu  sync.Mutex
		got []string
	)
	a := newTestApp(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		got = append(got, r.Method+" "+r.URL.Path)
		mu.Unlock()
		if strings.Contains(r.URL.Path, "/b2/") {
			w.WriteHeader(http.StatusConflict)
			w.Write([]byte(`{"message":"cannot stop db"}`))
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	t1 := a.current()
	t1.setItems(containerItems(containersFixture()))
	selectID(t, t1, "a1")
	t1.toggleMark() // web
	selectID(t, t1, "b2")
	t1.toggleMark() // db
	selectID(t, t1, "c3")

	msg := a.handleKey(keyRunes("x"))().(actionMsg)
	sort.Strings(got)
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

func TestMarkProject(t *testing.T) {
	a := newTestApp(t, http.NotFoundHandler())
	t1 := a.current()
	t1.setItems(containerItems(containersFixture()))
	selectID(t, t1, "b2") // db, project shop
	a.handleKey(keyRunes("C"))
	if got := ids(t1.markedItems()); got != "a1,b2" {
		t.Errorf("marked = %s, want a1,b2", got)
	}
	if a.status != "marked 2 containers of project shop" {
		t.Errorf("status = %q", a.status)
	}

	t1.clearMarks()
	selectID(t, t1, "c3") // cache, no project
	a.handleKey(keyRunes("C"))
	if len(t1.marked) != 0 || !a.statusErr {
		t.Errorf("container without project: marked %v, status %q", t1.marked, a.status)
	}
}

func TestOpenLogsOfMarkedContainers(t *testing.T) {
	a := newTestApp(t, http.NotFoundHandler())
	t1 := a.current()
	t1.setItems(containerItems(containersFixture()))
	selectID(t, t1, "b2")
	a.handleKey(keyRunes("C"))
	a.openLogs()
	l, ok := a.pane.(*logView)
	if !ok {
		t.Fatal("log view not opened")
	}
	l.close()
	if len(l.sources) != 2 || l.sources[0].name != "web" || l.sources[1].name != "db" {
		t.Errorf("sources = %+v", l.sources)
	}
	if l.name != "2 containers: web, db" {
		t.Errorf("title = %q", l.name)
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

// selectID moves the cursor of tb to the row of the object with id.
func selectID(t *testing.T, tb *tab, id string) {
	t.Helper()
	for i, it := range tb.items {
		if it.id == id {
			tb.table.SetCursor(i)
			return
		}
	}
	t.Fatalf("no row with id %s", id)
}

func TestComposeProjectRows(t *testing.T) {
	a := newTestApp(t, http.NotFoundHandler())
	t1 := a.current()
	t1.setItems(containerItems(containersFixture()))
	names := func() string {
		var out []string
		for _, it := range t1.items {
			out = append(out, it.cells[containerNameCol])
		}
		return strings.Join(out, "|")
	}
	if got := names(); got != "▾ shop|  web|  db|cache" {
		t.Fatalf("rows = %q", got)
	}
	group := t1.items[0]
	if group.cells[containerStateCol] != "running" || group.cells[containerStatusCol] != "2/2 running" {
		t.Errorf("project row = %q", group.cells)
	}

	// Actions on the project row apply to all its containers.
	selectID(t, t1, groupIDPrefix+"shop")
	if got := ids(a.targets()); got != "a1,b2" {
		t.Errorf("targets of the project row = %s", got)
	}
	if _, ok := a.selectedObject(); ok {
		t.Error("project row returned as a single object")
	}
	if a.handleKey(keyRunes("i")) != nil || a.pane != nil {
		t.Error("inspect opened for a project row")
	}
	t1.toggleMark()
	if got := ids(t1.markedItems()); got != "a1,b2" {
		t.Errorf("marking the project row marked %s", got)
	}
	t1.clearMarks()

	// Folding hides the members and keeps the cursor on the project; it
	// works from a member too.
	selectID(t, t1, "b2")
	a.handleKey(keyRunes("c"))
	if got := names(); got != "▸ shop|cache" {
		t.Errorf("collapsed rows = %q", got)
	}
	if it, _ := t1.selected(); it.id != groupIDPrefix+"shop" {
		t.Errorf("cursor on %q after folding", it.id)
	}
	// Filtering and marking still see the folded containers.
	t1.markAll()
	if got := ids(t1.markedItems()); got != "a1,b2,c3" {
		t.Errorf("mark all = %s", got)
	}
	a.handleKey(keyRunes("c"))
	if got := names(); got != "▾ shop|  web|  db|cache" {
		t.Errorf("expanded rows = %q", got)
	}
	t1.setFilter("db")
	if got := names(); got != "▾ shop|  db" {
		t.Errorf("filtered rows = %q", got)
	}
}

func keyRunes(s string) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)} }
