// Copyright (C) 2026 Suleyman Poyraz
// SPDX-License-Identifier: GPL-3.0-or-later

package docker

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
)

// writeContext creates the metadata of a docker CLI context in dir.
func writeContext(t *testing.T, dir, name, host string) {
	t.Helper()
	sum := sha256.Sum256([]byte(name))
	metaDir := filepath.Join(dir, "contexts", "meta", hex.EncodeToString(sum[:]))
	if err := os.MkdirAll(metaDir, 0o755); err != nil {
		t.Fatal(err)
	}
	meta := `{"Name":"` + name + `","Metadata":{},"Endpoints":{"docker":{"Host":"` + host + `","SkipTLSVerify":false}}}`
	if err := os.WriteFile(filepath.Join(metaDir, "meta.json"), []byte(meta), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestCurrentContext(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("DOCKER_CONFIG", dir)
	t.Setenv("DOCKER_CONTEXT", "")
	if got := CurrentContext(); got != "default" {
		t.Errorf("without config: %q, want default", got)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(`{"currentContext":"remote"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := CurrentContext(); got != "remote" {
		t.Errorf("from config: %q, want remote", got)
	}
	t.Setenv("DOCKER_CONTEXT", "other")
	if got := CurrentContext(); got != "other" {
		t.Errorf("from DOCKER_CONTEXT: %q, want other", got)
	}
}

func TestContextHost(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("DOCKER_CONFIG", dir)
	writeContext(t, dir, "remote", "tcp://10.0.0.7:2375")

	host, err := ContextHost("remote")
	if err != nil || host != "tcp://10.0.0.7:2375" {
		t.Errorf("ContextHost(remote) = %q, %v", host, err)
	}
	t.Setenv("DOCKER_HOST", "tcp://10.0.0.1:2375")
	if host, err := ContextHost("default"); err != nil || host != "tcp://10.0.0.1:2375" {
		t.Errorf("ContextHost(default) = %q, %v; want DOCKER_HOST", host, err)
	}
	if _, err := ContextHost("missing"); err == nil {
		t.Error("ContextHost(missing) succeeded")
	}
}

func TestNewClientUsesCurrentContext(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("DOCKER_CONFIG", dir)
	t.Setenv("DOCKER_HOST", "")
	t.Setenv("DOCKER_CONTEXT", "remote")
	writeContext(t, dir, "remote", "tcp://10.0.0.7:2375")

	c, err := NewClient("")
	if err != nil {
		t.Fatal(err)
	}
	if c.Host() != "tcp://10.0.0.7:2375" {
		t.Errorf("Host() = %q", c.Host())
	}

	// DOCKER_HOST takes precedence over the context, like in the CLI.
	t.Setenv("DOCKER_HOST", "tcp://10.0.0.1:2375")
	if c, err := NewClient(""); err != nil || c.Host() != "tcp://10.0.0.1:2375" {
		t.Errorf("with DOCKER_HOST: %v, %v", c, err)
	}
}
