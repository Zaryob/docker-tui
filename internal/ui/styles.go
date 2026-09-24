// Copyright (C) 2026 Suleyman Poyraz
// SPDX-License-Identifier: GPL-3.0-or-later

package ui

import "github.com/charmbracelet/lipgloss"

var (
	colorAccent = lipgloss.AdaptiveColor{Light: "#0B5CAD", Dark: "#2496ED"}
	colorMuted  = lipgloss.AdaptiveColor{Light: "#6C6C6C", Dark: "#8A8A8A"}
	colorError  = lipgloss.AdaptiveColor{Light: "#C62828", Dark: "#FF5F5F"}
	colorOK     = lipgloss.AdaptiveColor{Light: "#2E7D32", Dark: "#5FD787"}
	colorWarn   = lipgloss.AdaptiveColor{Light: "#B26A00", Dark: "#FFAF5F"}
	colorSelBg  = lipgloss.AdaptiveColor{Light: "#D6E8FA", Dark: "#1F3A5A"}

	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#FFFFFF")).
			Background(colorAccent).
			Padding(0, 1)

	tabStyle       = lipgloss.NewStyle().Foreground(colorMuted).Padding(0, 1)
	activeTabStyle = lipgloss.NewStyle().Foreground(colorAccent).Bold(true).Underline(true).Padding(0, 1)

	headerInfoStyle = lipgloss.NewStyle().Foreground(colorMuted).PaddingLeft(1)

	statusStyle = lipgloss.NewStyle().Foreground(colorMuted)
	errorStyle  = lipgloss.NewStyle().Foreground(colorError)
	okStyle     = lipgloss.NewStyle().Foreground(colorOK)
	warnStyle   = lipgloss.NewStyle().Foreground(colorWarn)
)
