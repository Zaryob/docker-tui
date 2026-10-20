// Copyright (C) 2026 Suleyman Poyraz
// SPDX-License-Identifier: GPL-3.0-or-later

package buildx

import (
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func composeFixture(t *testing.T) Project {
	t.Helper()
	dir := t.TempDir()
	file := filepath.Join(dir, "compose.yaml")
	if err := os.WriteFile(file, []byte("services: {}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	return Project{Name: "shop", ConfigFiles: file, WorkingDir: dir}
}

func TestComposeCommandsUseSelectedContext(t *testing.T) {
	p := composeFixture(t)
	for _, action := range []string{"up", "rebuild", "down"} {
		t.Run(action, func(t *testing.T) {
			var args []string
			c := &CLI{Context: "remote", Host: "tcp://wrong:2375", Command: func(ctx context.Context, argv ...string) (*exec.Cmd, error) {
				args = argv
				return exec.CommandContext(ctx, "sh", "-c", `test "$DOCKER_CONTEXT" = remote && test -z "$DOCKER_HOST" && echo progress`), nil
			}}
			rc, err := c.Compose(context.Background(), p, action)
			if err != nil {
				t.Fatal(err)
			}
			defer rc.Close()
			out, err := io.ReadAll(rc)
			if err != nil || !strings.Contains(string(out), "progress") {
				t.Fatalf("output %q: %v", out, err)
			}
			cmdline := strings.Join(args, " ")
			if !strings.Contains(cmdline, "--project-name shop --file "+p.ConfigFiles) {
				t.Fatalf("args %q", args)
			}
			if action == "rebuild" && !strings.HasSuffix(cmdline, "up --detach --build") {
				t.Fatal(cmdline)
			}
			if action == "down" && !strings.HasSuffix(cmdline, " down") {
				t.Fatal(cmdline)
			}
		})
	}
}

func TestComposeRejectsUnavailableLocalFiles(t *testing.T) {
	p := composeFixture(t)
	p.ConfigFiles += "," + filepath.Join(t.TempDir(), "remote.yaml")
	if _, err := p.Args(); err == nil {
		t.Fatal("accepted missing override file")
	}
	p.ConfigFiles = "relative.yaml"
	if _, err := p.Args(); err == nil {
		t.Fatal("accepted ambiguous relative path")
	}
	p.Name = "--other-project"
	if _, err := p.Args(); err == nil {
		t.Fatal("accepted invalid project name")
	}
}

func TestComposeDiscoveryAndLocalFile(t *testing.T) {
	p := composeFixture(t)
	var args []string
	c := fakeCLI(`echo '[{"Name":"shop","Status":"running(1)","ConfigFiles":"/remote/compose.yaml"}]'`, &args)
	projects, err := c.ComposeProjects(context.Background())
	if err != nil || len(projects) != 1 || projects[0].Name != "shop" {
		t.Fatalf("projects %+v: %v", projects, err)
	}
	if strings.Join(args, " ") != "compose ls --all --format json" {
		t.Fatal(args)
	}
	c = fakeCLI(`echo '{"name":"shop"}'`, &args)
	opened, err := c.OpenCompose(context.Background(), p.ConfigFiles)
	if err != nil || opened.Name != p.Name || opened.ConfigFiles != p.ConfigFiles {
		t.Fatalf("opened %+v: %v", opened, err)
	}
}

func TestComposeCancellationStopsProcessGroup(t *testing.T) {
	var args []string
	c := fakeCLI("echo started; sleep 30; echo late", &args)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	rc, err := c.Compose(ctx, composeFixture(t), "up")
	if err != nil {
		t.Fatal(err)
	}
	defer rc.Close()
	buf := make([]byte, 8)
	if _, err := io.ReadFull(rc, buf); err != nil {
		t.Fatal(err)
	}
	cancel()
	done := make(chan error, 1)
	go func() { _, err := io.ReadAll(rc); done <- err }()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("canceled operation succeeded")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("child process survived cancellation")
	}
}
