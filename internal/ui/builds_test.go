// Copyright (C) 2026 Suleyman Poyraz
// SPDX-License-Identifier: GPL-3.0-or-later

package ui

import (
	"context"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/zaryob/docker-tui/internal/buildx"
)

func TestBuildItems(t *testing.T) {
	created := time.Date(2026, 10, 15, 16, 38, 29, 0, time.UTC)
	items := buildItems([]buildx.Record{{
		Ref: "desktop-linux/desktop-linux/l0a85j0ug3achlbybao4rttcr", Name: "runtime", Status: "Completed",
		CreatedAt: created, CompletedAt: created.Add(21700 * time.Millisecond),
		TotalSteps: 24, CompletedSteps: 24, CachedSteps: 6,
	}}, created.Add(time.Hour))
	want := []string{"l0a85j0ug3ac", "runtime", "Completed", "24/24", "25%", "an hour ago", "21.7s"}
	if !slices.Equal(items[0].cells, want) {
		t.Errorf("cells = %q, want %q", items[0].cells, want)
	}
	if items[0].id != "l0a85j0ug3achlbybao4rttcr" {
		t.Errorf("id = %q", items[0].id)
	}
}

func TestFormatDuration(t *testing.T) {
	for d, want := range map[time.Duration]string{
		0: "-", 850 * time.Millisecond: "850ms", 21700 * time.Millisecond: "21.7s",
		185 * time.Second: "3m05s", 3*time.Hour + 4*time.Minute: "3h04m",
	} {
		if got := formatDuration(d); got != want {
			t.Errorf("formatDuration(%v) = %q, want %q", d, got, want)
		}
	}
}

func TestBuildFormOptions(t *testing.T) {
	dir := t.TempDir()
	f := newBuildForm(dir)
	if _, err := f.options(); err == nil || !strings.Contains(err.Error(), "no Dockerfile") {
		t.Errorf("missing Dockerfile: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "Dockerfile.dev"), []byte("FROM scratch\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	f.inputs[formDockerfile].SetValue("Dockerfile.dev")
	f.inputs[formTag].SetValue("app:dev, app:latest")
	opts, err := f.options()
	if err != nil {
		t.Fatal(err)
	}
	if opts.Context != dir || opts.Dockerfile != filepath.Join(dir, "Dockerfile.dev") || !slices.Equal(opts.Tags, []string{"app:dev", "app:latest"}) {
		t.Errorf("options = %+v", opts)
	}
	f.inputs[formContext].SetValue(filepath.Join(dir, "missing"))
	if _, err := f.options(); err == nil {
		t.Error("missing context accepted")
	}
}

func TestBuildFormFlow(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "Dockerfile"), []byte("FROM scratch\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var gotArgs []string
	a := newTestApp(t, http.NotFoundHandler())
	a.buildx.Command = func(ctx context.Context, args ...string) (*exec.Cmd, error) {
		gotArgs = args
		return exec.CommandContext(ctx, "sh", "-c", "echo '#1 [internal] load build definition'; echo '#2 DONE 0.1s'"), nil
	}
	a.switchTab(int(tabImages))
	a.handleKey(keyRunes("b"))
	f, ok := a.pane.(*buildForm)
	if !ok {
		t.Fatalf("pane = %T, want build form", a.pane)
	}
	// q is typed into the form instead of closing it.
	f.inputs[formContext].SetValue(dir)
	a.handleKey(tea.KeyMsg{Type: tea.KeyTab})
	a.handleKey(tea.KeyMsg{Type: tea.KeyTab})
	for _, r := range "q:1" {
		a.handleKey(keyRunes(string(r)))
	}
	cmd := a.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	_, cmd = a.Update(cmd())
	l, ok := a.pane.(*logView)
	if !ok {
		t.Fatalf("pane = %T, want build output", a.pane)
	}
	for cmd != nil {
		cmd = l.update(cmd())
	}
	if strings.Join(gotArgs, " ") != "build --progress plain --tag q:1 "+dir {
		t.Errorf("args = %q", gotArgs)
	}
	if len(l.lines) != 2 || !l.done || l.err != nil {
		t.Errorf("lines = %q, done = %v, err = %v", l.lines, l.done, l.err)
	}
	if !strings.Contains(l.view(), "build finished") {
		t.Errorf("header = %q", strings.SplitN(l.view(), "\n", 2)[0])
	}
}

// fakeBuild makes the builds of a run script instead of docker build.
func fakeBuild(a *App, script string) {
	a.buildx.Command = func(ctx context.Context, args ...string) (*exec.Cmd, error) {
		return exec.CommandContext(ctx, "sh", "-c", script), nil
	}
}

func TestBuildKeepsRunningInBackground(t *testing.T) {
	a := newTestApp(t, http.NotFoundHandler())
	fakeBuild(a, "echo step 1; sleep 0.3; echo step 2")
	a.switchTab(int(tabImages))
	r := newCmdRunner(a)
	r.start(a.startBuild(buildx.BuildOptions{Context: "/src", Tags: []string{"app:1"}}))
	l := a.build
	r.run(nil, 2*time.Second, func() bool { return len(l.lines) > 0 })

	// esc hides the output but the build goes on.
	a.handleKey(tea.KeyMsg{Type: tea.KeyEsc})
	if a.pane != nil || a.build != l {
		t.Fatalf("pane = %T, build = %v", a.pane, a.build)
	}
	a.Update(tea.WindowSizeMsg{Width: 80, Height: 30})
	if !strings.Contains(a.statusLine(), "building app:1") {
		t.Errorf("status line without build indicator: %q", a.statusLine())
	}
	// b shows it again, while it runs.
	a.handleKey(keyRunes("b"))
	if a.pane != asPane(l) {
		t.Fatalf("b opened %T instead of the running build", a.pane)
	}
	a.handleKey(tea.KeyMsg{Type: tea.KeyEsc})

	r.run(nil, 3*time.Second, func() bool { return a.build == nil })
	if len(l.lines) != 2 || l.err != nil {
		t.Errorf("lines = %q, err = %v", l.lines, l.err)
	}
	if a.status != "build of app:1 finished" {
		t.Errorf("status = %q", a.status)
	}
	// Once finished, b opens a new build form.
	a.handleKey(keyRunes("b"))
	if _, ok := a.pane.(*buildForm); !ok {
		t.Errorf("pane = %T, want build form", a.pane)
	}
}

func TestCancelBuild(t *testing.T) {
	a := newTestApp(t, http.NotFoundHandler())
	fakeBuild(a, "echo started; sleep 30")
	r := newCmdRunner(a)
	r.start(a.startBuild(buildx.BuildOptions{Context: "/src"}))
	l := a.build
	r.run(nil, 2*time.Second, func() bool { return len(l.lines) > 0 })
	a.handleKey(keyRunes("x"))
	a.handleKey(tea.KeyMsg{Type: tea.KeyEsc})
	start := time.Now()
	r.run(nil, 5*time.Second, func() bool { return a.build == nil })
	if a.build != nil || time.Since(start) > 4*time.Second {
		t.Fatal("build not canceled")
	}
	if !l.canceled || a.status != "build of src canceled" {
		t.Errorf("canceled = %v, status = %q", l.canceled, a.status)
	}
}

func TestBuildFormEscCloses(t *testing.T) {
	a := newTestApp(t, http.NotFoundHandler())
	a.switchTab(int(tabBuilds))
	a.openBuildForm()
	cmd := a.handleKey(tea.KeyMsg{Type: tea.KeyEsc})
	a.Update(cmd())
	if a.pane != nil {
		t.Errorf("pane = %T after esc", a.pane)
	}
}
