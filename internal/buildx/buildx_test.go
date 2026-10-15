// Copyright (C) 2026 Suleyman Poyraz
// SPDX-License-Identifier: GPL-3.0-or-later

package buildx

import (
	"context"
	"io"
	"os/exec"
	"slices"
	"strings"
	"testing"
	"time"
)

// fakeCLI returns a CLI that runs script with sh instead of docker and
// records the arguments it was called with.
func fakeCLI(script string, got *[]string) *CLI {
	return &CLI{
		Host: "tcp://10.0.0.1:2375",
		Command: func(ctx context.Context, args ...string) (*exec.Cmd, error) {
			*got = args
			return exec.CommandContext(ctx, "sh", "-c", script), nil
		},
	}
}

func TestParseHistory(t *testing.T) {
	data := []byte(`{"cached_steps":5,"completed_at":"2026-10-09T22:26:22Z","completed_steps":16,"created_at":"2026-10-09T22:26:19Z","name":"docker","ref":"desktop-linux/desktop-linux/muf2zlmnp6w8","status":"Completed","total_steps":16}
{"created_at":"2026-10-09T22:30:00Z","name":"app","ref":"default/default/abc","status":"Running","total_steps":4,"completed_steps":1}
`)
	records, err := parseHistory(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 2 {
		t.Fatalf("got %d records", len(records))
	}
	r := records[0]
	if r.ID() != "muf2zlmnp6w8" || r.Name != "docker" || r.CachedSteps != 5 || r.Duration(time.Time{}) != 3*time.Second {
		t.Errorf("record = %+v", r)
	}
	now := records[1].CreatedAt.Add(time.Minute)
	if d := records[1].Duration(now); d != time.Minute {
		t.Errorf("running build duration = %v", d)
	}
	if _, err := parseHistory([]byte("not json\n")); err == nil {
		t.Error("invalid output accepted")
	}
}

func TestBuildArgs(t *testing.T) {
	got := BuildOptions{Context: "/src", Dockerfile: "/src/Dockerfile.dev", Tags: []string{"app:dev", "app:latest"}}.Args()
	want := []string{"build", "--progress", "plain", "--tag", "app:dev", "--tag", "app:latest", "--file", "/src/Dockerfile.dev", "/src"}
	if !slices.Equal(got, want) {
		t.Errorf("args = %q, want %q", got, want)
	}
	if got := (BuildOptions{Context: "."}).Args(); !slices.Equal(got, []string{"build", "--progress", "plain", "."}) {
		t.Errorf("minimal args = %q", got)
	}
}

func TestEnv(t *testing.T) {
	env := Env([]string{"PATH=/bin", "DOCKER_HOST=unix:///old", "DOCKER_CONTEXT=remote"}, "tcp://h:1")
	if !slices.Equal(env, []string{"PATH=/bin", "DOCKER_HOST=tcp://h:1"}) {
		t.Errorf("env = %q", env)
	}
}

func TestHistoryUsesCLI(t *testing.T) {
	var args []string
	c := fakeCLI(`echo '{"ref":"b/n/id1","name":"web","status":"Completed"}'`, &args)
	records, err := c.History(context.Background())
	if err != nil || len(records) != 1 || records[0].ID() != "id1" {
		t.Fatalf("History = %+v, %v", records, err)
	}
	if strings.Join(args, " ") != "buildx history ls --no-trunc --format json" {
		t.Errorf("args = %q", args)
	}

	c = fakeCLI("echo 'ERROR: no builder' >&2; exit 1", &args)
	if _, err := c.History(context.Background()); err == nil || err.Error() != "ERROR: no builder" {
		t.Errorf("error = %v", err)
	}
}

func TestBuildStreamsOutputAndError(t *testing.T) {
	var args []string
	c := fakeCLI("echo '#1 load'; echo '#2 ERROR: failed to solve' >&2; exit 1", &args)
	rc, err := c.Build(context.Background(), BuildOptions{Context: "."})
	if err != nil {
		t.Fatal(err)
	}
	defer rc.Close()
	out, err := io.ReadAll(rc)
	if string(out) != "#1 load\n#2 ERROR: failed to solve\n" {
		t.Errorf("output = %q", out)
	}
	if err == nil || !strings.HasPrefix(err.Error(), "#2 ERROR: failed to solve") {
		t.Errorf("error = %v", err)
	}

	c = fakeCLI("echo done", &args)
	rc, err = c.Build(context.Background(), BuildOptions{Context: "."})
	if err != nil {
		t.Fatal(err)
	}
	if out, err := io.ReadAll(rc); err != nil || string(out) != "done\n" {
		t.Errorf("successful build: %q, %v", out, err)
	}
	rc.Close()
}

func TestCloseAbortsBuild(t *testing.T) {
	var args []string
	c := fakeCLI("echo started; sleep 30", &args)
	rc, err := c.Build(context.Background(), BuildOptions{Context: "."})
	if err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 8)
	if _, err := rc.Read(buf); err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	rc.Close()
	if _, err := io.ReadAll(rc); err == nil {
		t.Error("read after close succeeded")
	}
	if time.Since(start) > 5*time.Second {
		t.Error("closing did not abort the build")
	}
}
