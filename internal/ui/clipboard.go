// Copyright (C) 2026 Suleyman Poyraz
// SPDX-License-Identifier: GPL-3.0-or-later

package ui

import (
	"github.com/atotto/clipboard"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/muesli/termenv"
)

// copyToClipboard writes text to the system clipboard, falling back to OSC 52.
func copyToClipboard(text string) error {
	if err := clipboard.WriteAll(text); err != nil {
		termenv.Copy(text)
		return err
	}
	return nil
}

// copyID copies the full ID (or name, for volumes) of the selected object to
// the clipboard.
func (a *App) copyID() tea.Cmd {
	it, ok := a.selectedObject()
	if !ok {
		return nil
	}
	id := it.objectID()
	copyToClipboard(id)
	return a.setStatus("copied "+id+" to clipboard", false)
}
