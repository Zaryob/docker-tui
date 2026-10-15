// Copyright (C) 2026 Suleyman Poyraz
// SPDX-License-Identifier: GPL-3.0-or-later

package ui

import (
	"github.com/atotto/clipboard"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/muesli/termenv"
)

// copyID copies the full ID (or name, for volumes) of the selected object to
// the clipboard.
func (a *App) copyID() tea.Cmd {
	it, ok := a.selectedObject()
	if !ok {
		return nil
	}
	if err := clipboard.WriteAll(it.id); err != nil {
		// No clipboard utility available (e.g. over SSH): fall back to the
		// OSC 52 escape sequence, which most terminal emulators support.
		termenv.Copy(it.id)
	}
	return a.setStatus("copied "+it.id+" to clipboard", false)
}
