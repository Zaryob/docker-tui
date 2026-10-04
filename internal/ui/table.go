// Copyright (C) 2026 Suleyman Poyraz
// SPDX-License-Identifier: GPL-3.0-or-later

package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/mattn/go-runewidth"
)

const columnGap = 2

var (
	tableHeaderStyle = lipgloss.NewStyle().Bold(true).Foreground(colorAccent)
	selectedRowStyle = lipgloss.NewStyle().Background(colorSelBg).Bold(true)
)

// Column describes a table column. A Width of zero makes the column flexible:
// flexible columns share whatever horizontal space is left after the fixed
// columns have been laid out, in proportion to their Grow weight (default 1).
type Column struct {
	Title string
	Width int
	Grow  int
}

func (c Column) weight() int {
	if c.Grow > 0 {
		return c.Grow
	}
	return 1
}

// Table is a simple scrollable table with a single-row cursor.
type Table struct {
	columns  []Column
	rows     [][]string
	cursor   int
	offset   int
	width    int
	height   int
	sortCol  int
	sortDesc bool
	// cellStyle, when set, returns the style for a cell of an unselected row.
	cellStyle cellStyler
}

// NewTable returns a table with the given columns.
func NewTable(columns ...Column) *Table {
	return &Table{columns: columns, sortCol: -1}
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

// SetCellStyle sets a function that styles individual cells.
func (t *Table) SetCellStyle(fn cellStyler) {
	t.cellStyle = fn
}

// SetSort marks column col as the sort column in the header; -1 clears it.
func (t *Table) SetSort(col int, desc bool) {
	t.sortCol, t.sortDesc = col, desc
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
	const minFlex = 8
	widths := make([]int, len(t.columns))
	fixed, weights := 0, 0
	for i, c := range t.columns {
		if c.Width > 0 {
			widths[i] = c.Width
			fixed += c.Width
		} else {
			weights += c.weight()
		}
	}
	fixed += columnGap * (len(t.columns) - 1)
	if weights == 0 {
		return widths
	}
	remaining := t.width - fixed
	last := -1
	used := 0
	for i, c := range t.columns {
		if c.Width == 0 {
			widths[i] = max(remaining*c.weight()/weights, minFlex)
			used += widths[i]
			last = i
		}
	}
	// Give the rounding remainder to the last flexible column.
	if extra := remaining - used; extra > 0 {
		widths[last] += extra
	}
	return widths
}

type cellStyler func(col int, value string) (lipgloss.Style, bool)

func renderCells(cells []string, widths []int, style cellStyler) string {
	parts := make([]string, len(widths))
	for i, w := range widths {
		cell := ""
		if i < len(cells) {
			cell = cells[i]
		}
		raw := cell
		cell = runewidth.FillRight(runewidth.Truncate(cell, w, "…"), w)
		if style != nil {
			if st, ok := style(i, raw); ok {
				cell = st.Render(cell)
			}
		}
		parts[i] = cell
	}
	return strings.Join(parts, strings.Repeat(" ", columnGap))
}

// fit pads or truncates a rendered line to exactly the table width. It is
// aware of ANSI escape sequences.
func (t *Table) fit(line string) string {
	if t.width <= 0 {
		return line
	}
	line = ansi.Truncate(line, t.width, "")
	if pad := t.width - ansi.StringWidth(line); pad > 0 {
		line += strings.Repeat(" ", pad)
	}
	return line
}

// View renders the table.
func (t *Table) View() string {
	widths := t.widths()
	titles := make([]string, len(t.columns))
	for i, c := range t.columns {
		titles[i] = c.Title
		if i == t.sortCol {
			if t.sortDesc {
				titles[i] += " ▼"
			} else {
				titles[i] += " ▲"
			}
		}
	}

	lines := make([]string, 0, t.height)
	lines = append(lines, tableHeaderStyle.Render(t.fit(renderCells(titles, widths, nil))))

	end := t.offset + t.PageSize()
	if end > len(t.rows) {
		end = len(t.rows)
	}
	for i := t.offset; i < end; i++ {
		var line string
		if i == t.cursor {
			// Cell styles would reset the selection background, so the
			// selected row is rendered plain.
			line = selectedRowStyle.Render(t.fit(renderCells(t.rows[i], widths, nil)))
		} else {
			line = t.fit(renderCells(t.rows[i], widths, t.cellStyle))
		}
		lines = append(lines, line)
	}
	for len(lines) < t.height {
		lines = append(lines, "")
	}
	return strings.Join(lines, "\n")
}
