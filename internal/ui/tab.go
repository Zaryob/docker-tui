// Copyright (C) 2026 Suleyman Poyraz
// SPDX-License-Identifier: GPL-3.0-or-later

package ui

import (
	"slices"
	"sort"
	"strings"
)

type tabKind int

const (
	tabContainers tabKind = iota
	tabImages
	tabVolumes
	tabNetworks
	tabBuilds
	tabLogs
	tabProjects
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
	// members is set on group rows and holds the objects of the group.
	members []item
	// usage is the resource usage of a container, when sampled.
	usage containerUsage
}

// isGroup reports whether the item is a group row rather than an object.
func (it item) isGroup() bool { return it.members != nil }

// grouping folds objects sharing a key, e.g. the containers of a compose
// project, under a collapsible group row.
type grouping struct {
	// key returns the group of an object, or "" for none.
	key func(item) string
	// nameCol is the column indented for the members of a group.
	nameCol int
	// memberName returns the name shown for a member of group key.
	memberName func(it item, key string) string
	// row builds the group row for the visible members of group key.
	row func(key string, members []item, collapsed bool) item
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
	// partial allows usable results to replace the list while reporting
	// failures from other sources, e.g. a combined log snapshot.
	partial bool
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
	// highlighted is a temporary range of row IDs, separate from action marks.
	// Identities keep daemon refreshes from highlighting replacement objects.
	highlighted map[string]bool
	rangeAnchor string
	// visible holds the objects matching the filter, in display order and
	// including the members of collapsed groups.
	visible []item
	// group, when set, groups the rows; collapsed holds the keys of the
	// collapsed groups.
	group     *grouping
	collapsed map[string]bool
}

func newTab(kind tabKind, title, noun string, columns ...Column) *tab {
	return &tab{kind: kind, title: title, noun: noun, table: NewTable(columns...), sortCol: -1, marked: map[string]bool{}, collapsed: map[string]bool{}}
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
	var rows []item
	if len(t.highlighted) > 0 {
		for _, it := range t.items {
			if t.highlighted[it.id] {
				rows = append(rows, it)
			}
		}
	} else if it, ok := t.selected(); ok {
		rows = []item{it}
	}
	// A range can contain both a project row and its children. Deduplicate
	// objects so every checkbox is changed exactly once.
	var targets []item
	seen := make(map[string]bool)
	for _, row := range rows {
		members := []item{row}
		if row.isGroup() {
			members = row.members
		}
		for _, it := range members {
			if !seen[it.id] {
				targets = append(targets, it)
				seen[it.id] = true
			}
		}
	}
	if len(targets) == 0 {
		return
	}
	unmark := allMarked(targets, t.marked)
	for _, it := range targets {
		if unmark {
			delete(t.marked, it.id)
		} else {
			t.marked[it.id] = true
		}
	}
	t.refresh()
}

// extendRange moves the cursor while growing or shrinking a temporary range.
func (t *tab) extendRange(delta int) {
	it, ok := t.selected()
	if !ok {
		return
	}
	if t.rangeAnchor == "" {
		t.rangeAnchor = it.id
	}
	anchor := -1
	for i, row := range t.items {
		if row.id == t.rangeAnchor {
			anchor = i
			break
		}
	}
	if anchor < 0 {
		t.clearRange()
		return
	}
	t.table.Move(delta)
	lo, hi := min(anchor, t.table.Cursor()), max(anchor, t.table.Cursor())
	t.highlighted = make(map[string]bool, hi-lo+1)
	for _, row := range t.items[lo : hi+1] {
		t.highlighted[row.id] = true
	}
	t.syncHighlights()
}

func (t *tab) clearRange() {
	t.rangeAnchor = ""
	t.highlighted = nil
	t.table.SetHighlights(nil)
}

func (t *tab) syncHighlights() {
	if len(t.highlighted) == 0 {
		t.table.SetHighlights(nil)
		return
	}
	rows := make([]bool, len(t.items))
	for i, it := range t.items {
		rows[i] = t.highlighted[it.id]
	}
	t.table.SetHighlights(rows)
}

