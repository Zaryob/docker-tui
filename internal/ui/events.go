// Copyright (C) 2026 Suleyman Poyraz
// SPDX-License-Identifier: GPL-3.0-or-later

package ui

import (
	"context"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/zaryob/docker-tui/internal/docker"
)

const (
	// eventDebounce collects bursts of events, e.g. from "docker compose
	// up", into a single reload.
	eventDebounce = 250 * time.Millisecond
	// eventRetry is the delay before reconnecting a broken event stream.
	eventRetry = 5 * time.Second
)

// eventsMsg reports which tabs are affected by the events received since
// the previous message.
type eventsMsg struct {
	kinds map[tabKind]bool
}

// eventTabs returns the tabs whose lists change because of ev.
func eventTabs(ev docker.Event) []tabKind {
	switch ev.Type {
	case "container":
		// These actions do not change anything shown in the lists; "top"
		// is also emitted by the process view every few seconds.
		if strings.HasPrefix(ev.Action, "exec_") {
			return nil
		}
		switch ev.Action {
		case "attach", "detach", "resize", "top", "archive-path", "extract-to-dir", "export", "copy":
			return nil
		case "create", "destroy":
			// The image list counts the containers using each image.
			return []tabKind{tabContainers, tabImages}
		}
		return []tabKind{tabContainers}
	case "image":
		return []tabKind{tabImages}
	case "volume":
		if ev.Action == "mount" || ev.Action == "unmount" {
			return nil
		}
		return []tabKind{tabVolumes}
	case "network":
		if ev.Action == "connect" || ev.Action == "disconnect" {
			return nil
		}
		return []tabKind{tabNetworks}
	}
	return nil
}

// watchEvents follows the daemon events for the lifetime of the program and
// sends the affected tabs to ch. The stream is reopened after errors.
func watchEvents(ctx context.Context, client *docker.Client, ch chan<- tabKind) {
	for ctx.Err() == nil {
		s, err := client.Events(ctx, "container", "image", "volume", "network")
		if err == nil {
			for {
				ev, err := s.Next()
				if err != nil {
					break
				}
				for _, k := range eventTabs(ev) {
					select {
					case ch <- k:
					case <-ctx.Done():
					}
				}
			}
			s.Close()
		}
		select {
		case <-time.After(eventRetry):
		case <-ctx.Done():
		}
	}
}

// waitEvents returns a command that blocks until events arrive and then
// reports every tab affected within the debounce window.
func waitEvents(ch <-chan tabKind) tea.Cmd {
	return func() tea.Msg {
		k, ok := <-ch
		if !ok {
			return nil
		}
		kinds := map[tabKind]bool{k: true}
		deadline := time.After(eventDebounce)
		for {
			select {
			case k, ok := <-ch:
				if !ok {
					return eventsMsg{kinds: kinds}
				}
				kinds[k] = true
			case <-deadline:
				return eventsMsg{kinds: kinds}
			}
		}
	}
}
