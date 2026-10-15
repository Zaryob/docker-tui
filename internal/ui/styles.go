// Copyright (C) 2026 Suleyman Poyraz
// SPDX-License-Identifier: GPL-3.0-or-later

package ui

import "github.com/charmbracelet/lipgloss"

var (
	// Base palette
	colorAccent = lipgloss.AdaptiveColor{Light: "#0B5CAD", Dark: "#2496ED"}
	colorMuted  = lipgloss.AdaptiveColor{Light: "#6C6C6C", Dark: "#8A8A8A"}
	colorError  = lipgloss.AdaptiveColor{Light: "#C62828", Dark: "#FF5F5F"}
	colorOK     = lipgloss.AdaptiveColor{Light: "#2E7D32", Dark: "#5FD787"}
	colorWarn   = lipgloss.AdaptiveColor{Light: "#B26A00", Dark: "#FFAF5F"}
	colorSelBg  = lipgloss.AdaptiveColor{Light: "#D6E8FA", Dark: "#1F3A5A"}

	// Modern TUI palette for logo and navigation
	colorDockerBlue   = lipgloss.Color("#00A6F4") // Logo containers
	colorDockerPale   = lipgloss.Color("#41758E") // Logo whale body
	colorActiveTabBg  = lipgloss.Color("#009FE3") // Active tab background
	colorActiveTabFg  = lipgloss.Color("#0D1117") // Active tab text (dark, bold)
	colorPassiveTabFg = lipgloss.Color("#A0AAB5") // Passive tab text (readable light gray)
	colorPassiveTabBg = lipgloss.Color("#1C242D") // Passive tab background
	colorTabSep       = lipgloss.Color("#36414D") // Tab and header separator
	colorTabHover     = lipgloss.Color("#263F52")

	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#FFFFFF")).
			Background(colorAccent).
			Padding(0, 1)

	// Modern compact tabs without underlines
	tabStyle = lipgloss.NewStyle().
			Background(colorPassiveTabBg).
			Foreground(colorPassiveTabFg).
			Padding(0, 1)

	activeTabStyle = lipgloss.NewStyle().
			Background(colorActiveTabBg).
			Foreground(colorActiveTabFg).
			Bold(true)

	tabSepStyle = lipgloss.NewStyle().Foreground(colorTabSep)

	headerInfoStyle = lipgloss.NewStyle().Foreground(colorPassiveTabFg).PaddingLeft(1)

	statusStyle = lipgloss.NewStyle().Foreground(colorMuted)
	errorStyle  = lipgloss.NewStyle().Foreground(colorError)
	okStyle     = lipgloss.NewStyle().Foreground(colorOK)
	warnStyle   = lipgloss.NewStyle().Foreground(colorWarn)
)
