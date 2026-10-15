// Copyright (C) 2026 Suleyman Poyraz
// SPDX-License-Identifier: GPL-3.0-or-later

package ui

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

// pullServer serves a pull of two layers; release lets the pull finish.
func pullServer(t *testing.T, release <-chan struct{}) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/images/create" {
			http.NotFound(w, r)
			return
		}
		enc := json.NewEncoder(w)
		send := func(v map[string]any) {
			_ = enc.Encode(v)
			w.(http.Flusher).Flush()
		}
		send(map[string]any{"status": "Pulling from library/alpine", "id": "latest"})
		send(map[string]any{"status": "Already exists", "id": "aaa111"})
		send(map[string]any{"status": "Downloading", "id": "bbb222", "progressDetail": map[string]any{"current": 25, "total": 100}})
		select {
		case <-release:
		case <-r.Context().Done():
			return
		}
		send(map[string]any{"status": "Pull complete", "id": "bbb222"})
		send(map[string]any{"status": "Digest: sha256:123"})
		send(map[string]any{"status": "Status: Downloaded newer image for alpine:latest"})
	})
}

func TestPullInBackground(t *testing.T) {
	release := make(chan struct{})
	a := newTestApp(t, pullServer(t, release))
	a.switchTab(int(tabImages))
	r := newCmdRunner(a)
	r.run(func() tea.Msg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("u")} }, 200*time.Millisecond, nil)
	v, ok := a.pane.(*pullView)
	if !ok {
		t.Fatalf("pane = %T, want pull view", a.pane)
	}
	// q is typed into the reference.
	for _, k := range "alpine q" {
		a.Update(keyRunes(string(k)))
	}
	if v.input.Value() != "alpine q" {
		t.Fatalf("input = %q", v.input.Value())
	}
	v.input.SetValue("alpine")
	_, cmd := a.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if a.pull != v {
		t.Fatal("pull not tracked")
	}
	r.run(cmd, 2*time.Second, func() bool { return len(v.layers) == 2 })
	screen := ansi.Strip(v.view())
	if !strings.Contains(screen, "Pull: alpine") || !strings.Contains(screen, "1/2 layers") || !strings.Contains(screen, "Downloading") {
		t.Fatalf("view:\n%s", screen)
	}

	// esc keeps the pull running; u shows it again.
	a.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if a.pane != nil || a.pull != v {
		t.Fatalf("pane = %T, pull = %v", a.pane, a.pull)
	}
	if !strings.Contains(ansi.Strip(a.statusLine()), "pulling alpine 1/2") {
		t.Errorf("status line = %q", ansi.Strip(a.statusLine()))
	}
	a.Update(keyRunes("u"))
	if a.pane != pane(v) {
		t.Fatalf("u opened %T", a.pane)
	}
	a.Update(tea.KeyMsg{Type: tea.KeyEsc})

	close(release)
	r.run(nil, 3*time.Second, func() bool { return a.pull == nil })
	if !v.done || v.err != nil || a.status != "pulled alpine" {
		t.Errorf("done = %v, err = %v, status = %q", v.done, v.err, a.status)
	}
	if got := v.messages[len(v.messages)-1]; got != "Status: Downloaded newer image for alpine:latest" {
		t.Errorf("last message = %q", got)
	}
}

func TestCancelPull(t *testing.T) {
	a := newTestApp(t, pullServer(t, make(chan struct{})))
	a.switchTab(int(tabImages))
	a.Update(keyRunes("u"))
	v := a.pane.(*pullView)
	v.input.SetValue("alpine")
	_, cmd := a.Update(tea.KeyMsg{Type: tea.KeyEnter})
	r := newCmdRunner(a)
	r.run(cmd, 2*time.Second, func() bool { return len(v.layers) == 2 })
	a.Update(keyRunes("x"))
	r.run(nil, 3*time.Second, func() bool { return a.pull == nil })
	if !v.canceled || !v.done {
		t.Errorf("canceled = %v, done = %v", v.canceled, v.done)
	}
	if !strings.Contains(ansi.Strip(v.view()), "pull canceled") {
		t.Errorf("header = %q", strings.SplitN(ansi.Strip(v.view()), "\n", 2)[0])
	}
}

func TestPullFailure(t *testing.T) {
	a := newTestApp(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"error": "pull access denied for nope"})
	}))
	a.switchTab(int(tabImages))
	a.Update(keyRunes("u"))
	v := a.pane.(*pullView)
	v.input.SetValue("nope")
	_, cmd := a.Update(tea.KeyMsg{Type: tea.KeyEnter})
	newCmdRunner(a).run(cmd, 2*time.Second, func() bool { return a.pull == nil })
	if v.err == nil || !strings.Contains(ansi.Strip(v.view()), "pull access denied") {
		t.Errorf("err = %v", v.err)
	}
}
