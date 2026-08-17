// Copyright (C) 2026 Suleyman Poyraz
// SPDX-License-Identifier: GPL-3.0-or-later

package ui

type tabKind int

const (
	tabContainers tabKind = iota
	tabImages
)

// item is a single row of a resource tab.
type item struct {
	id    string
	name  string
	cells []string
	data  any
}

// listMsg delivers a freshly loaded list of items for a tab.
type listMsg struct {
	kind  tabKind
	items []item
	err   error
}

// tab holds the state of one resource list (containers, images, ...).
type tab struct {
	kind    tabKind
	title   string
	noun    string
	table   *Table
	items   []item
	loaded  bool
	loadErr error
}

func newTab(kind tabKind, title, noun string, columns ...Column) *tab {
	return &tab{kind: kind, title: title, noun: noun, table: NewTable(columns...)}
}

// setItems replaces the items of the tab, trying to keep the cursor on the
// previously selected object.
func (t *tab) setItems(items []item) {
	prev, hadPrev := t.selected()
	t.items = items
	t.loaded = true
	rows := make([][]string, len(items))
	for i, it := range items {
		rows[i] = it.cells
	}
	t.table.SetRows(rows)
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