func allMarked(items []item, marked map[string]bool) bool {
	for _, it := range items {
		if !marked[it.id] {
			return false
		}
	}
	return len(items) > 0
}

// toggleCollapse collapses or expands the group under the cursor, or the
// group of the selected object, which then moves the cursor to the group.
func (t *tab) toggleCollapse() {
	t.clearRange()
	if t.group == nil {
		return
	}
	it, ok := t.selected()
	if !ok {
		return
	}
	key := t.group.key(it)
	if it.isGroup() {
		key = strings.TrimPrefix(it.id, groupIDPrefix)
	}
	if key == "" {
		return
	}
	if t.collapsed[key] {
		delete(t.collapsed, key)
	} else {
		t.collapsed[key] = true
	}
	t.refresh()
	for i, row := range t.items {
		if row.id == groupIDPrefix+key {
			t.table.SetCursor(i)
			break
		}
	}
}

// markAll marks every visible object, or unmarks them if all of them are
// already marked.
func (t *tab) markAll() {
	all := allMarked(t.visible, t.marked)
	for _, it := range t.visible {
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
	t.clearRange()
	t.filter = filter
	t.refresh()
}

// cycleSort sorts by the next column, toggling the direction once every
// column has been used ascending, and finally returns to the default order.
func (t *tab) cycleSort() {
	t.clearRange()
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
	t.clearRange()
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
	if strings.Contains(strings.ToLower(it.name), filter) {
		return true
	}
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
	t.visible = items
	if t.group != nil {
		items = t.groupRows(items)
	}
	t.items = items
	rows := make([][]string, len(items))
	marks := make([]bool, len(items))
	for i, it := range items {
		rows[i] = it.cells
		if it.isGroup() {
			marks[i] = allMarked(it.members, t.marked)
		} else {
			marks[i] = t.marked[it.id]
		}
	}
	t.table.SetRows(rows)
	t.table.SetMarks(marks)
	foundPrev := !hadPrev
	if hadPrev {
		for i, it := range items {
			if it.id == prev.id {
				t.table.SetCursor(i)
				foundPrev = true
				break
			}
		}
	}
	if len(t.highlighted) > 0 {
		present := make(map[string]bool, len(items))
		for _, it := range items {
			present[it.id] = true
		}
		if !foundPrev || !present[t.rangeAnchor] {
			t.clearRange()
		} else {
			for id := range t.highlighted {
				if !present[id] {
					delete(t.highlighted, id)
				}
			}
		}
	}
	t.syncHighlights()
}

// groupIDPrefix starts the IDs of group rows, keeping them apart from the
// IDs of objects.
const groupIDPrefix = "group:"

// groupRows inserts a group row before the members of every group, at the
// position of its first member, and hides the members of collapsed groups.
func (t *tab) groupRows(items []item) []item {
	g := t.group
	members := map[string][]item{}
	for _, it := range items {
		if k := g.key(it); k != "" {
			members[k] = append(members[k], it)
		}
	}
	out := make([]item, 0, len(items)+len(members))
	for _, it := range items {
		k := g.key(it)
		if k == "" {
			out = append(out, it)
			continue
		}
		ms, first := members[k]
		if !first {
			continue // already emitted with its group
		}
		delete(members, k)
		row := g.row(k, ms, t.collapsed[k])
		row.id = groupIDPrefix + k
		out = append(out, row)
		if t.collapsed[k] {
			continue
		}
		for _, m := range ms {
			cells := slices.Clone(m.cells)
			cells[g.nameCol] = "  " + g.memberName(m, k)
			m.cells = cells
			out = append(out, m)
		}
	}
	return out
}

// selected returns the item under the cursor.
func (t *tab) selected() (item, bool) {
	i := t.table.Cursor()
	if i < 0 || i >= len(t.items) {
		return item{}, false
	}
	return t.items[i], true
}
