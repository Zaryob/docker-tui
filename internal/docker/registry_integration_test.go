// Copyright (C) 2026 Suleyman Poyraz
// SPDX-License-Identifier: GPL-3.0-or-later

//go:build integration

package docker_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/zaryob/docker-tui/internal/buildx"
	"github.com/zaryob/docker-tui/internal/registry"
)

func TestRealRegistryLoginPullLogout(t *testing.T) {
	client := integrationClient(t)
	if !strings.HasPrefix(client.Host(), "unix://") {
		unavailable(t, "registry fixture requires a local Unix-socket daemon")
	}
	bin, err := exec.LookPath("docker")
	if err != nil {
		unavailable(t, "Docker CLI required for registry fixture")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	configDir := t.TempDir()
	// Login/logout must never alter the user's Docker credential store.
	t.Setenv("DOCKER_CONFIG", configDir)
	t.Setenv("DOCKER_AUTH_CONFIG", "")
	// A nonempty file store disables Docker's automatic native-store detection.
	// Synthetic fixture credentials stay entirely in this temporary directory.
	if err := os.WriteFile(filepath.Join(configDir, "config.json"), []byte(`{"auths":{"fixture.invalid":{}}}`), 0600); err != nil {
		t.Fatal(err)
	}
	env := buildx.EnvFor(os.Environ(), client.Host(), "")
	cli := func(ctx context.Context, args ...string) ([]byte, error) {
		cmd := exec.CommandContext(ctx, bin, args...)
		cmd.Env = env
		return cmd.CombinedOutput()
	}
	dir := t.TempDir()
	// Synthetic bcrypt credential for the disposable registry only.
	htpasswd := "dtui-test:$2y$05$isl0ERja1mlHi7To1fzPkunJlp5s9rpNz62Uit6Dp2HRvcS7U/Bnm\n"
	if err := os.WriteFile(filepath.Join(dir, "htpasswd"), []byte(htpasswd), 0644); err != nil {
		t.Fatal(err)
	}
	name := fmt.Sprintf("dtui-registry-%d", time.Now().UnixNano())
	var ref string
	t.Cleanup(func() {
		cleanCtx, cleanCancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cleanCancel()
		if out, err := cli(cleanCtx, "rm", "-f", name); err != nil {
			t.Errorf("registry cleanup: %v (%s)", err, out)
		}
		if ref != "" {
			if _, err := cli(cleanCtx, "image", "inspect", ref); err == nil {
				if out, err := cli(cleanCtx, "image", "rm", ref); err != nil {
					t.Errorf("fixture image cleanup: %v (%s)", err, out)
				}
			}
		}
	})
	out, err := cli(ctx, "run", "-d", "--name", name, "--label", "docker-tui-test=true", "--network", "host", "-e", "REGISTRY_HTTP_ADDR=127.0.0.1:0",
		"--mount", "type=bind,source="+dir+",target=/auth,readonly",
		"-e", "REGISTRY_AUTH=htpasswd", "-e", "REGISTRY_AUTH_HTPASSWD_REALM=dtui-test", "-e", "REGISTRY_AUTH_HTPASSWD_PATH=/auth/htpasswd", "registry:3")
	if err != nil {
		t.Fatalf("start registry: %v (%s)", err, out)
	}
	// Listen on an ephemeral loopback port in the daemon's host namespace.
	// This is reachable by the Engine on both Linux and Docker Desktop; Desktop
	// publishes regular -p ports outside the VM, away from the Engine loopback.
	readyCtx, readyCancel := context.WithTimeout(ctx, 15*time.Second)
	defer readyCancel()
	listener := regexp.MustCompile(`listening on (127\.0\.0\.1:[0-9]+)`)
	var server string
	for server == "" {
		logs, _ := cli(readyCtx, "logs", name)
		if match := listener.FindSubmatch(logs); len(match) == 2 {
			server = string(match[1])
			break
		}
		select {
		case <-readyCtx.Done():
			t.Fatal("registry listener did not become ready")
		case <-time.After(100 * time.Millisecond):
		}
	}
	auth := &registry.Client{ConfigDir: configDir, Host: client.Host()}
	if err := auth.Login(ctx, server, "dtui-test", "dtui-test-password"); err != nil {
		t.Fatal(err)
	}

	ref = server + "/dtui-test:smoke"
	for _, args := range [][]string{{"pull", "alpine:3.21"}, {"tag", "alpine:3.21", ref}, {"push", ref}, {"image", "rm", ref}} {
		if out, err := cli(ctx, args...); err != nil {
			t.Fatalf("prepare private image (%s): %v (%s)", args[0], err, out)
		}
	}
	stream, err := client.PullImage(ctx, ref)
	if err != nil {
		t.Fatal(err)
	}
	for {
		_, err = stream.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			stream.Close()
			t.Fatalf("authenticated pull: %v", err)
		}
	}
	stream.Close()
	if err := auth.Logout(ctx, server); err != nil {
		t.Fatal(err)
	}
	credentials, err := auth.Credentials(ctx, ref)
	if err != nil || credentials.Username != "" || credentials.Password != "" {
		t.Fatal("logout retained fixture credentials")
	}
	stream, err = client.PullImage(ctx, ref)
	if err == nil {
		defer stream.Close()
		for err == nil {
			_, err = stream.Next()
		}
	}
	if err == nil || errors.Is(err, io.EOF) || !strings.Contains(err.Error(), "ctrl+l") {
		t.Fatalf("unauthenticated pull should require login: %v", err)
	}
}
