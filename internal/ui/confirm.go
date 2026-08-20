// Copyright (C) 2026 Suleyman Poyraz
// SPDX-License-Identifier: GPL-3.0-or-later

package ui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var (
	dialogStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorWarn).
			Padding(1, 3)
	dialogTitleStyle = lipgloss.NewStyle().Bold(true).Foreground(colorWarn)
	dialogKeyStyle   = lipgloss.NewStyle().Bold(true).Foreground(colorAccent)
)

// confirmDialog asks the user to confirm a destructive action.
type confirmDialog struct {
	title   string
	message string
	// onYes runs when the user confirms with "y".
	onYes func() tea.Cmd
	// onForce, when set, offers an additional "f" choice for forced removal.
	onForce func() tea.Cmd
}

// handleKey returns the command to run and whether the dialog is finished.
func (d *confirmDialog) handleKey(msg tea.KeyMsg) (tea.Cmd, bool) {
	switch msg.String() {
	case "y", "Y", "enter":
		return d.onYes(), true
	case "f", "F":
		if d.onForce != nil {
			return d.onForce(), true
		}
	case "n", "N", "esc", "q":
		return nil, true
	}
	return nil, false
}

func (d *confirmDialog) view(width, height int) string {
	choices := []string{dialogKeyStyle.Render("y") + " yes"}
	if d.onForce != nil {
		choices = append(choices, dialogKeyStyle.Render("f")+" force")
	}
	choices = append(choices, dialogKeyStyle.Render("n")+" cancel")

	body := strings.Join([]string{
		dialogTitleStyle.Render(d.title),
		"",
		d.message,
		"",
		strings.Join(choices, "   "),
	}, "\n")
	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, dialogStyle.Render(body))
}
