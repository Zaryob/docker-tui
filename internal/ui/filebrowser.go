// Copyright (C) 2026 Suleyman Poyraz
// SPDX-License-Identifier: GPL-3.0-or-later

package ui

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
)

type fileEntry struct {
	name      string
	isDir     bool
	isParent  bool
	isCompose bool
}

// isComposeFile checks whether a file name matches Docker Compose conventions.
func isComposeFile(name string) bool {
	low := strings.ToLower(name)
	if strings.HasSuffix(low, ".yaml") || strings.HasSuffix(low, ".yml") {
		return true
	}
	return false
}

// isPrimaryComposeFile returns true for standard compose names like compose.yaml or docker-compose.yml.
func isPrimaryComposeFile(name string) bool {
	low := strings.ToLower(name)
	return low == "compose.yaml" || low == "compose.yml" ||
		low == "docker-compose.yaml" || low == "docker-compose.yml" ||
		strings.HasPrefix(low, "compose.") || strings.HasPrefix(low, "docker-compose.")
}

// readDirEntries lists entries in dir, ordering them for compose selection:
// 1. ".." (parent directory, if not at root)
// 2. Compose files (docker-compose.yml, compose.yaml, other YAMLs)
// 3. Subdirectories
// 4. Other files
func readDirEntries(dir string) ([]fileEntry, error) {
	cleanDir := filepath.Clean(expandHome(dir))
	entries, err := os.ReadDir(cleanDir)
	if err != nil {
		return nil, err
	}

	var parent []fileEntry
	parentDir := filepath.Dir(cleanDir)
	if parentDir != cleanDir {
		parent = append(parent, fileEntry{
			name:     "..",
			isDir:    true,
			isParent: true,
		})
	}

	var primaryCompose []fileEntry
	var otherCompose []fileEntry
	var dirs []fileEntry
	var otherFiles []fileEntry

	for _, e := range entries {
		name := e.Name()
		// Skip hidden files/directories by default
		if strings.HasPrefix(name, ".") {
			continue
		}

		switch {
		case e.IsDir():
			dirs = append(dirs, fileEntry{
				name:  name,
				isDir: true,
			})
		case isComposeFile(name):
			entry := fileEntry{
				name:      name,
				isDir:     false,
				isCompose: true,
			}
			if isPrimaryComposeFile(name) {
				primaryCompose = append(primaryCompose, entry)
			} else {
				otherCompose = append(otherCompose, entry)
			}
		default:
			otherFiles = append(otherFiles, fileEntry{
				name:  name,
				isDir: false,
			})
		}
	}

	// Sort groups alphabetically
	sortByName := func(a, b fileEntry) int {
		return strings.Compare(strings.ToLower(a.name), strings.ToLower(b.name))
	}
	slices.SortFunc(primaryCompose, sortByName)
	slices.SortFunc(otherCompose, sortByName)
	slices.SortFunc(dirs, sortByName)
	slices.SortFunc(otherFiles, sortByName)

	result := make([]fileEntry, 0, len(parent)+len(primaryCompose)+len(otherCompose)+len(dirs)+len(otherFiles))
	result = append(result, parent...)
	result = append(result, primaryCompose...)
	result = append(result, otherCompose...)
	result = append(result, dirs...)
	result = append(result, otherFiles...)

	return result, nil
}

// completePath expands ~ and performs tab completion on the given path.
func completePath(raw string) (string, bool) {
	if raw == "" {
		return raw, false
	}

	expanded := expandHome(raw)
	var dir, prefix string
	if strings.HasSuffix(raw, "/") || strings.HasSuffix(raw, string(filepath.Separator)) {
		dir = expanded
		prefix = ""
	} else {
		dir = filepath.Dir(expanded)
		prefix = filepath.Base(expanded)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return raw, false
	}

	var matches []string
	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(strings.ToLower(name), strings.ToLower(prefix)) {
			matches = append(matches, name)
		}
	}

	if len(matches) == 0 {
		return raw, false
	}

	if len(matches) == 1 {
		completed := filepath.Join(dir, matches[0])
		if st, err := os.Stat(completed); err == nil && st.IsDir() {
			completed += "/"
		}
		if strings.HasPrefix(raw, "~") {
			if home, err := os.UserHomeDir(); err == nil && strings.HasPrefix(completed, home) {
				completed = "~" + completed[len(home):]
			}
		}
		return completed, true
	}

	// Multiple matches: find longest common prefix
	lcp := longestCommonPrefix(matches)
	if len(lcp) > len(prefix) {
		completed := filepath.Join(dir, lcp)
		if strings.HasPrefix(raw, "~") {
			if home, err := os.UserHomeDir(); err == nil && strings.HasPrefix(completed, home) {
				completed = "~" + completed[len(home):]
			}
		}
		return completed, true
	}

	return raw, false
}

func longestCommonPrefix(strs []string) string {
	if len(strs) == 0 {
		return ""
	}
	prefix := strs[0]
	for _, s := range strs[1:] {
		for !strings.HasPrefix(strings.ToLower(s), strings.ToLower(prefix)) {
			prefix = prefix[:len(prefix)-1]
			if prefix == "" {
				return ""
			}
		}
	}
	return prefix
}
