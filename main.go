// Copyright (C) 2026 Suleyman Poyraz
// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"context"
	"fmt"
	"os"

	"github.com/zaryob/docker-tui/internal/docker"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "docker-tui:", err)
		os.Exit(1)
	}
}

func run() error {
	client := docker.NewClient("")
	ctx := context.Background()
	if err := client.Ping(ctx); err != nil {
		return fmt.Errorf("cannot reach docker daemon: %w", err)
	}
	v, err := client.ServerVersion(ctx)
	if err != nil {
		return err
	}
	fmt.Printf("connected to docker %s (api %s, %s/%s)\n", v.Version, v.APIVersion, v.Os, v.Arch)
	return nil
}
