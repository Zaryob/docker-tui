// Copyright (C) 2026 Suleyman Poyraz
// SPDX-License-Identifier: GPL-3.0-or-later

package ui

import (
	"net/http"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func rangeTestApp(t *testing.T) (*App, *tab) {
	t.Helper()
	a := newTestApp(t, http.NotFoundHandler())
	list := newTab(tabContainers, "Containers", "containers", Column{Title: "ID"}, Column{Title: "NAME"})
	list.table.SetSize(50, 3)
	list.setItems(testItems())
	a.tabs[tabContainers] = list
	return a, list
}

func TestSpaceKeepsCursorAndScrollPosition(t *testing.T) {
	a, list := rangeTestApp(t)
	list.table.SetSize(50, 2)
	list.table.SetCursor(1)
	beforeCursor, beforeOffset := list.table.Cursor(), list.table.offset
	a.handleKey(keyRunes(" "))
	if got := ids(list.markedItems()); got != "2" {
		t.Fatal("marked wrong object:", got)
	}
	a.handleKey(keyRunes(" "))
	if len(list.marked) != 0 {
		t.Fatal("space did not unmark current object")
	}
	if list.table.Cursor() != beforeCursor || list.table.offset != beforeOffset {
		t.Fatal("space advanced the cursor or scrolled the table")
	}
}

func TestShiftArrowsHighlightBeforeMarkingAndShrinkRange(t *testing.T) {
	a, list := rangeTestApp(t)
	list.table.SetCursor(1)
	a.handleKey(tea.KeyMsg{Type: tea.KeyShiftDown})
	if len(list.highlighted) != 2 || !list.highlighted["2"] || !list.highlighted["3"] {
		t.Fatal("range did not include anchor and cursor")
	}
	if len(list.marked) != 0 || ids(a.targets()) != "3" {
		t.Fatal("highlight alone became a bulk-action target")
	}
	if !list.table.highlighted(1) || !list.table.highlighted(2) {
		t.Fatal("range not sent to renderer")
	}
	a.handleKey(tea.KeyMsg{Type: tea.KeyShiftUp})
	if len(list.highlighted) != 1 || !list.highlighted["2"] {
		t.Fatal("reversing direction did not shrink range")
	}
	a.handleKey(tea.KeyMsg{Type: tea.KeyShiftUp})
	if len(list.highlighted) != 2 || !list.highlighted["1"] || !list.highlighted["2"] {
		t.Fatal("range did not extend above anchor")
	}
	a.handleKey(keyRunes(" "))
	if got := ids(a.targets()); got != "1,2" {
		t.Fatal("range not marked:", got)
	}
	if list.table.Cursor() != 0 {
		t.Fatal("space moved range endpoint")
	}
	if !strings.Contains(a.statusLine(), "2 highlighted") {
		t.Fatal("missing range status")
	}
	a.handleKey(keyRunes(" "))
	if len(list.marked) != 0 || len(list.highlighted) != 2 {
		t.Fatal("space should toggle marks while keeping the range")
	}
}

func TestRangeRefreshKeepsIdentitiesWithoutSelectingInsertedRows(t *testing.T) {
	a, list := rangeTestApp(t)
	a.handleKey(tea.KeyMsg{Type: tea.KeyShiftDown})
	items := testItems()
	inserted := item{id: "new", name: "new", cells: []string{"new", "new"}}
	list.setItems([]item{items[0], inserted, items[2], items[1]})
	if !list.highlighted["1"] || !list.highlighted["2"] || list.highlighted["new"] || list.highlighted["3"] {
		t.Fatal("refresh moved highlight to different objects")
	}
	if selected, _ := list.selected(); selected.id != "2" {
		t.Fatal("refresh moved the endpoint")
	}
	a.handleKey(keyRunes(" "))
	if got := ids(list.markedItems()); got != "1,2" {
		t.Fatal("inserted row became a target:", got)
	}
	// The endpoint vanishes and another row occupies its position. Space must
	// act on the current row, not an invisible or stale range.
	list.setItems([]item{items[0], inserted, items[2]})
	if len(list.highlighted) != 0 || list.rangeAnchor != "" {
		t.Fatal("range survived endpoint removal")
	}
	a.handleKey(keyRunes(" "))
	if !list.marked["3"] {
		t.Fatal("space acted on stale range after endpoint disappeared")
	}
}

func TestRangeClearsWhenAnchorDisappears(t *testing.T) {
	a, list := rangeTestApp(t)
	a.handleKey(tea.KeyMsg{Type: tea.KeyShiftDown})
	list.setItems(testItems()[1:])
	if len(list.highlighted) != 0 || list.rangeAnchor != "" {
		t.Fatal("range survived anchor removal")
	}
	if selected, _ := list.selected(); selected.id != "2" {
		t.Fatal("remaining endpoint was not preserved")
	}
}

func TestRangeNormalNavigationAndEscapePreserveMarks(t *testing.T) {
	for _, key := range []tea.KeyMsg{{Type: tea.KeyDown}, {Type: tea.KeyUp}, {Type: tea.KeyPgDown}, {Type: tea.KeyPgUp}, keyRunes("g"), keyRunes("G"), {Type: tea.KeyEsc}} {
		t.Run(key.String(), func(t *testing.T) {
			a, list := rangeTestApp(t)
			a.handleKey(tea.KeyMsg{Type: tea.KeyShiftDown})
			a.handleKey(keyRunes(" "))
			a.handleKey(key)
			if len(list.highlighted) != 0 || list.rangeAnchor != "" {
				t.Fatal("navigation left a stale range")
			}
			if got := ids(list.markedItems()); got != "1,2" {
				t.Fatal("navigation changed confirmed marks:", got)
			}
			if list.table.highlighted(0) || list.table.highlighted(1) {
				t.Fatal("highlight remained in renderer")
			}
		})
	}
}

func TestRangeEscapePrecedesFilterAndMarks(t *testing.T) {
	a, list := rangeTestApp(t)
	items := testItems()
	items[0].name, items[2].name = "selected-web", "selected-cache"
	list.setItems(items)
	list.setFilter("selected-")
	a.handleKey(tea.KeyMsg{Type: tea.KeyShiftDown})
	a.handleKey(keyRunes(" "))
	a.handleKey(tea.KeyMsg{Type: tea.KeyEsc})
	if list.filter != "selected-" || ids(list.markedItems()) != "1,3" {
		t.Fatal("escape cleared filter or marks instead of just the range")
	}
}

func TestRangeClearsOnListTransformsAndTabSwitch(t *testing.T) {
	for _, key := range []tea.KeyMsg{keyRunes("o"), keyRunes("O"), keyRunes("/"), {Type: tea.KeyTab}} {
		t.Run(key.String(), func(t *testing.T) {
			a, list := rangeTestApp(t)
			a.handleKey(tea.KeyMsg{Type: tea.KeyShiftDown})
			a.handleKey(key)
			if len(list.highlighted) != 0 {
				t.Fatal("view change left a stale range")
			}
		})
	}
}

func TestRangeProjectRowsDeduplicateMembers(t *testing.T) {
	a := newTestApp(t, http.NotFoundHandler())
	list := a.current()
	list.setItems(containerItems(containersFixture()))
	selectID(t, list, groupIDPrefix+"shop")
	// Include both the project summary and one of its children.
	a.handleKey(tea.KeyMsg{Type: tea.KeyShiftDown})
	a.handleKey(keyRunes(" "))
	if got := ids(list.markedItems()); got != "a1,b2" {
		t.Fatal("range toggled a project member twice:", got)
	}
	// A mixed range should become uniformly marked; a second Space unmarks it.
	delete(list.marked, "a1")
	a.handleKey(keyRunes(" "))
	if got := ids(list.markedItems()); got != "a1,b2" {
		t.Fatal("mixed range did not become fully marked:", got)
	}
	a.handleKey(keyRunes(" "))
	if len(list.marked) != 0 {
		t.Fatal("fully marked project range did not unmark")
	}
	a.handleKey(keyRunes("c"))
	if len(list.highlighted) != 0 {
		t.Fatal("folding retained hidden row highlights")
	}
	// A folded project contributes its members to a range with a standalone row.
	a.handleKey(tea.KeyMsg{Type: tea.KeyShiftDown})
	a.handleKey(keyRunes(" "))
	if got := ids(list.markedItems()); got != "a1,b2,c3" {
		t.Fatal("folded project range lost its members:", got)
	}
}

func TestRangeEmptyAndBoundaryNavigation(t *testing.T) {
	a, list := rangeTestApp(t)
	list.setItems(nil)
	a.handleKey(tea.KeyMsg{Type: tea.KeyShiftUp})
	a.handleKey(tea.KeyMsg{Type: tea.KeyShiftDown})
	a.handleKey(keyRunes(" "))
	if len(list.highlighted) != 0 || len(list.marked) != 0 {
		t.Fatal("empty list acquired a selection")
	}
	list.setItems(testItems()[:1])
	a.handleKey(tea.KeyMsg{Type: tea.KeyShiftUp})
	a.handleKey(tea.KeyMsg{Type: tea.KeyShiftDown})
	a.handleKey(keyRunes(" "))
	if list.table.Cursor() != 0 || ids(list.markedItems()) != "1" {
		t.Fatal("single-row range overflowed")
	}
}
