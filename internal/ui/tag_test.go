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

func TestTagImageForm(t *testing.T) {
	var requestedPath, requestedRepo, requestedTag string
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/images/img123/tag" {
			requestedPath = r.URL.Path
			requestedRepo = r.URL.Query().Get("repo")
			requestedTag = r.URL.Query().Get("tag")
			w.WriteHeader(http.StatusOK)
			return
		}
		_ = json.NewEncoder(w).Encode([]docker.Image{})
	})

	a := newTestApp(t, handler)
	a.switchTab(int(tabImages))
	img := docker.Image{ID: "img123", RepoTags: []string{"app:old"}}
	a.tabs[tabImages].setItems(imageItems([]docker.Image{img}, time.Now()))

	// Press 't' to open tag form
	a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("t")})
	f, ok := a.pane.(*tagForm)
	if !ok {
		t.Fatalf("pane = %T, want *tagForm", a.pane)
	}

	f.input.SetValue("myuser/app:v2.0")
	cmd := a.pane.update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected command from Enter in tagForm")
	}

	msg := cmd()
	submit, ok := msg.(tagSubmitMsg)
	if !ok {
		t.Fatalf("msg = %T, want tagSubmitMsg", msg)
	}
	if submit.imageID != "img123" || submit.target != "myuser/app:v2.0" {
		t.Errorf("unexpected submit: %+v", submit)
	}

	// Dispatch submit to App
	_, actionCmd := a.Update(submit)
	if actionCmd == nil {
		t.Fatal("expected actionCmd")
	}
	actionRes := actionCmd()
	act, ok := actionRes.(actionMsg)
	if !ok {
		t.Fatalf("res = %T, want actionMsg", actionRes)
	}
	if act.err != nil {
		t.Fatal(act.err)
	}
	if requestedPath != "/images/img123/tag" || requestedRepo != "myuser/app" || requestedTag != "v2.0" {
		t.Errorf("got path=%s, repo=%s, tag=%s", requestedPath, requestedRepo, requestedTag)
	}
}
