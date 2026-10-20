// Copyright (C) 2026 Suleyman Poyraz
// SPDX-License-Identifier: GPL-3.0-or-later

package ui

import (
	"context"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/zaryob/docker-tui/internal/registry"
)

func TestRegistryFormMasksAndClearsPassword(t *testing.T) {
	a := newTestApp(t, http.NotFoundHandler())
	file := filepath.Join(t.TempDir(), "preferences.json")
	a.opts.PreferencesPath = file
	a.active = int(tabImages)
	a.Update(tea.KeyMsg{Type: tea.KeyCtrlL})
	f, ok := a.pane.(*registryForm)
	if !ok {
		t.Fatal("registry login form did not open")
	}
	f.inputs[0].SetValue("private.example")
	f.inputs[1].SetValue("alice")
	f.inputs[2].SetValue("private-password")
	if strings.Contains(f.view(), "private-password") {
		t.Fatal("password visible")
	}
	f.client.Command = func(ctx context.Context, _ string, _ ...string) *exec.Cmd {
		return exec.CommandContext(ctx, "sh", "-c", `cat >/dev/null; echo private-password; exit 1`)
	}
	cmd := f.update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil || f.inputs[2].Value() != "" {
		t.Fatal("password retained in form after submit")
	}
	result := cmd().(registryResultMsg)
	a.Update(result)
	if f.busy || !strings.Contains(f.err, "login failed") || strings.Contains(a.View(), "private-password") {
		t.Fatal("unsafe login failure")
	}
	f.inputs[2].SetValue("private-password")
	f.client.Command = func(ctx context.Context, _ string, _ ...string) *exec.Cmd {
		return exec.CommandContext(ctx, "sh", "-c", `cat >/dev/null`)
	}
	result = f.update(tea.KeyMsg{Type: tea.KeyEnter})().(registryResultMsg)
	a.Update(result)
	if a.pane != nil || strings.Contains(a.status, "private-password") {
		t.Fatal("login result leaked credentials or failed to close")
	}
	// Registry form changes do not save any credentials or input values.
	if data, err := os.ReadFile(file); err == nil && strings.Contains(string(data), "private-password") {
		t.Fatal("password persisted")
	}
}

func TestRegistryLogoutAndCanceledFormResults(t *testing.T) {
	a := newTestApp(t, http.NotFoundHandler())
	a.active = int(tabImages)
	a.Update(tea.KeyMsg{Type: tea.KeyCtrlO})
	old := a.pane.(*registryForm)
	if !old.logout || len(old.inputs) != 1 {
		t.Fatal("logout asks for credentials")
	}
	old.client = &registry.Client{Command: func(ctx context.Context, _ string, _ ...string) *exec.Cmd {
		return exec.CommandContext(ctx, "sh", "-c", "exit 0")
	}}
	result := old.update(tea.KeyMsg{Type: tea.KeyEnter})().(registryResultMsg)
	a.Update(result)
	if a.pane != nil || a.status != "registry credentials removed" {
		t.Fatal("logout result")
	}
	a.openRegistry(false)
	current := a.pane
	a.Update(registryResultMsg{form: old})
	if a.pane != current {
		t.Fatal("stale registry result closed a newer form")
	}
	current.close()
}
