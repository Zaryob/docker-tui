// Copyright (C) 2026 Suleyman Poyraz
// SPDX-License-Identifier: GPL-3.0-or-later

package ui

import (
	"crypto/sha256"
	"fmt"
	"sort"
	"strings"

	"github.com/zaryob/docker-tui/internal/buildx"
	"github.com/zaryob/docker-tui/internal/preferences"
)

func (a *App) daemonKey() string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(a.client.Host()+"\x00"+a.client.Context())))
}

func (a *App) restorePreferences() {
	if a.opts.PreferencesPath == "" {
		return
	}
	state, err := preferences.Load(a.opts.PreferencesPath)
	a.savedPreferences = state
	if err != nil {
		a.status, a.statusErr = err.Error(), true
	}
	a.onlyRunning = state.OnlyRunning
	for _, t := range a.tabs {
		saved, ok := state.Lists[strings.ToLower(t.title)]
		if !ok {
			continue
		}
		// A newer/edited config must never index a nonexistent column.
		if saved.SortColumn < -1 || saved.SortColumn >= len(t.table.columns) {
			saved.SortColumn = -1
			saved.SortDescending = false
		}
		if len(saved.Filter) > 4096 {
			saved.Filter = ""
		}
		t.filter, t.sortCol, t.sortDesc = saved.Filter, saved.SortColumn, saved.SortDescending
		t.table.SetSort(t.sortCol, t.sortDesc)
		t.refresh()
	}
	for _, saved := range state.Projects[a.daemonKey()] {
		if saved.Name == "" || saved.Files == "" {
			continue
		}
		a.projects[saved.Name] = buildx.Project{Name: saved.Name, ConfigFiles: saved.Files, WorkingDir: saved.WorkingDirectory, Status: "not running"}
	}
	// Rebuild rows without marking discovery as complete; the first visit still
	// queries the selected daemon for running and externally created projects.
	a.refreshProjects()
	a.tabs[tabProjects].loaded = false
}

func (a *App) preferenceSnapshot() preferences.State {
	state := preferences.Default()
	state.OnlyRunning = a.onlyRunning
	for _, t := range a.tabs {
		state.Lists[strings.ToLower(t.title)] = preferences.List{Filter: t.filter, SortColumn: t.sortCol, SortDescending: t.sortDesc}
	}
	// Preserve sources associated with other daemons/contexts.
	for daemon, projects := range a.savedPreferences.Projects {
		state.Projects[daemon] = projects
	}
	var projects []preferences.Project
	for _, p := range a.projects {
		if p.ConfigFiles != "" {
			projects = append(projects, preferences.Project{Name: p.Name, Files: p.ConfigFiles, WorkingDirectory: p.WorkingDir})
		}
	}
	sort.Slice(projects, func(i, j int) bool { return projects[i].Name < projects[j].Name })
	if len(projects) > 0 {
		state.Projects[a.daemonKey()] = projects
	} else {
		delete(state.Projects, a.daemonKey())
	}
	return state
}
