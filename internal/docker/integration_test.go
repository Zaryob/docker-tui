// Copyright (C) 2026 Suleyman Poyraz
// SPDX-License-Identifier: GPL-3.0-or-later

//go:build integration

package docker_test

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/zaryob/docker-tui/internal/archive"
	"github.com/zaryob/docker-tui/internal/buildx"
	"github.com/zaryob/docker-tui/internal/docker"
)

// Local runs may skip missing prerequisites. CI must exercise the daemon.
func unavailable(t *testing.T, format string, args ...any) {
	t.Helper()
	if os.Getenv("DOCKER_TUI_INTEGRATION_REQUIRED") == "1" {
		t.Fatalf(format, args...)
	}
	t.Skipf(format, args...)
}

func integrationClient(t *testing.T) *docker.Client {
	t.Helper()
	c, err := docker.NewClient("")
	if err != nil {
		unavailable(t, "cannot configure docker client: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	v, err := c.ServerVersion(ctx)
	if err != nil {
		unavailable(t, "docker daemon unreachable: %v", err)
	}
	t.Logf("connected to Docker daemon %s (API %s)", v.Version, v.APIVersion)
	return c
}

func TestRealDaemonPingAndVersion(t *testing.T) {
	c := integrationClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := c.Ping(ctx); err != nil {
		t.Fatalf("Ping failed: %v", err)
	}
	v, err := c.ServerVersion(ctx)
	if err != nil {
		t.Fatalf("ServerVersion failed: %v", err)
	}
	if v.Version == "" {
		t.Fatal("empty version returned by daemon")
	}
}

func TestRealDaemonListResources(t *testing.T) {
	c := integrationClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if _, err := c.ListContainers(ctx, true); err != nil {
		t.Errorf("ListContainers failed: %v", err)
	}
	if _, err := c.ListImages(ctx); err != nil {
		t.Errorf("ListImages failed: %v", err)
	}
	if _, err := c.ListVolumes(ctx); err != nil {
		t.Errorf("ListVolumes failed: %v", err)
	}
	if _, err := c.ListNetworks(ctx); err != nil {
		t.Errorf("ListNetworks failed: %v", err)
	}
}

func TestRealDaemonContainerOperations(t *testing.T) {
	c := integrationClient(t)

	// Check if docker CLI is available to launch a test container
	dockerBin, err := exec.LookPath("docker")
	if err != nil {
		unavailable(t, "docker CLI not available to launch test container")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	cliEnv := buildx.EnvFor(os.Environ(), c.Host(), c.Context())
	cli := func(ctx context.Context, args ...string) *exec.Cmd {
		cmd := exec.CommandContext(ctx, dockerBin, args...)
		cmd.Env = cliEnv
		return cmd
	}
	// Always use the known fixture, regardless of other images on the daemon.
	const testImage = "alpine:3.21"
	if out, err := cli(ctx, "pull", testImage).CombinedOutput(); err != nil {
		t.Fatalf("pull fixture: %v (%s)", err, out)
	}
	containerName := fmt.Sprintf("dtui-test-%d", time.Now().UnixNano())
	// Register cleanup before creating anything, including partially failed runs.
	removed := false
	t.Cleanup(func() {
		if removed {
			return
		}
		cleanCtx, cleanCancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cleanCancel()
		if out, err := cli(cleanCtx, "rm", "-f", containerName).CombinedOutput(); err != nil {
			t.Errorf("cleanup fixture: %v (%s)", err, out)
		}
	})
	out, err := cli(ctx, "run", "-d", "--name", containerName,
		"--label", "docker-tui-test=true", testImage,
		"sh", "-c", "echo 'hello from dtui test' && exec sleep 300").CombinedOutput()
	if err != nil {
		t.Fatalf("docker run: %v (%s)", err, out)
	}

	// 1. Inspect container
	rawInspect, err := c.InspectContainer(ctx, containerName)
	if err != nil {
		t.Fatalf("InspectContainer failed: %v", err)
	}
	if !strings.Contains(string(rawInspect), containerName) {
		t.Errorf("InspectContainer output does not contain %s", containerName)
	}

	// 2. Follow until the fixture is ready; no arbitrary startup sleep.
	logCtx, logCancel := context.WithTimeout(ctx, 10*time.Second)
	defer logCancel()
	logsRC, err := c.Logs(logCtx, containerName, docker.LogOptions{Follow: true, Tail: 10})
	if err != nil {
		t.Fatalf("Logs: %v", err)
	}
	var logBytes []byte
	buf := make([]byte, 4096)
	for !bytes.Contains(logBytes, []byte("hello from dtui test")) {
		n, err := logsRC.Read(buf)
		logBytes = append(logBytes, buf[:n]...)
		if err != nil {
			logsRC.Close()
			t.Fatalf("fixture readiness: %v (%q)", err, logBytes)
		}
	}
	logsRC.Close()

	// 3. Copy file to container
	var tarBuf bytes.Buffer
	tw := tar.NewWriter(&tarBuf)
	fileContent := []byte("dtui integration copy test")
	_ = tw.WriteHeader(&tar.Header{
		Name: "testfile.txt",
		Mode: 0o644,
		Size: int64(len(fileContent)),
	})
	_, _ = tw.Write(fileContent)
	_ = tw.Close()

	if err := c.CopyToContainer(ctx, containerName, "/tmp", bytes.NewReader(tarBuf.Bytes())); err != nil {
		t.Fatalf("CopyToContainer failed: %v", err)
	}

	// 4. Copy file from container and untar locally
	arcRC, err := c.CopyFromContainer(ctx, containerName, "/tmp/testfile.txt")
	if err != nil {
		t.Fatalf("CopyFromContainer failed: %v", err)
	}
	defer arcRC.Close()

	tmpDir := t.TempDir()
	n, err := archive.Untar(arcRC, tmpDir, "")
	if err != nil {
		t.Fatalf("Untar failed: %v", err)
	}
	if n == 0 {
		t.Fatal("Untar extracted 0 entries")
	}

	extracted, err := os.ReadFile(filepath.Join(tmpDir, "testfile.txt"))
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}
	if string(extracted) != string(fileContent) {
		t.Fatalf("extracted content mismatch: got %q, want %q", string(extracted), string(fileContent))
	}

	// 5. Container top/processes
	if procs, err := c.Top(ctx, containerName); err != nil || len(procs.Processes) == 0 {
		t.Fatalf("Top: no processes or error: %v", err)
	}

	// 6. Stop container
	if err := c.StopContainer(ctx, containerName); err != nil {
		t.Fatalf("StopContainer: %v", err)
	}

	rawInspect, err = c.InspectContainer(ctx, containerName)
	if err != nil {
		t.Fatal(err)
	}
	var stopped struct{ State struct{ Running bool } }
	if err := json.Unmarshal(rawInspect, &stopped); err != nil {
		t.Fatal(err)
	}
	if stopped.State.Running {
		t.Fatal("container still running after stop")
	}

	// 7. Remove container
	if err := c.RemoveContainer(ctx, containerName, true); err != nil {
		t.Fatalf("RemoveContainer: %v", err)
	}
	removed = true
	if _, err := c.InspectContainer(ctx, containerName); err == nil {
		t.Fatal("container still exists after removal")
	}
}
