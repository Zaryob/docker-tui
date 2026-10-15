// Copyright (C) 2026 Suleyman Poyraz
// SPDX-License-Identifier: GPL-3.0-or-later

package ui

import (
	"sort"
	"strings"
)

type tabKind int

const (
	tabContainers tabKind = iota
	tabImages
	tabVolumes
	tabNetworks
)

// item is a single row of a resource tab.
type item struct {
	id    string
	name  string
	cells []string
	// sortKeys optionally overrides cells as the values compared when
	// sorting, e.g. to sort sizes numerically.
	sortKeys []string
	data     any
}

func (it item) sortKey(col int) string {
	if col < len(it.sortKeys) && it.sortKeys[col] != "" {
		return it.sortKeys[col]
	}
	if col < len(it.cells) {
		return strings.ToLower(it.cells[col])
	}
	return ""
}

// listMsg delivers a freshly loaded list of items for a tab.
type listMsg struct {
	kind  tabKind
	items []item
	err   error
}

// tab holds the state of one resource list (containers, images, ...).
type tab struct {
	kind   tabKind
	title  string
	noun   string
	table  *Table
	all    []item // every item as loaded
	items  []item // items matching the filter, as shown in the table
	filter string
	// sortCol is the column used for sorting, or -1 for the daemon's order.
	sortCol  int
	sortDesc bool
	loaded   bool
	loadErr  error
	// marked holds the IDs of the objects marked for a bulk action.
	marked map[string]bool
}

func newTab(kind tabKind, title, noun string, columns ...Column) *tab {
	return &tab{kind: kind, title: title, noun: noun, table: NewTable(columns...), sortCol: -1, marked: map[string]bool{}}
}

// setItems replaces the items of the tab.
func (t *tab) setItems(items []item) {
	t.all = items
	t.loaded = true
	// Forget marks of objects that are gone.
	present := make(map[string]bool, len(items))
	for _, it := range items {
		present[it.id] = true
	}
	for id := range t.marked {
		if !present[id] {
			delete(t.marked, id)
		}
	}
	t.refresh()
}

// toggleMark marks or unmarks the selected object.
func (t *tab) toggleMark() {
	it, ok := t.selected()
	if !ok {
		return
	}
	if t.marked[it.id] {
		delete(t.marked, it.id)
	} else {
		t.marked[it.id] = true
	}
	t.refresh()
}

// markAll marks every visible object, or unmarks them if all of them are
// already marked.
func (t *tab) markAll() {
	all := len(t.items) > 0
	for _, it := range t.items {
		all = all && t.marked[it.id]
	}
	for _, it := range t.items {
		if all {
			delete(t.marked, it.id)
		} else {
			t.marked[it.id] = true
		}
	}
	t.refresh()
}

// markWhere marks every object, visible or not, for which match is true and
// returns how many there are.
func (t *tab) markWhere(match func(item) bool) int {
	n := 0
	for _, it := range t.all {
		if match(it) {
			t.marked[it.id] = true
			n++
		}
	}
	t.refresh()
	return n
}

// clearMarks unmarks every object.
func (t *tab) clearMarks() {
	clear(t.marked)
	t.refresh()
}

// markedItems returns the marked objects in list order, including the ones
// hidden by the filter.
func (t *tab) markedItems() []item {
	var out []item
	for _, it := range t.all {
		if t.marked[it.id] {
			out = append(out, it)
		}
	}
	return out
}

// setFilter changes the filter and updates the visible rows.
func (t *tab) setFilter(filter string) {
	t.filter = filter
	t.refresh()
}

// cycleSort sorts by the next column, toggling the direction once every
// column has been used ascending, and finally returns to the default order.
func (t *tab) cycleSort() {
	n := len(t.table.columns)
	switch {
	case t.sortCol < 0:
		t.sortCol, t.sortDesc = 0, false
	case t.sortCol < n-1:
		t.sortCol++
	case !t.sortDesc:
		t.sortCol, t.sortDesc = 0, true
	default:
		t.sortCol, t.sortDesc = -1, false
	}
	t.table.SetSort(t.sortCol, t.sortDesc)
	t.refresh()
}

// reverseSort flips the sort direction of the current sort column.
func (t *tab) reverseSort() {
	if t.sortCol < 0 {
		t.sortCol = 0
	}
	t.sortDesc = !t.sortDesc
	t.table.SetSort(t.sortCol, t.sortDesc)
	t.refresh()
}

// matches reports whether any cell of it contains the filter, ignoring case.
func (it item) matches(filter string) bool {
	if filter == "" {
		return true
	}
	filter = strings.ToLower(filter)
	for _, c := range it.cells {
		if strings.Contains(strings.ToLower(c), filter) {
			return true
		}
	}
	return false
}

// refresh rebuilds the table rows from the items and the filter, trying to
// keep the cursor on the previously selected object.
func (t *tab) refresh() {
	prev, hadPrev := t.selected()
	items := make([]item, 0, len(t.all))
	for _, it := range t.all {
		if it.matches(t.filter) {
			items = append(items, it)
		}
	}
	if t.sortCol >= 0 {
		col, desc := t.sortCol, t.sortDesc
		sort.SliceStable(items, func(i, j int) bool {
			a, b := items[i].sortKey(col), items[j].sortKey(col)
			if desc {
				return a > b
			}
			return a < b
		})
	}
	t.items = items
	rows := make([][]string, len(items))
	marks := make([]bool, len(items))
	for i, it := range items {
		rows[i] = it.cells
		marks[i] = t.marked[it.id]
	}
	t.table.SetRows(rows)
	t.table.SetMarks(marks)
	if hadPrev {
		for i, it := range items {
			if it.id == prev.id {
				t.table.SetCursor(i)
				break
			}
		}
	}
}

// selected returns the item under the cursor.
func (t *tab) selected() (item, bool) {
	i := t.table.Cursor()
	if i < 0 || i >= len(t.items) {
		return item{}, false
	}
	return t.items[i], true
}
