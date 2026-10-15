// Copyright (C) 2026 Suleyman Poyraz
// SPDX-License-Identifier: GPL-3.0-or-later

package ui

import (
	"slices"
	"testing"

	"github.com/zaryob/docker-tui/internal/docker"
)

func TestEventTabs(t *testing.T) {
	tests := []struct {
		typ, action string
		want        []tabKind
	}{
		{"container", "start", []tabKind{tabContainers}},
		{"container", "health_status: healthy", []tabKind{tabContainers}},
		{"container", "create", []tabKind{tabContainers, tabImages}},
		{"container", "exec_start: sh", nil},
		{"container", "top", nil},
		{"image", "pull", []tabKind{tabImages, tabBuilds}},
		{"volume", "create", []tabKind{tabVolumes}},
		{"volume", "mount", nil},
		{"network", "destroy", []tabKind{tabNetworks}},
		{"network", "connect", nil},
		{"daemon", "reload", nil},
	}
	for _, tt := range tests {
		got := eventTabs(docker.Event{Type: tt.typ, Action: tt.action})
		if !slices.Equal(got, tt.want) {
			t.Errorf("%s %s: %v, want %v", tt.typ, tt.action, got, tt.want)
		}
	}
}

func TestWaitEventsCollectsBurst(t *testing.T) {
	ch := make(chan tabKind, 4)
	ch <- tabContainers
	ch <- tabImages
	ch <- tabContainers
	msg, ok := waitEvents(ch)().(eventsMsg)
	if !ok {
		t.Fatal("expected eventsMsg")
	}
	if len(msg.kinds) != 2 || !msg.kinds[tabContainers] || !msg.kinds[tabImages] {
		t.Errorf("kinds = %v", msg.kinds)
	}
}
