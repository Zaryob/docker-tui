// Copyright (C) 2026 Suleyman Poyraz
// SPDX-License-Identifier: GPL-3.0-or-later

package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/zaryob/docker-tui/internal/docker"
)

func TestPullViewCompletesAllLayers(t *testing.T) {
	v := &pullView{ref: "busybox", progress: map[string]*layerProgress{}, height: 20}
	v.apply(docker.PullProgress{ID: "a12bd3844df3", Status: "Pull complete"})
	v.apply(docker.PullProgress{ID: "58c798e0a9cb", Status: "Download complete"})
	if v.layersDone() != 1 {
		t.Fatalf("while running: %d layers done, want 1", v.layersDone())
	}
	v.done = true
	screen := ansi.Strip(v.view())
	if !strings.Contains(screen, "2/2 layers") || strings.Contains(screen, "Download complete") {
		t.Errorf("finished view:\n%s", screen)
	}
	if strings.Contains(screen, "····") || strings.Contains(screen, "░") {
		t.Errorf("empty bar after the pull finished:\n%s", screen)
	}
}

func TestPullIndicatorWithoutLayers(t *testing.T) {
	a := &App{pull: &pullView{ref: "alpine", progress: map[string]*layerProgress{}}}
	if got := ansi.Strip(a.pullIndicator()); got != "⇣ pulling alpine" {
		t.Errorf("indicator = %q", got)
	}
}
