// Copyright (C) 2026 Suleyman Poyraz
// SPDX-License-Identifier: GPL-3.0-or-later

package ui

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/zaryob/docker-tui/internal/docker"
)

func TestPushView(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/images/myuser/app/push" {
			enc := json.NewEncoder(w)
			_ = enc.Encode(map[string]any{"status": "Pushing", "id": "layer1", "progressDetail": map[string]any{"current": 50, "total": 100}})
			_ = enc.Encode(map[string]any{"status": "Pushed", "id": "layer1"})
			return
		}
		_ = json.NewEncoder(w).Encode([]docker.Image{})
	})

	a := newTestApp(t, handler)
	a.switchTab(int(tabImages))
	img := docker.Image{ID: "img123", RepoTags: []string{"myuser/app:v1.0"}}
	a.tabs[tabImages].setItems(imageItems([]docker.Image{img}, time.Now()))

	// Press 'p' to open push view
	a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("p")})
	v, ok := a.pane.(*pushView)
	if !ok {
		t.Fatalf("pane = %T, want *pushView", a.pane)
	}
	if v.input.Value() != "myuser/app:v1.0" {
		t.Errorf("got prefilled ref %q", v.input.Value())
	}

	cmd := v.update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected startPush cmd")
	}
	if !v.running() {
		t.Errorf("expected push to be running")
	}
}
