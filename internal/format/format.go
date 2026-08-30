// Copyright (C) 2026 Suleyman Poyraz
// SPDX-License-Identifier: GPL-3.0-or-later

// Package format contains small helpers for presenting Docker data.
package format

import (
	"fmt"
	"strings"
	"time"
)

// Size formats a byte count using decimal units, the same way the docker CLI
// does (e.g. "12.3MB").
func Size(n int64) string {
	const unit = 1000
	if n < unit {
		return fmt.Sprintf("%dB", n)
	}
	units := []string{"kB", "MB", "GB", "TB", "PB", "EB"}
	v := float64(n)
	i := -1
	for v >= unit && i < len(units)-1 {
		v /= unit
		i++
	}
	if v < 10 {
		return fmt.Sprintf("%.2f%s", v, units[i])
	}
	if v < 100 {
		return fmt.Sprintf("%.1f%s", v, units[i])
	}
	return fmt.Sprintf("%.0f%s", v, units[i])
}

// Ago formats the time elapsed between t and now in a human friendly way,
// e.g. "5 minutes ago".
func Ago(t, now time.Time) string {
	d := now.Sub(t)
	if d < 0 {
		d = 0
	}
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return plural(int(d/time.Minute), "minute") + " ago"
	case d < 48*time.Hour:
		return plural(int(d/time.Hour), "hour") + " ago"
	case d < 14*24*time.Hour:
		return plural(int(d/(24*time.Hour)), "day") + " ago"
	case d < 60*24*time.Hour:
		return plural(int(d/(7*24*time.Hour)), "week") + " ago"
	case d < 365*24*time.Hour:
		return plural(int(d/(30*24*time.Hour)), "month") + " ago"
	default:
		return plural(int(d/(365*24*time.Hour)), "year") + " ago"
	}
}

func plural(n int, word string) string {
	if n == 1 {
		if word == "hour" {
			return "an hour"
		}
		return "a " + word
	}
	return fmt.Sprintf("%d %ss", n, word)
}

// ShortID returns the 12 character short form of a Docker object ID,
// stripping any "sha256:" prefix.
func ShortID(id string) string {
	id = strings.TrimPrefix(id, "sha256:")
	if len(id) > 12 {
		return id[:12]
	}
	return id
}
