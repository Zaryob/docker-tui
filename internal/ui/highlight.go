// Copyright (C) 2026 Suleyman Poyraz
// SPDX-License-Identifier: GPL-3.0-or-later

package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

var (
	jsonKeyStyle    = lipgloss.NewStyle().Foreground(colorAccent)
	jsonStringStyle = lipgloss.NewStyle().Foreground(colorOK)
	jsonNumberStyle = lipgloss.NewStyle().Foreground(colorWarn)
	jsonNullStyle   = lipgloss.NewStyle().Foreground(colorMuted)
)

// highlightJSON adds colors to (already indented) JSON text. It does not
// validate its input; anything it does not recognize is copied unchanged.
func highlightJSON(src string) string {
	var b strings.Builder
	b.Grow(len(src) * 2)
	for i := 0; i < len(src); {
		c := src[i]
		switch {
		case c == '"':
			end := scanString(src, i)
			tok := src[i:end]
			// A string followed by a colon is an object key.
			rest := strings.TrimLeft(src[end:], " ")
			if strings.HasPrefix(rest, ":") {
				b.WriteString(jsonKeyStyle.Render(tok))
			} else {
				b.WriteString(jsonStringStyle.Render(tok))
			}
			i = end
		case c == '-' || (c >= '0' && c <= '9'):
			end := i + 1
			for end < len(src) && strings.IndexByte("0123456789.eE+-", src[end]) >= 0 {
				end++
			}
			b.WriteString(jsonNumberStyle.Render(src[i:end]))
			i = end
		case strings.HasPrefix(src[i:], "true"):
			b.WriteString(jsonNumberStyle.Render("true"))
			i += 4
		case strings.HasPrefix(src[i:], "false"):
			b.WriteString(jsonNumberStyle.Render("false"))
			i += 5
		case strings.HasPrefix(src[i:], "null"):
			b.WriteString(jsonNullStyle.Render("null"))
			i += 4
		default:
			b.WriteByte(c)
			i++
		}
	}
	return b.String()
}

// scanString returns the index just past the JSON string starting at i.
func scanString(src string, i int) int {
	for j := i + 1; j < len(src); j++ {
		switch src[j] {
		case '\\':
			j++
		case '"':
			return j + 1
		case '\n':
			return j
		}
	}
	return len(src)
}
