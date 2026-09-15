// Copyright (C) 2026 Suleyman Poyraz
// SPDX-License-Identifier: GPL-3.0-or-later

package ui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func makeRows(n int) [][]string {
	rows := make([][]string, n)
	for i := range rows {
		rows[i] = []string{fmt.Sprintf("id%d", i), fmt.Sprintf("name-%d", i)}
	}
	return rows
}

func TestTableCursorClamp(t *testing.T) {
	tb := NewTable(Column{Title: "ID", Width: 6}, Column{Title: "NAME"})
	tb.SetSize(40, 6)
	tb.SetRows(makeRows(3))

	tb.Move(-5)
	if tb.Cursor() != 0 {
		t.Errorf("cursor = %d, want 0", tb.Cursor())
	}
	tb.Move(10)
	if tb.Cursor() != 2 {
		t.Errorf("cursor = %d, want 2", tb.Cursor())
	}
	tb.SetRows(makeRows(1))
	if tb.Cursor() != 0 {
		t.Errorf("cursor after shrinking = %d, want 0", tb.Cursor())
	}
	tb.SetRows(nil)
	if tb.Cursor() != 0 {
		t.Errorf("cursor on empty table = %d, want 0", tb.Cursor())
	}
}

func TestTableScrolling(t *testing.T) {
	tb := NewTable(Column{Title: "ID", Width: 6}, Column{Title: "NAME"})
	tb.SetSize(40, 5) // header + 4 rows
	tb.SetRows(makeRows(20))

	tb.Move(6)
	view := ansi.Strip(tb.View())
	lines := strings.Split(view, "\n")
	if len(lines) != 5 {
		t.Fatalf("view has %d lines, want 5", len(lines))
	}
	if !strings.HasPrefix(lines[4], "id6") {
		t.Errorf("last visible row = %q, want id6", lines[4])
	}
	tb.Bottom()
	lines = strings.Split(ansi.Strip(tb.View()), "\n")
	if !strings.HasPrefix(lines[4], "id19") {
		t.Errorf("last visible row = %q, want id19", lines[4])
	}
	for i, l := range lines {
		if w := ansi.StringWidth(l); w != 40 {
			t.Errorf("line %d has width %d, want 40", i, w)
		}
	}
}

func TestTableTruncatesCells(t *testing.T) {
	tb := NewTable(Column{Title: "ID", Width: 4}, Column{Title: "NAME", Width: 6})
	tb.SetSize(12, 2)
	tb.SetRows([][]string{{"abcdefgh", "verylongname"}})
	lines := strings.Split(ansi.Strip(tb.View()), "\n")
	if lines[1] != "abc…  veryl…" {
		t.Errorf("row = %q", lines[1])
	}
}

func TestEmptyTableView(t *testing.T) {
	tb := NewTable(Column{Title: "ID"})
	tb.SetSize(10, 3)
	if got := len(strings.Split(tb.View(), "\n")); got != 3 {
		t.Errorf("empty table renders %d lines, want 3", got)
	}
}
