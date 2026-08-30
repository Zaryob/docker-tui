// Copyright (C) 2026 Suleyman Poyraz
// SPDX-License-Identifier: GPL-3.0-or-later

package format

import (
	"testing"
	"time"
)

func TestSize(t *testing.T) {
	tests := []struct {
		in   int64
		want string
	}{
		{0, "0B"},
		{999, "999B"},
		{1000, "1.00kB"},
		{1234567, "1.23MB"},
		{72_800_000, "72.8MB"},
		{512_000_000, "512MB"},
		{3_100_000_000, "3.10GB"},
	}
	for _, tt := range tests {
		if got := Size(tt.in); got != tt.want {
			t.Errorf("Size(%d) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestAgo(t *testing.T) {
	now := time.Date(2026, 8, 16, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		d    time.Duration
		want string
	}{
		{10 * time.Second, "just now"},
		{time.Minute, "a minute ago"},
		{5 * time.Minute, "5 minutes ago"},
		{time.Hour, "an hour ago"},
		{30 * time.Hour, "30 hours ago"},
		{3 * 24 * time.Hour, "3 days ago"},
		{21 * 24 * time.Hour, "3 weeks ago"},
		{90 * 24 * time.Hour, "3 months ago"},
		{800 * 24 * time.Hour, "2 years ago"},
		{-time.Hour, "just now"},
	}
	for _, tt := range tests {
		if got := Ago(now.Add(-tt.d), now); got != tt.want {
			t.Errorf("Ago(-%v) = %q, want %q", tt.d, got, tt.want)
		}
	}
}

func TestShortID(t *testing.T) {
	tests := map[string]string{
		"sha256:4f9e1c0b7a2d6e8f00112233": "4f9e1c0b7a2d",
		"abc":                             "abc",
		"0123456789abcdef":                "0123456789ab",
	}
	for in, want := range tests {
		if got := ShortID(in); got != want {
			t.Errorf("ShortID(%q) = %q, want %q", in, got, want)
		}
	}
}
