// Copyright (C) 2026 Suleyman Poyraz
// SPDX-License-Identifier: GPL-3.0-or-later

package preferences

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestRoundTripAndReset(t *testing.T) {
	file := filepath.Join(t.TempDir(), "settings", "preferences.json")
	state := Default()
	state.OnlyRunning = true
	state.Lists["containers"] = List{Filter: "shop", SortColumn: 2, SortDescending: true}
	state.Projects["daemon"] = []Project{{Name: "shop", Files: "/src/compose.yaml"}}
	if err := Save(file, state); err != nil {
		t.Fatal(err)
	}
	got, err := Load(file)
	if err != nil || !reflect.DeepEqual(got, state) {
		t.Fatalf("round trip %+v: %v", got, err)
	}
	if info, err := os.Stat(file); err != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("file permissions: %v %v", info, err)
	}
	if err := ResetLists(file); err != nil {
		t.Fatal(err)
	}
	got, err = Load(file)
	if err != nil || got.OnlyRunning || len(got.Lists) != 0 || len(got.Projects["daemon"]) != 1 {
		t.Fatalf("reset %+v: %v", got, err)
	}
	files, err := filepath.Glob(filepath.Join(filepath.Dir(file), ".preferences-*"))
	if err != nil || len(files) != 0 {
		t.Fatalf("temporary files left: %v, %v", files, err)
	}
}

func TestMissingMalformedAndFutureConfig(t *testing.T) {
	file := filepath.Join(t.TempDir(), "preferences.json")
	if state, err := Load(file); err != nil || !reflect.DeepEqual(state, Default()) {
		t.Fatalf("missing %+v: %v", state, err)
	}
	for _, data := range []string{"{", `{"version":99}`, `{"version":1,"lists":{"containers":{"sort_column":"oops"}}}`} {
		if err := os.WriteFile(file, []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
		state, err := Load(file)
		if err == nil || !reflect.DeepEqual(state, Default()) {
			t.Fatalf("malformed %q: %+v, %v", data, state, err)
		}
		if err := ResetLists(file); err != nil {
			t.Fatalf("reset malformed: %v", err)
		}
	}
	if err := os.WriteFile(file, []byte(`{"version":1,"lists":{"containers":{"filter":"web"}}}`), 0600); err != nil {
		t.Fatal(err)
	}
	state, err := Load(file)
	if err != nil || state.Lists["containers"].SortColumn != -1 {
		t.Fatalf("missing column %+v: %v", state, err)
	}
}

func TestFailedSaveKeepsPreviousFile(t *testing.T) {
	dir := t.TempDir()
	// A directory at the destination prevents rename after the temporary file
	// has been synced. Failure must leave the destination and no partial file.
	file := filepath.Join(dir, "preferences.json")
	if err := os.Mkdir(file, 0700); err != nil {
		t.Fatal(err)
	}
	if err := Save(file, Default()); err == nil {
		t.Fatal("expected rename failure")
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 1 || !entries[0].IsDir() {
		t.Fatalf("partial write artifacts: %v, %v", entries, err)
	}
}
