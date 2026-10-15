// Copyright (C) 2026 Suleyman Poyraz
// SPDX-License-Identifier: GPL-3.0-or-later

package docker

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// defaultContext is the name of the implicit context of the docker CLI,
// which uses DOCKER_HOST or the default socket.
const defaultContext = "default"

// configDir returns the docker CLI configuration directory, honoring
// DOCKER_CONFIG.
func configDir() (string, error) {
	if dir := os.Getenv("DOCKER_CONFIG"); dir != "" {
		return dir, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".docker"), nil
}

// CurrentContext returns the context selected for the docker CLI, either
// through DOCKER_CONTEXT or "docker context use". It returns "default" when
// no context is selected.
func CurrentContext() string {
	if name := os.Getenv("DOCKER_CONTEXT"); name != "" {
		return name
	}
	dir, err := configDir()
	if err != nil {
		return defaultContext
	}
	data, err := os.ReadFile(filepath.Join(dir, "config.json"))
	if err != nil {
		return defaultContext
	}
	var cfg struct {
		CurrentContext string `json:"currentContext"`
	}
	if json.Unmarshal(data, &cfg) != nil || cfg.CurrentContext == "" {
		return defaultContext
	}
	return cfg.CurrentContext
}

// ContextHost returns the daemon address of a docker CLI context. The
// "default" context resolves to DOCKER_HOST or, when it is not set, to the
// default socket.
func ContextHost(name string) (string, error) {
	if name == "" || name == defaultContext {
		if host := os.Getenv("DOCKER_HOST"); host != "" {
			return host, nil
		}
		return defaultHost(), nil
	}
	dir, err := configDir()
	if err != nil {
		return "", err
	}
	// The CLI stores the metadata of a context in a directory named after
	// the SHA-256 digest of its name.
	sum := sha256.Sum256([]byte(name))
	path := filepath.Join(dir, "contexts", "meta", hex.EncodeToString(sum[:]), "meta.json")
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return "", fmt.Errorf("docker context %q not found", name)
	}
	if err != nil {
		return "", err
	}
	var meta struct {
		Endpoints map[string]struct {
			Host string `json:"Host"`
		} `json:"Endpoints"`
	}
	if err := json.Unmarshal(data, &meta); err != nil {
		return "", fmt.Errorf("docker context %q: %w", name, err)
	}
	host := meta.Endpoints["docker"].Host
	if host == "" {
		return "", fmt.Errorf("docker context %q has no docker endpoint", name)
	}
	return host, nil
}
