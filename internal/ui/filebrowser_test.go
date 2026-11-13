// Copyright (C) 2026 Suleyman Poyraz
// SPDX-License-Identifier: GPL-3.0-or-later

package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadDirEntries(t *testing.T) {
	tmp := t.TempDir()

	// Create test files
	_ = os.WriteFile(filepath.Join(tmp, "docker-compose.yml"), []byte("version: '3'"), 0644)
	_ = os.WriteFile(filepath.Join(tmp, "custom.yaml"), []byte("services: {}"), 0644)
	_ = os.WriteFile(filepath.Join(tmp, "README.md"), []byte("# Test"), 0644)
	_ = os.Mkdir(filepath.Join(tmp, "subfolder"), 0755)
	_ = os.Mkdir(filepath.Join(tmp, ".hidden"), 0755)

	entries, err := readDirEntries(tmp)
	if err != nil {
		t.Fatalf("readDirEntries failed: %v", err)
	}

	if len(entries) < 4 {
		t.Fatalf("expected at least 4 entries, got %d", len(entries))
	}

	// 1. ".." parent directory
	if entries[0].name != ".." || !entries[0].isParent {
		t.Errorf("first entry should be '..', got %+v", entries[0])
	}

	// 2. docker-compose.yml should come before custom.yaml and subfolder
	if entries[1].name != "docker-compose.yml" || !entries[1].isCompose {
		t.Errorf("second entry should be 'docker-compose.yml', got %+v", entries[1])
	}

	// 3. custom.yaml is also a compose/yaml file
	if entries[2].name != "custom.yaml" || !entries[2].isCompose {
		t.Errorf("third entry should be 'custom.yaml', got %+v", entries[2])
	}

	// 4. subfolder should be directory
	foundSubfolder := false
	for _, e := range entries {
		if e.name == "subfolder" && e.isDir {
			foundSubfolder = true
		}
		if e.name == ".hidden" {
			t.Errorf("hidden entry should be filtered out: %s", e.name)
		}
	}
	if !foundSubfolder {
		t.Errorf("expected 'subfolder' directory in entries")
	}
}

func TestCompletePath(t *testing.T) {
	tmp := t.TempDir()
	_ = os.WriteFile(filepath.Join(tmp, "app-service.yaml"), []byte(""), 0644)
	_ = os.WriteFile(filepath.Join(tmp, "app-worker.yaml"), []byte(""), 0644)
	_ = os.Mkdir(filepath.Join(tmp, "deploy"), 0755)

	// Single directory match
	got, ok := completePath(filepath.Join(tmp, "dep"))
	if !ok || !strings.HasSuffix(got, "deploy/") {
		t.Fatalf("completePath('dep') = %q, want trailing 'deploy/'", got)
	}

	// Longest common prefix match for app-*
	got, ok = completePath(filepath.Join(tmp, "app"))
	if !ok || !strings.HasSuffix(got, "app-") {
		t.Fatalf("completePath('app') = %q, want trailing 'app-'", got)
	}
}

func TestComposeFormBrowsing(t *testing.T) {
	tmp := t.TempDir()
	composePath := filepath.Join(tmp, "compose.yaml")
	_ = os.WriteFile(composePath, []byte("services: {}"), 0644)
	subDir := filepath.Join(tmp, "nested")
	_ = os.Mkdir(subDir, 0755)

	f := newComposeForm(nil)
	f.setDir(tmp)
	f.setSize(80, 24)

	// Pre-selection of compose.yaml
	if len(f.entries) < 2 {
		t.Fatalf("expected entries in tmp dir, got %d", len(f.entries))
	}
	if f.entries[f.cursor].name != "compose.yaml" {
		t.Errorf("cursor should be on compose.yaml, got %s", f.entries[f.cursor].name)
	}

	// Navigation with arrow keys
	f.update(keyRunes("down")) // moves down
	if f.cursor >= len(f.entries) {
		t.Fatalf("cursor out of bounds: %d", f.cursor)
	}

	// Render view check
	view := f.view()
	if !strings.Contains(view, "compose.yaml") || !strings.Contains(view, "Directory:") {
		t.Fatalf("view does not render compose.yaml or directory: %s", view)
	}
}
