// Copyright (C) 2026 Suleyman Poyraz
// SPDX-License-Identifier: GPL-3.0-or-later

// Package preferences stores non-secret UI preferences and local project sources.
package preferences

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// Version identifies the supported preferences file format.
const Version = 1

// List contains the display preferences of a resource tab.
type List struct {
	Filter         string `json:"filter"`
	SortColumn     int    `json:"sort_column"`
	SortDescending bool   `json:"sort_descending"`
}

// UnmarshalJSON preserves the default daemon order when sort_column is absent.
func (l *List) UnmarshalJSON(data []byte) error {
	type plain List
	value := plain{SortColumn: -1}
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	*l = List(value)
	return nil
}

// Project is a local Compose source retained after its containers are removed.
type Project struct {
	LocalSource      bool   `json:"local_source,omitempty"`
	Name             string `json:"name"`
	Files            string `json:"files"`
	WorkingDirectory string `json:"working_directory,omitempty"`
}

// State is versioned independently of the application releases.
type State struct {
	Version     int             `json:"version"`
	Lists       map[string]List `json:"lists"`
	OnlyRunning bool            `json:"only_running"`
	// Projects is keyed by a hash of the daemon endpoint and selected context.
	Projects map[string][]Project `json:"projects,omitempty"`
}

// Default returns a fresh configuration with default list behavior.
func Default() State {
	return State{Version: Version, Lists: map[string]List{}, Projects: map[string][]Project{}}
}

// Path returns the platform-specific user configuration file location.
func Path() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "docker-tui", "preferences.json"), nil
}

// Load uses defaults for missing or malformed files. The error lets the UI
// explain why saved preferences could not be restored.
func Load(file string) (State, error) {
	data, err := os.ReadFile(file)
	if errors.Is(err, os.ErrNotExist) {
		return Default(), nil
	}
	if err != nil {
		return Default(), fmt.Errorf("read preferences: %w", err)
	}
	state := Default()
	if err := json.Unmarshal(data, &state); err != nil {
		return Default(), fmt.Errorf("invalid preferences: %w", err)
	}
	if state.Version != Version {
		return Default(), fmt.Errorf("unsupported preferences version %d", state.Version)
	}
	if state.Lists == nil {
		state.Lists = map[string]List{}
	}
	if state.Projects == nil {
		state.Projects = map[string][]Project{}
	}
	return state, nil
}

// Save atomically replaces the file. Interrupted writes leave the previous
// version intact, and the file is readable only by the current user.
func Save(file string, state State) error {
	state.Version = Version
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(file), 0700); err != nil {
		return fmt.Errorf("create preferences directory: %w", err)
	}
	temp, err := os.CreateTemp(filepath.Dir(file), ".preferences-*")
	if err != nil {
		return err
	}
	name := temp.Name()
	defer os.Remove(name)
	defer temp.Close()
	if _, err := temp.Write(append(data, '\n')); err != nil {
		return err
	}
	if err := temp.Sync(); err != nil {
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	if err := os.Rename(name, file); err != nil {
		return fmt.Errorf("replace preferences: %w", err)
	}
	return nil
}

// ResetLists resets UI preferences while preserving local Compose sources.
func ResetLists(file string) error {
	state, err := Load(file)
	if err != nil {
		// Malformed/unsupported files can be explicitly reset; read failures cannot.
		if _, readErr := os.ReadFile(file); readErr != nil {
			return err
		}
		state = Default()
	}
	state.Lists = map[string]List{}
	state.OnlyRunning = false
	return Save(file, state)
}
