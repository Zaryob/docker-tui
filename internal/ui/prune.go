// Copyright (C) 2026 Suleyman Poyraz
// SPDX-License-Identifier: GPL-3.0-or-later

package ui

import (
	"context"
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/zaryob/docker-tui/internal/docker"
	"github.com/zaryob/docker-tui/internal/format"
)

// pruneDialog asks for confirmation before pruning unused objects of the
// active tab.
func (a *App) pruneDialog() *confirmDialog {
	t := a.current()
	var (
		what  string
		prune func(context.Context) (docker.PruneReport, error)
	)
	switch t.kind {
	case tabContainers:
		what, prune = "all stopped containers", a.client.PruneContainers
	case tabImages:
		what, prune = "all dangling images", a.client.PruneImages
	case tabVolumes:
		what, prune = "all unused anonymous volumes", a.client.PruneVolumes
	case tabNetworks:
		what, prune = "all unused networks", a.client.PruneNetworks
	default:
		return nil
	}
	kind := t.kind
	return &confirmDialog{
		title:   "Prune " + t.noun + "?",
		message: "This removes " + what + ".",
		onYes: func() tea.Cmd {
			a.busy++
			return func() tea.Msg {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
				defer cancel()
				r, err := prune(ctx)
				if err != nil {
					return actionMsg{kind: kind, err: fmt.Errorf("prune: %w", err)}
				}
				return actionMsg{kind: kind, status: fmt.Sprintf("pruned %s, reclaimed %s",
					countNoun(r.Deleted, "objects"), format.Size(int64(r.SpaceReclaimed)))}
			}
		},
	}
}
