// Copyright (C) 2026 Suleyman Poyraz
// SPDX-License-Identifier: GPL-3.0-or-later

package ui

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"github.com/zaryob/docker-tui/internal/buildx"
	"github.com/zaryob/docker-tui/internal/docker"
)

// TestScreenshots renders the screens shown in the README from fixtures and
// writes them, with colors, to the directory named by DOCKER_TUI_SCREENSHOTS.
// It is skipped otherwise. Turn the files into images with e.g.
//
//	freeze -x "cat containers.ansi" -o containers.png
func TestScreenshots(t *testing.T) {
	dir := os.Getenv("DOCKER_TUI_SCREENSHOTS")
	if dir == "" {
		t.Skip("DOCKER_TUI_SCREENSHOTS not set")
	}
	lipgloss.SetColorProfile(termenv.TrueColor)
	lipgloss.SetHasDarkBackground(true)
	t.Cleanup(func() { lipgloss.SetColorProfile(termenv.Ascii) })

	const width, height = 132, 26
	now := time.Now()
	hours := func(h int) int64 { return now.Add(-time.Duration(h) * time.Hour).Unix() }

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/json"):
			io.WriteString(w, `{"Config":{"Tty":true}}`)
		case r.URL.Path == "/containers/a1f3c9e2b7d4/logs":
			io.WriteString(w, strings.Join([]string{
				`2026-10-31T09:14:02.120000000Z 10.0.0.12 - - "GET /api/products?page=2 HTTP/1.1" 200 4821 2.1ms`,
				`2026-10-31T09:14:02.388000000Z 10.0.0.12 - - "GET /api/cart HTTP/1.1" 200 312 0.8ms`,
				`2026-10-31T09:14:03.014000000Z 10.0.0.31 - - "POST /api/checkout HTTP/1.1" 201 96 41.7ms`,
				`2026-10-31T09:14:03.502000000Z 10.0.0.12 - - "GET /static/app.js HTTP/1.1" 304 0 0.2ms`,
				`2026-10-31T09:14:05.771000000Z 10.0.0.44 - - "GET /api/products/7 HTTP/1.1" 404 51 1.1ms`,
			}, "\n")+"\n")
		case r.URL.Path == "/containers/b2e8d1a4c6f0/logs":
			io.WriteString(w, strings.Join([]string{
				`2026-10-31T09:14:01.901000000Z LOG:  checkpoint starting: time`,
				`2026-10-31T09:14:02.990000000Z LOG:  duration: 38.412 ms  statement: INSERT INTO orders ...`,
				`2026-10-31T09:14:04.233000000Z LOG:  checkpoint complete: wrote 42 buffers (0.3%)`,
				`2026-10-31T09:14:05.102000000Z ERROR:  duplicate key value violates unique constraint "orders_pkey"`,
			}, "\n")+"\n")
		default:
			io.WriteString(w, "[]")
		}
	}))
	defer srv.Close()
	client, err := docker.NewClient("tcp://" + strings.TrimPrefix(srv.URL, "http://"))
	if err != nil {
		t.Fatal(err)
	}
	a := New(client, Options{})
	a.version = docker.Version{Version: "28.3.2", APIVersion: "1.51", Os: "linux"}
	a.Update(tea.WindowSizeMsg{Width: width, Height: height})

	save := func(name string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name+".ansi"), []byte(a.View()), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	shop := map[string]string{composeProjectLabel: "shop"}
	containers := []docker.Container{
		{ID: "a1f3c9e2b7d4", Names: []string{"/shop-web-1"}, Image: "shop/web:1.8.2", State: "running", Status: "Up 3 days", Created: hours(72), Labels: shop,
			Ports: []docker.Port{{IP: "0.0.0.0", PrivatePort: 8080, PublicPort: 80, Type: "tcp"}, {IP: "0.0.0.0", PrivatePort: 8443, PublicPort: 443, Type: "tcp"}}},
		{ID: "b2e8d1a4c6f0", Names: []string{"/shop-db-1"}, Image: "postgres:16-alpine", State: "running", Status: "Up 3 days (healthy)", Created: hours(72), Labels: shop,
			Ports: []docker.Port{{PrivatePort: 5432, Type: "tcp"}}},
		{ID: "c3d7a2e9f1b5", Names: []string{"/shop-cache-1"}, Image: "redis:7", State: "running", Status: "Up 3 days", Created: hours(72), Labels: shop,
			Ports: []docker.Port{{PrivatePort: 6379, Type: "tcp"}}},
		{ID: "d4c1b8f7a2e6", Names: []string{"/shop-worker-1"}, Image: "shop/worker:1.8.2", State: "exited", Status: "Exited (1) 2 hours ago", Created: hours(72), Labels: shop},
		{ID: "e5a9f3d2c8b1", Names: []string{"/grafana"}, Image: "grafana/grafana:11.2.0", State: "running", Status: "Up 12 days", Created: hours(290),
			Ports: []docker.Port{{IP: "127.0.0.1", PrivatePort: 3000, PublicPort: 3000, Type: "tcp"}}},
		{ID: "f6b2e7c1d9a3", Names: []string{"/prometheus"}, Image: "prom/prometheus:v2.54.1", State: "running", Status: "Up 12 days", Created: hours(290),
			Ports: []docker.Port{{IP: "127.0.0.1", PrivatePort: 9090, PublicPort: 9090, Type: "tcp"}}},
		{ID: "a7c4d9e1f2b8", Names: []string{"/registry"}, Image: "registry:2", State: "paused", Status: "Up 5 days (Paused)", Created: hours(120),
			Ports: []docker.Port{{IP: "0.0.0.0", PrivatePort: 5000, PublicPort: 5000, Type: "tcp"}}},
		{ID: "b8d5e2f3a1c9", Names: []string{"/nightly-backup"}, Image: "offen/docker-volume-backup:v2", State: "exited", Status: "Exited (0) 7 hours ago", Created: hours(200)},
	}
	a.usage = map[string]containerUsage{
		"a1f3c9e2b7d4": {ok: true, cpu: 12.4, mem: 412 << 20},
		"b2e8d1a4c6f0": {ok: true, cpu: 3.1, mem: 1_288 << 20},
		"c3d7a2e9f1b5": {ok: true, cpu: 0.4, mem: 38 << 20},
		"e5a9f3d2c8b1": {ok: true, cpu: 1.7, mem: 221 << 20},
		"f6b2e7c1d9a3": {ok: true, cpu: 5.9, mem: 645 << 20},
	}
	a.Update(listMsg{kind: tabContainers, items: containerItems(containers)})
	selectID(t, a.current(), "b2e8d1a4c6f0")
	a.current().marked["a1f3c9e2b7d4"] = true
	a.current().marked["b2e8d1a4c6f0"] = true
	a.current().refresh()
	a.status = "stopped 1 container"
	save("containers")
	a.status = ""

	// Logs of the two marked containers.
	cmd := a.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	l, ok := a.pane.(*logView)
	if !ok {
		t.Fatalf("pane = %T", a.pane)
	}
	for cmd != nil {
		cmd = l.update(cmd())
	}
	l.setQuery("checkout")
	l.cursor = len(l.lines) - 1
	l.render()
	save("logs")
	a.handleKey(tea.KeyMsg{Type: tea.KeyEsc})
	a.handleKey(tea.KeyMsg{Type: tea.KeyEsc})

	// Stats of the database.
	a.handleKey(keyRunes("t"))
	sv, ok := a.pane.(*statsView)
	if !ok {
		t.Fatalf("pane = %T", a.pane)
	}
	var st docker.Stats
	st.PreCPUStats.CPUUsage.TotalUsage, st.PreCPUStats.SystemUsage = 1_000_000, 100_000_000
	st.CPUStats.CPUUsage.TotalUsage, st.CPUStats.SystemUsage = 1_062_000, 101_000_000
	st.CPUStats.OnlineCPUs = 8
	st.MemoryStats.Usage, st.MemoryStats.Limit = 1_288<<20, 8<<30
	st.Networks = map[string]struct {
		RxBytes uint64 `json:"rx_bytes"`
		TxBytes uint64 `json:"tx_bytes"`
	}{"eth0": {RxBytes: 3_874_112_000, TxBytes: 9_120_480_000}}
	st.BlkioStats.IoServiceBytesRecursive = []struct {
		Op    string `json:"op"`
		Value uint64 `json:"value"`
	}{{Op: "read", Value: 12_582_912_000}, {Op: "write", Value: 48_318_382_080}}
	st.PidsStats.Current = 14
	sv.loading, sv.stats = false, st
	sv.cpu = []float64{2.1, 2.4, 3.9, 8.2, 14.6, 31.0, 48.8, 22.1, 9.5, 4.2, 3.0, 2.8, 5.1, 12.7, 26.4, 41.9, 37.2, 18.0, 7.4, 3.3, 2.9, 6.0, 15.8, 33.5, 49.6, 44.1, 20.7, 8.8, 4.0, 3.1}
	save("stats")
	a.handleKey(tea.KeyMsg{Type: tea.KeyEsc})

	// Images.
	a.switchTab(int(tabImages))
	images := []docker.Image{
		{ID: "sha256:3f1a9c2e7b8d4f0a6c1e9b2d7a3f8c4e1b6d9a2f7c3e8b1d4a6f9c2e7b3d8a1f", RepoTags: []string{"shop/web:1.8.2"}, Created: hours(26), Size: 212 << 20, Containers: 1},
		{ID: "sha256:7b2d4e9f1a3c8e6b0d5f2a9c4e7b1d3f6a8c2e5b9d1f4a7c3e6b8d2f5a9c1e4b", RepoTags: []string{"shop/worker:1.8.2"}, Created: hours(26), Size: 198 << 20, Containers: 1},
		{ID: "sha256:9e4c1b7a2d8f3e6c0a5b9d2f7c4e1a8b3d6f9c2e5a7b1d4f8c3e6a9b2d5f7c1e", RepoTags: []string{"postgres:16-alpine"}, Created: hours(400), Size: 274 << 20, Containers: 1},
		{ID: "sha256:2a8f5c1e9b3d7a4f6c0e2b8d5a1f9c3e7b4d6a2f8c1e5b9d3a7f4c6e0b2d8a5f", RepoTags: []string{"redis:7"}, Created: hours(900), Size: 117 << 20, Containers: 1},
		{ID: "sha256:5d1e8a3c7f2b9e4d6a0c3f8b1e5d2a9c7f4b6e1d8a3c5f9b2e7d4a1c6f8b3e5d", RepoTags: []string{"grafana/grafana:11.2.0"}, Created: hours(1300), Size: 465 << 20, Containers: 1},
		{ID: "sha256:8c3b6f2d9a1e5c7b4f0d8a2e6c9b3f1d7a5e2c8b4f6d1a9e3c7b5f2d8a4e6c1b", RepoTags: []string{"prom/prometheus:v2.54.1"}, Created: hours(1500), Size: 271 << 20, Containers: 1},
		{ID: "sha256:1f7d3a9c5e2b8f4d6a0c7e3b9f1d5a2c8e4b6f0d3a7c1e9b5f2d8a4c6e0b3f7d", RepoTags: []string{"alpine:3.21"}, Created: hours(2200), Size: 8 << 20},
		{ID: "sha256:6a2c9e5f1b7d3a8c4e0f6b2d9a5c1e7f3b8d4a0c6e2f9b5d1a7c3e8f4b0d6a2c", RepoTags: []string{"<none>:<none>"}, Created: hours(30), Size: 201 << 20},
		{ID: "sha256:4e8b1d6f3a9c2e7b5d0f4a8c1e6b3d9f2a7c5e1b8d4f0a6c3e9b2d7f5a1c8e4b", RepoTags: []string{"node:22-alpine"}, Created: hours(600), Size: 158 << 20},
	}
	a.Update(listMsg{kind: tabImages, items: imageItems(images, now)})
	a.current().table.Move(2)
	save("images")

	// Builds.
	a.switchTab(int(tabBuilds))
	records := []buildx.Record{
		{Ref: "default/default/k9x2m4p7q1", Name: "shop/web:1.8.2", Status: "completed", CreatedAt: now.Add(-26 * time.Hour), CompletedAt: now.Add(-26*time.Hour + 84*time.Second), TotalSteps: 14, CompletedSteps: 14, CachedSteps: 9},
		{Ref: "default/default/h3v8n1r5t2", Name: "shop/worker:1.8.2", Status: "completed", CreatedAt: now.Add(-26 * time.Hour), CompletedAt: now.Add(-26*time.Hour + 61*time.Second), TotalSteps: 11, CompletedSteps: 11, CachedSteps: 8},
		{Ref: "default/default/f7c2w9b4s6", Name: "shop/web:1.8.1", Status: "failed", CreatedAt: now.Add(-50 * time.Hour), CompletedAt: now.Add(-50*time.Hour + 37*time.Second), TotalSteps: 14, CompletedSteps: 9, CachedSteps: 9},
		{Ref: "default/default/d1z5y8k3m0", Name: "shop/web:1.8.0", Status: "completed", CreatedAt: now.Add(-9 * 24 * time.Hour), CompletedAt: now.Add(-9*24*time.Hour + 312*time.Second), TotalSteps: 14, CompletedSteps: 14, CachedSteps: 2},
		{Ref: "default/default/q6t3p9x1v4", Name: "tools/migrate:latest", Status: "completed", CreatedAt: now.Add(-11 * 24 * time.Hour), CompletedAt: now.Add(-11*24*time.Hour + 142*time.Second), TotalSteps: 7, CompletedSteps: 7},
	}
	a.Update(listMsg{kind: tabBuilds, items: buildItems(records, now)})
	save("builds")
}
