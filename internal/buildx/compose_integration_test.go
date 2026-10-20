// Copyright (C) 2026 Suleyman Poyraz
// SPDX-License-Identifier: GPL-3.0-or-later

//go:build integration

package buildx

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/zaryob/docker-tui/internal/docker"
)

func TestRealComposeLifecycle(t *testing.T) {
	client, err := docker.NewClient("")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	cli := &CLI{Host: client.Host(), Context: client.Context()}
	if _, err := cli.output(ctx, "info", "--format", "{{.ServerVersion}}"); err != nil {
		if os.Getenv("DOCKER_TUI_INTEGRATION_REQUIRED") == "1" {
			t.Fatal(err)
		}
		t.Skipf("Docker unavailable: %v", err)
	}
	if _, err := cli.output(ctx, "compose", "version"); err != nil {
		if os.Getenv("DOCKER_TUI_INTEGRATION_REQUIRED") == "1" {
			t.Fatal(err)
		}
		t.Skipf("Compose unavailable: %v", err)
	}
	name := fmt.Sprintf("dtui-compose-%d", time.Now().UnixNano())
	dir := t.TempDir()
	file := filepath.Join(dir, "compose.yaml")
	yaml := fmt.Sprintf("name: %s\nservices:\n  smoke:\n    image: %s:smoke\n    build: .\n", name, name)
	if err := os.WriteFile(file, []byte(yaml), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "Dockerfile"), []byte("FROM alpine:3.21\nCMD [\"sleep\",\"300\"]\n"), 0600); err != nil {
		t.Fatal(err)
	}
	p, err := cli.OpenCompose(ctx, file)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanCtx, cleanCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cleanCancel()
		rc, err := cli.Compose(cleanCtx, p, "down")
		if err != nil {
			t.Errorf("cleanup down: %v", err)
		} else {
			_, err = io.Copy(io.Discard, rc)
			rc.Close()
			if err != nil {
				t.Errorf("cleanup down output: %v", err)
			}
		}
		if _, err := cli.output(cleanCtx, "image", "rm", name+":smoke"); err != nil {
			t.Errorf("cleanup image: %v", err)
		}
	})
	for _, action := range []string{"up", "rebuild", "down", "up", "down"} {
		rc, err := cli.Compose(ctx, p, action)
		if err != nil {
			t.Fatal(err)
		}
		out, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			t.Fatalf("%s: %v\n%s", action, err, out)
		}
		projects, err := cli.ComposeProjects(ctx)
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, project := range projects {
			if project.Name == name {
				found = true
			}
		}
		if found != (action != "down") {
			t.Fatalf("%s: project presence = %v", action, found)
		}
	}
}
