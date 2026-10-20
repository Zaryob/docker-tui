// Copyright (C) 2026 Suleyman Poyraz
// SPDX-License-Identifier: GPL-3.0-or-later

package ui

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/zaryob/docker-tui/internal/preferences"
)

func TestPreferencesPersistUIChangesAndProjects(t *testing.T) {
	a := newTestApp(t, http.NotFoundHandler())
	file := filepath.Join(t.TempDir(), "preferences.json")
	a.opts.PreferencesPath = file
	a.Update(keyRunes("o"))
	a.Update(keyRunes("O"))
	a.Update(keyRunes("a"))
	a.Update(keyRunes("/"))
	a.Update(keyRunes("shop"))
	a.Update(tea.KeyMsg{Type: tea.KeyEnter})
	project := localProject(t)
	project.LocalSource = true
	form := &composeForm{}
	a.pane = form
	a.Update(openComposeMsg{form: form, project: project})
	restored := New(a.client, Options{PreferencesPath: file})
	tab := restored.tabs[tabContainers]
	if tab.filter != "shop" || tab.sortCol != 0 || !tab.sortDesc || !restored.onlyRunning {
		t.Fatalf("restored filter=%q sort=%d/%v stopped=%v", tab.filter, tab.sortCol, tab.sortDesc, restored.onlyRunning)
	}
	if restored.projects[project.Name].ConfigFiles != project.ConfigFiles || !restored.projects[project.Name].LocalSource {
		t.Fatal("project source lost")
	}
	if restored.tabs[tabProjects].loaded {
		t.Fatal("restored projects suppress daemon discovery")
	}
	// Bookmarks cannot bleed into another selected daemon.
	other := newTestApp(t, http.NotFoundHandler())
	other.opts.PreferencesPath = file
	other.restorePreferences()
	if len(other.projects) != 0 {
		t.Fatal("project sources crossed daemon boundaries")
	}
	other.Update(keyRunes("o"))
	again := New(a.client, Options{PreferencesPath: file})
	if len(again.projects) != 1 {
		t.Fatal("saving another daemon discarded original bookmarks")
	}
}

func TestPreferencesValidateFieldsAndSurviveMalformedFile(t *testing.T) {
	a := newTestApp(t, http.NotFoundHandler())
	file := filepath.Join(t.TempDir(), "preferences.json")
	state := preferences.Default()
	state.Lists["containers"] = preferences.List{SortColumn: 999, SortDescending: true}
	if err := preferences.Save(file, state); err != nil {
		t.Fatal(err)
	}
	b := New(a.client, Options{PreferencesPath: file})
	if b.current().sortCol != -1 || b.current().sortDesc {
		t.Fatal("invalid column restored")
	}
	if err := os.WriteFile(file, []byte("{"), 0600); err != nil {
		t.Fatal(err)
	}
	b = New(a.client, Options{PreferencesPath: file})
	if !b.statusErr || b.current().sortCol != -1 {
		t.Fatal("malformed config not handled gracefully")
	}
	b.Update(keyRunes("o"))
	if _, err := preferences.Load(file); err != nil {
		t.Fatal("changing a preference did not repair malformed config")
	}
}
