// Copyright (C) 2026 Suleyman Poyraz
// SPDX-License-Identifier: GPL-3.0-or-later

//go:build integration

package docker_test

import (
	"archive/tar"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/zaryob/docker-tui/internal/archive"
	"github.com/zaryob/docker-tui/internal/docker"
)

func integrationClient(t *testing.T) *docker.Client {
	t.Helper()
	c, err := docker.NewClient("")
	if err != nil {
		t.Skipf("skipping integration test: cannot configure docker client: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	v, err := c.ServerVersion(ctx)
	if err != nil {
		t.Skipf("skipping integration test: docker daemon unreachable: %v", err)
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
		t.Skip("docker CLI not available to launch test container")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Find an existing image to use
	images, err := c.ListImages(ctx)
	if err != nil {
		t.Fatalf("ListImages: %v", err)
	}
	var testImage string
	for _, img := range images {
		for _, tag := range img.RepoTags {
			if tag != "<none>:<none>" {
				testImage = tag
				break
			}
		}
		if testImage != "" {
			break
		}
	}
	if testImage == "" {
		// Pull alpine:latest
		pullCmd := exec.CommandContext(ctx, dockerBin, "pull", "alpine:latest")
		if err := pullCmd.Run(); err != nil {
			t.Skipf("cannot pull alpine test image: %v", err)
		}
		testImage = "alpine:latest"
	}

	containerName := fmt.Sprintf("dtui-test-%d", time.Now().UnixNano())
	runCmd := exec.CommandContext(ctx, dockerBin, "run", "-d",
		"--name", containerName,
		"--label", "docker-tui-test=true",
		testImage,
		"sh", "-c", "echo 'hello from dtui test' && sleep 30",
	)
	out, err := runCmd.CombinedOutput()
	if err != nil {
		t.Skipf("docker run failed: %v (%s)", err, strings.TrimSpace(string(out)))
	}
	containerID := strings.TrimSpace(string(out))
	if len(containerID) > 12 {
		containerID = containerID[:12]
	}

	// Clean up after test
	t.Cleanup(func() {
		cleanCtx, cleanCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cleanCancel()
		_ = exec.CommandContext(cleanCtx, dockerBin, "rm", "-f", containerName).Run()
	})

	// Wait briefly for container to start
	time.Sleep(300 * time.Millisecond)

	// 1. Inspect container
	rawInspect, err := c.InspectContainer(ctx, containerName)
	if err != nil {
		t.Fatalf("InspectContainer failed: %v", err)
	}
	if !strings.Contains(string(rawInspect), containerName) {
		t.Errorf("InspectContainer output does not contain %s", containerName)
	}

	// 2. Read logs
	logsRC, err := c.Logs(ctx, containerName, docker.LogOptions{Follow: false, Tail: 10})
	if err != nil {
		t.Fatalf("Logs failed: %v", err)
	}
	logBytes, err := io.ReadAll(logsRC)
	logsRC.Close()
	if err != nil {
		t.Fatalf("read logs error: %v", err)
	}
	if !strings.Contains(string(logBytes), "hello from dtui test") {
		t.Logf("logs: %q", string(logBytes))
	}

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
	if procs, err := c.Top(ctx, containerName); err == nil {
		t.Logf("container processes: %d", len(procs.Processes))
	}

	// 6. Stop container
	if err := c.StopContainer(ctx, containerName); err != nil {
		t.Logf("StopContainer: %v", err)
	}

	// 7. Remove container
	if err := c.RemoveContainer(ctx, containerName, true); err != nil {
		t.Logf("RemoveContainer: %v", err)
	}
}
