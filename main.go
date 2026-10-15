// Copyright (C) 2026 Suleyman Poyraz
// SPDX-License-Identifier: GPL-3.0-or-later

// Command docker-tui is a terminal user interface for Docker.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/zaryob/docker-tui/internal/docker"
	"github.com/zaryob/docker-tui/internal/ui"
)

// version is set at build time with -ldflags "-X main.version=...".
var version = "dev"

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "docker-tui:", err)
		os.Exit(1)
	}
}

func run() error {
	var (
		host        string
		contextName string
		refresh     time.Duration
		showVersion bool
	)
	flag.StringVar(&host, "host", "", "Docker daemon address (default $DOCKER_HOST or "+docker.DefaultHost+")")
	flag.StringVar(&host, "H", "", "shorthand for -host")
	flag.StringVar(&contextName, "context", "", "docker CLI context to use (default $DOCKER_CONTEXT or the current context)")
	flag.DurationVar(&refresh, "refresh", 3*time.Second, "interval for refreshing lists, 0 disables")
	flag.BoolVar(&showVersion, "version", false, "print version and exit")
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "Usage: docker-tui [flags]\n\nA terminal user interface for Docker.\n\nFlags:\n")
		flag.PrintDefaults()
	}
	flag.Parse()

	if showVersion {
		fmt.Println("docker-tui", version)
		return nil
	}
	if flag.NArg() > 0 {
		flag.Usage()
		return fmt.Errorf("unexpected arguments: %v", flag.Args())
	}

	if contextName != "" {
		if host != "" {
			return errors.New("-host and -context cannot be used together")
		}
		var err error
		if host, err = docker.ContextHost(contextName); err != nil {
			return err
		}
	}

	client, err := docker.NewClient(host)
	if err != nil {
		return err
	}
	if err := client.Ping(context.Background()); err != nil {
		return fmt.Errorf("cannot reach docker daemon at %s: %w", client.Host(), err)
	}

	// Query the terminal background color before the program starts reading
	// input; doing it lazily during the first render can swallow keystrokes.
	lipgloss.SetHasDarkBackground(lipgloss.HasDarkBackground())

	app := ui.New(client, ui.Options{RefreshInterval: refresh})
	p := tea.NewProgram(app, tea.WithAltScreen())
	_, err = p.Run()
	return err
}
