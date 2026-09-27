// Copyright (C) 2026 Suleyman Poyraz
// SPDX-License-Identifier: GPL-3.0-or-later

package ui

import (
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestHighlightJSONPreservesText(t *testing.T) {
	src := `{
  "Id": "abc\"def",
  "Count": -12.5e3,
  "Running": true,
  "Paused": false,
  "Labels": null,
  "Args": ["a", "b"]
}`
	if got := ansi.Strip(highlightJSON(src)); got != src {
		t.Errorf("highlighted text differs:\n%s\nwant:\n%s", got, src)
	}
}

func TestScanString(t *testing.T) {
	tests := []struct {
		src  string
		want int
	}{
		{`"abc" x`, 5},
		{`"a\"b"`, 6},
		{`"unterminated`, 13},
		{"\"line\nnext", 5},
	}
	for _, tt := range tests {
		if got := scanString(tt.src, 0); got != tt.want {
			t.Errorf("scanString(%q) = %d, want %d", tt.src, got, tt.want)
		}
	}
}
