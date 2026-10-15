// Copyright (C) 2026 Suleyman Poyraz
// SPDX-License-Identifier: GPL-3.0-or-later

package ui

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestUsageColumns(t *testing.T) {
	a := newTestApp(t, http.NotFoundHandler())
	t1 := a.current()
	items := containerItems(containersFixture())
	applyUsage(items, map[string]containerUsage{
		"a1": {ok: true, cpu: 12.5, mem: 60_000_000},
		"b2": {ok: true, cpu: 2.25, mem: 30_000_000},
	})
	t1.setItems(items)
	row := func(id string) []string {
		for _, it := range t1.items {
			if it.id == id {
				return it.cells
			}
		}
		t.Fatalf("no row %s", id)
		return nil
	}
	if c := row("a1"); c[containerCPUCol] != "12.5%" || c[containerMemCol] != "60.0MB" {
		t.Errorf("web usage = %q / %q", c[containerCPUCol], c[containerMemCol])
	}
	if c := row("c3"); c[containerCPUCol] != "" || c[containerMemCol] != "" {
		t.Errorf("stopped container usage = %q / %q", c[containerCPUCol], c[containerMemCol])
	}
	// The project row shows the sum of its containers.
	if c := row(groupIDPrefix + "shop"); c[containerCPUCol] != "14.8%" {
		t.Errorf("project CPU = %q, want 14.8%%", c[containerCPUCol])
	} else if c[containerMemCol] != "90.0MB" {
		t.Errorf("project memory = %q, want 90.0MB", c[containerMemCol])
	}
	// Sorting by CPU is numeric.
	if k := t1.all[0].sortKeys[containerCPUCol]; k != "00000000000000012500" {
		t.Errorf("sort key = %q", k)
	}
}

func TestUsagePolling(t *testing.T) {
	stats := map[string]any{
		"cpu_stats":    map[string]any{"cpu_usage": map[string]any{"total_usage": 300}, "system_cpu_usage": 2000, "online_cpus": 2},
		"precpu_stats": map[string]any{"cpu_usage": map[string]any{"total_usage": 100}, "system_cpu_usage": 1000},
		"memory_stats": map[string]any{"usage": 50 << 20},
	}
	a := newTestApp(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/stats") {
			_ = json.NewEncoder(w).Encode(stats)
			return
		}
		http.NotFound(w, r)
	}))
	_, cmd := a.Update(listMsg{kind: tabContainers, items: containerItems(containersFixture())})
	if cmd == nil || !a.usagePolling {
		t.Fatal("usage sampling not started")
	}
	newCmdRunner(a).run(cmd, 3*time.Second, func() bool { return len(a.usage) == 2 })
	if u := a.usage["a1"]; !u.ok || u.cpu != 40 || u.mem != 50<<20 {
		t.Errorf("usage of web = %+v", u)
	}
	if _, ok := a.usage["c3"]; ok {
		t.Error("stopped container sampled")
	}
	// A second list does not start a second sampling loop.
	if _, cmd := a.Update(listMsg{kind: tabContainers, items: containerItems(containersFixture())}); cmd != nil {
		if _, ok := cmd().(usageMsg); ok {
			t.Error("second sampling loop started")
		}
	}
}
