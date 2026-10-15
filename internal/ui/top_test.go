// Copyright (C) 2026 Suleyman Poyraz
// SPDX-License-Identifier: GPL-3.0-or-later

package ui

import "testing"

func TestTopColumns(t *testing.T) {
	titles := []string{"UID", "PID", "PPID", "CMD"}
	rows := [][]string{
		{"root", "1", "0", "nginx: master process"},
		{"nginx", "29", "1", "nginx: worker process"},
	}
	cols := topColumns(titles, rows)
	want := []Column{
		{Title: "UID", Width: 5},
		{Title: "PID", Width: 3},
		{Title: "PPID", Width: 4},
		{Title: "CMD"},
	}
	if len(cols) != len(want) {
		t.Fatalf("got %d columns, want %d", len(cols), len(want))
	}
	for i := range want {
		if cols[i] != want[i] {
			t.Errorf("column %d = %+v, want %+v", i, cols[i], want[i])
		}
	}
}

func TestProcessCount(t *testing.T) {
	for n, want := range map[int]string{0: "0 processes", 1: "1 process", 7: "7 processes"} {
		if got := processCount(n); got != want {
			t.Errorf("processCount(%d) = %q, want %q", n, got, want)
		}
	}
}
