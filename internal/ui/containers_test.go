// Copyright (C) 2026 Suleyman Poyraz
// SPDX-License-Identifier: GPL-3.0-or-later

package ui

import (
	"testing"

	"github.com/zaryob/docker-tui/internal/docker"
)

func TestFormatPorts(t *testing.T) {
	ports := []docker.Port{
		{IP: "::", PrivatePort: 80, PublicPort: 8080, Type: "tcp"},
		{PrivatePort: 443, Type: "tcp"},
		{IP: "0.0.0.0", PrivatePort: 80, PublicPort: 8080, Type: "tcp"},
		{PrivatePort: 53, Type: "udp"},
	}
	want := "53/udp, 8080->80/tcp, 443/tcp"
	if got := formatPorts(ports); got != want {
		t.Errorf("formatPorts = %q, want %q", got, want)
	}
	if got := formatPorts(nil); got != "" {
		t.Errorf("formatPorts(nil) = %q, want empty", got)
	}
}
