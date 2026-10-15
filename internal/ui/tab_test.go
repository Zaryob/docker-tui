// Copyright (C) 2026 Suleyman Poyraz
// SPDX-License-Identifier: GPL-3.0-or-later

package ui

import (
	"strings"
	"testing"
)

func testItems() []item {
	return []item{
		{id: "1", name: "web", cells: []string{"1", "web", "nginx:latest"}},
		{id: "2", name: "db", cells: []string{"2", "db", "postgres:16"}},
		{id: "3", name: "cache", cells: []string{"3", "cache", "redis"}},
	}
}

func TestTabFilter(t *testing.T) {
	tb := newTab(tabContainers, "Containers", "containers", Column{Title: "ID"}, Column{Title: "NAME"})
	tb.table.SetSize(40, 10)
	tb.setItems(testItems())

	tb.setFilter("POST")
	if len(tb.items) != 1 || tb.items[0].id != "2" {
		t.Fatalf("filter POST matched %+v", tb.items)
	}
	tb.setFilter("")
	if len(tb.items) != 3 {
		t.Fatalf("empty filter shows %d items, want 3", len(tb.items))
	}
}

func TestTabKeepsSelectionAcrossReloads(t *testing.T) {
	tb := newTab(tabContainers, "Containers", "containers", Column{Title: "ID"})
	tb.table.SetSize(40, 10)
	tb.setItems(testItems())
	tb.table.SetCursor(2) // cache

	items := testItems()
	items[0], items[2] = items[2], items[0] // cache moves to the top
	tb.setItems(items)
	if it, _ := tb.selected(); it.id != "3" {
		t.Errorf("selected %q after reload, want 3", it.id)
	}
}

func TestTabMarks(t *testing.T) {
	tb := newTab(tabContainers, "Containers", "containers", Column{Title: "ID"}, Column{Title: "NAME"})
	tb.table.SetSize(40, 10)
	tb.setItems(testItems())

	tb.toggleMark() // web
	tb.table.SetCursor(2)
	tb.toggleMark() // cache
	if got := ids(tb.markedItems()); got != "1,3" {
		t.Fatalf("marked = %s, want 1,3", got)
	}
	tb.toggleMark()
	if got := ids(tb.markedItems()); got != "1" {
		t.Fatalf("after unmarking cache: %s, want 1", got)
	}

	// Marks survive filtering and are dropped when the object disappears.
	tb.setFilter("redis")
	if got := ids(tb.markedItems()); got != "1" {
		t.Errorf("marks after filtering = %s, want 1", got)
	}
	tb.markAll()
	if got := ids(tb.markedItems()); got != "1,3" {
		t.Errorf("mark all visible = %s, want 1,3", got)
	}
	tb.markAll()
	if got := ids(tb.markedItems()); got != "1" {
		t.Errorf("mark all again unmarks visible = %s, want 1", got)
	}
	tb.setFilter("")
	tb.setItems(testItems()[1:])
	if got := ids(tb.markedItems()); got != "" {
		t.Errorf("marks of removed objects kept: %s", got)
	}

	if n := tb.markWhere(func(it item) bool { return it.name != "db" }); n != 1 {
		t.Errorf("markWhere marked %d, want 1", n)
	}
	tb.clearMarks()
	if len(tb.markedItems()) != 0 {
		t.Error("clearMarks left marks")
	}
}

func ids(items []item) string {
	out := make([]string, len(items))
	for i, it := range items {
		out[i] = it.id
	}
	return strings.Join(out, ",")
}

func TestCountNoun(t *testing.T) {
	tests := map[int]string{0: "0 images", 1: "1 image", 2: "2 images"}
	for n, want := range tests {
		if got := countNoun(n, "images"); got != want {
			t.Errorf("countNoun(%d) = %q, want %q", n, got, want)
		}
	}
}
