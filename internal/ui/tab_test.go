// Copyright (C) 2026 Suleyman Poyraz
// SPDX-License-Identifier: GPL-3.0-or-later

package ui

import "testing"

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

func TestCountNoun(t *testing.T) {
	tests := map[int]string{0: "0 images", 1: "1 image", 2: "2 images"}
	for n, want := range tests {
		if got := countNoun(n, "images"); got != want {
			t.Errorf("countNoun(%d) = %q, want %q", n, got, want)
		}
	}
}
