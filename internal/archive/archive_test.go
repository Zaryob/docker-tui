// Copyright (C) 2026 Suleyman Poyraz
// SPDX-License-Identifier: GPL-3.0-or-later

package archive

import (
	"archive/tar"
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestRoundTrip(t *testing.T) {
	src := t.TempDir()
	if err := os.MkdirAll(filepath.Join(src, "conf", "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(src, "conf", "a.txt"), []byte("alpha"), 0o644)
	os.WriteFile(filepath.Join(src, "conf", "sub", "b.sh"), []byte("#!/bin/sh"), 0o755)
	os.Symlink("a.txt", filepath.Join(src, "conf", "link"))

	dst := t.TempDir()
	n, err := Untar(Tar(filepath.Join(src, "conf"), "conf"), dst, "")
	if err != nil || n != 5 {
		t.Fatalf("Untar = %d, %v", n, err)
	}
	if b, _ := os.ReadFile(filepath.Join(dst, "conf", "a.txt")); string(b) != "alpha" {
		t.Errorf("a.txt = %q", b)
	}
	if fi, _ := os.Stat(filepath.Join(dst, "conf", "sub", "b.sh")); fi == nil || fi.Mode().Perm()&0o100 == 0 {
		t.Errorf("b.sh lost its mode: %v", fi)
	}
	if l, _ := os.Readlink(filepath.Join(dst, "conf", "link")); l != "a.txt" {
		t.Errorf("link = %q", l)
	}

	// Renaming the top-level entry copies a file under another name.
	if _, err := Untar(Tar(filepath.Join(src, "conf", "a.txt"), "a.txt"), dst, "renamed.txt"); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(filepath.Join(dst, "renamed.txt")); string(b) != "alpha" {
		t.Errorf("renamed.txt = %q", b)
	}
}

func TestUntarRejectsEscapes(t *testing.T) {
	outside := t.TempDir()
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	tw.WriteHeader(&tar.Header{Name: "evil", Typeflag: tar.TypeSymlink, Linkname: outside})
	tw.WriteHeader(&tar.Header{Name: "evil/pwned", Typeflag: tar.TypeReg, Mode: 0o644, Size: 1})
	tw.Write([]byte("x"))
	tw.Close()
	if _, err := Untar(&buf, t.TempDir(), ""); err == nil {
		t.Error("entry written through a symlink")
	}
	if _, err := os.Stat(filepath.Join(outside, "pwned")); err == nil {
		t.Error("file created outside the destination")
	}

	// "../" is confined to the destination.
	buf.Reset()
	tw = tar.NewWriter(&buf)
	tw.WriteHeader(&tar.Header{Name: "../../up.txt", Typeflag: tar.TypeReg, Mode: 0o644, Size: 1})
	tw.Write([]byte("x"))
	tw.Close()
	dst := t.TempDir()
	if _, err := Untar(&buf, dst, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dst, "up.txt")); err != nil {
		t.Errorf("../ entry not confined: %v", err)
	}
}
