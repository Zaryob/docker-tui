// Copyright (C) 2026 Suleyman Poyraz
// SPDX-License-Identifier: GPL-3.0-or-later

package ui

import (
	"net/http"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestContainerPath(t *testing.T) {
	for s, want := range map[string]string{"web:/etc": "/etc", "web:": "/", "web-1:/x": "", "/tmp": ""} {
		p, ok := containerPath(s, "web")
		if ok != (want != "") || p != want {
			t.Errorf("containerPath(%q) = %q, %v", s, p, ok)
		}
	}
}

func TestCopyForm(t *testing.T) {
	a := newTestApp(t, http.NotFoundHandler())
	t1 := a.current()
	t1.setItems(containerItems(containersFixture()))
	selectID(t, t1, "c3") // works on stopped containers
	a.handleKey(keyRunes("f"))
	f, ok := a.pane.(*copyForm)
	if !ok {
		t.Fatalf("pane = %T", a.pane)
	}
	if f.inputs[0].Value() != "cache:/" {
		t.Errorf("from = %q", f.inputs[0].Value())
	}
	// Both sides local is refused.
	f.inputs[0].SetValue("/tmp/a")
	if cmd := a.handleKey(tea.KeyMsg{Type: tea.KeyEnter}); cmd != nil || f.err == "" {
		t.Error("copy between two local paths accepted")
	}
	f.inputs[0].SetValue("cache:/data")
	f.inputs[1].SetValue("/tmp/out")
	a.handleKey(tea.KeyMsg{Type: tea.KeyCtrlS})
	if f.inputs[0].Value() != "/tmp/out" || f.inputs[1].Value() != "cache:/data" {
		t.Errorf("swap = %q -> %q", f.inputs[0].Value(), f.inputs[1].Value())
	}
	msg, ok := a.handleKey(tea.KeyMsg{Type: tea.KeyEnter})().(copyMsg)
	if !ok || msg.id != "c3" || msg.from != "/tmp/out" || msg.to != "cache:/data" {
		t.Errorf("copy = %+v", msg)
	}
}
