// Copyright (C) 2026 Suleyman Poyraz
// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"fmt"
	"os"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "docker-tui:", err)
		os.Exit(1)
	}
}

func run() error {
	fmt.Println("docker-tui")
	return nil
}
