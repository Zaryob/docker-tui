// Copyright (C) 2026 Suleyman Poyraz
// SPDX-License-Identifier: GPL-3.0-or-later

package buildx

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Project describes a Compose project and its local source files.
type Project struct {
	// LocalSource keeps an explicitly opened local file ahead of daemon labels.
	LocalSource bool   `json:"-"`
	Name        string `json:"Name"`
	Status      string `json:"Status"`
	ConfigFiles string `json:"ConfigFiles"`
	WorkingDir  string `json:"WorkingDir,omitempty"`
}

var projectName = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`)

// ComposeProjects queries the selected daemon, including stopped projects.
func (c *CLI) ComposeProjects(ctx context.Context) ([]Project, error) {
	out, err := c.output(ctx, "compose", "ls", "--all", "--format", "json")
	if err != nil {
		return nil, err
	}
	var projects []Project
	if err := json.Unmarshal(out, &projects); err != nil {
		return nil, fmt.Errorf("compose projects: %w", err)
	}
	return projects, nil
}

// Args validates that daemon-reported paths are available on this machine.
// Never silently run Compose in the application's working directory.
func (p Project) Args() ([]string, error) {
	if !projectName.MatchString(p.Name) {
		return nil, fmt.Errorf("invalid Compose project name")
	}
	args := []string{"compose", "--ansi", "never", "--progress", "plain", "--project-name", p.Name}
	if p.ConfigFiles == "" {
		return nil, fmt.Errorf("project %s has no local Compose files; use f to open a local file", p.Name)
	}
	for _, file := range strings.Split(p.ConfigFiles, ",") {
		file = strings.TrimSpace(file)
		if !filepath.IsAbs(file) {
			return nil, fmt.Errorf("Compose file must be an absolute local path; use f to open a local file")
		}
		info, err := os.Stat(file)
		if err != nil || !info.Mode().IsRegular() {
			return nil, fmt.Errorf("Compose files for %s are unavailable locally; use f to open a local file", p.Name)
		}
		args = append(args, "--file", file)
	}
	if p.WorkingDir != "" {
		if !filepath.IsAbs(p.WorkingDir) {
			return nil, fmt.Errorf("Compose working directory must be absolute")
		}
		info, err := os.Stat(p.WorkingDir)
		if err != nil || !info.IsDir() {
			return nil, fmt.Errorf("Compose working directory is unavailable locally")
		}
		args = append(args, "--project-directory", p.WorkingDir)
	}
	return args, nil
}

// OpenCompose validates a local file through Compose and resolves its project name.
func (c *CLI) OpenCompose(ctx context.Context, file string) (Project, error) {
	file, err := filepath.Abs(file)
	if err != nil {
		return Project{}, err
	}
	info, err := os.Stat(file)
	if err != nil || !info.Mode().IsRegular() {
		return Project{}, fmt.Errorf("select an existing local Compose file")
	}
	out, err := c.output(ctx, "compose", "--file", file, "config", "--format", "json")
	if err != nil {
		return Project{}, err
	}
	var config struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(out, &config); err != nil {
		return Project{}, fmt.Errorf("read Compose project name: %w", err)
	}
	p := Project{LocalSource: true, Name: config.Name, ConfigFiles: file, WorkingDir: filepath.Dir(file), Status: "not running"}
	_, err = p.Args()
	return p, err
}

// Compose runs one project operation with streamed output and cancellation.
func (c *CLI) Compose(ctx context.Context, p Project, action string) (io.ReadCloser, error) {
	args, err := p.Args()
	if err != nil {
		return nil, err
	}
	switch action {
	case "up":
		args = append(args, "up", "--detach")
	case "rebuild":
		args = append(args, "up", "--detach", "--build")
	case "down":
		args = append(args, "down")
	default:
		return nil, fmt.Errorf("unknown Compose operation %q", action)
	}
	return c.stream(ctx, args...)
}
