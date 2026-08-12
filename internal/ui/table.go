// Copyright (C) 2026 Suleyman Poyraz
// SPDX-License-Identifier: GPL-3.0-or-later

package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
)

const columnGap = 2

var (
	tableHeaderStyle = lipgloss.NewStyle().Bold(true).Foreground(colorAccent)
	selectedRowStyle = lipgloss.NewStyle().Background(colorSelBg).Bold(true)
)

// Column describes a table column. A Width of zero makes the column flexible:
// flexible columns share whatever horizontal space is left after the fixed
// columns have been laid out.
type Column struct {
	Title string
	Width int
}

// Table is a simple scrollable table with a single-row cursor.
type Table struct {
	columns []Column
	rows    [][]string
	cursor  int
	offset  int
	width   int
	height  int
}

// NewTable returns a table with the given columns.
func NewTable(columns ...Column) *Table {
	return &Table{columns: columns}
}

// SetSize sets the outer size of the table, including the header row.
func (t *Table) SetSize(width, height int) {
	t.width, t.height = width, height
	t.clamp()
}

// SetRows replaces the table rows, keeping the cursor in range.
func (t *Table) SetRows(rows [][]string) {
	t.rows = rows
	t.clamp()
}

// Len returns the number of rows.
func (t *Table) Len() int { return len(t.rows) }

// Cursor returns the index of the selected row.
func (t *Table) Cursor() int { return t.cursor }

// SetCursor moves the cursor to row i.
func (t *Table) SetCursor(i int) {
	t.cursor = i
	t.clamp()
}

// Move moves the cursor by delta rows.
func (t *Table) Move(delta int) { t.SetCursor(t.cursor + delta) }

// Top moves the cursor to the first row.
func (t *Table) Top() { t.SetCursor(0) }

// Bottom moves the cursor to the last row.
func (t *Table) Bottom() { t.SetCursor(len(t.rows) - 1) }

// PageSize returns the number of visible body rows.
func (t *Table) PageSize() int {
	if t.height <= 1 {
		return 1
	}
	return t.height - 1
}

func (t *Table) clamp() {
	if t.cursor >= len(t.rows) {
		t.cursor = len(t.rows) - 1
	}
	if t.cursor < 0 {
		t.cursor = 0
	}
	page := t.PageSize()
	if t.cursor < t.offset {
		t.offset = t.cursor
	}
	if t.cursor >= t.offset+page {
		t.offset = t.cursor - page + 1
	}
	if max := len(t.rows) - page; t.offset > max {
		t.offset = max
	}
	if t.offset < 0 {
		t.offset = 0
	}
}

func (t *Table) widths() []int {
	widths := make([]int, len(t.columns))
	fixed, flex := 0, 0
	for i, c := range t.columns {
		if c.Width > 0 {
			widths[i] = c.Width
			fixed += c.Width
		} else {
			flex++
		}
	}
	fixed += columnGap * (len(t.columns) - 1)
	if flex > 0 {
		remaining := t.width - fixed
		if remaining < flex*8 {
			remaining = flex * 8
		}
		share := remaining / flex
		for i, c := range t.columns {
			if c.Width == 0 {
				widths[i] = share
			}
		}
	}
	return widths
}

func renderCells(cells []string, widths []int) string {
	parts := make([]string, len(widths))
	for i, w := range widths {
		cell := ""
		if i < len(cells) {
			cell = cells[i]
		}
		cell = runewidth.Truncate(cell, w, "…")
		parts[i] = runewidth.FillRight(cell, w)
	}
	return strings.Join(parts, strings.Repeat(" ", columnGap))
}

// fit pads or truncates a rendered line to exactly the table width.
func (t *Table) fit(line string) string {
	if t.width <= 0 {
		return line
	}
	return runewidth.FillRight(runewidth.Truncate(line, t.width, ""), t.width)
}

// View renders the table.
func (t *Table) View() string {
	widths := t.widths()
	titles := make([]string, len(t.columns))
	for i, c := range t.columns {
		titles[i] = c.Title
	}

	lines := make([]string, 0, t.height)
	lines = append(lines, tableHeaderStyle.Render(t.fit(renderCells(titles, widths))))

	end := t.offset + t.PageSize()
	if end > len(t.rows) {
		end = len(t.rows)
	}
	for i := t.offset; i < end; i++ {
		line := t.fit(renderCells(t.rows[i], widths))
		if i == t.cursor {
			line = selectedRowStyle.Render(line)
		}
		lines = append(lines, line)
	}
	for len(lines) < t.height {
		lines = append(lines, "")
	}
	return strings.Join(lines, "\n")
}
