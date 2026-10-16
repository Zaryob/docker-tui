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

	// Traversal is rejected rather than silently changing the entry's path.
	buf.Reset()
	tw = tar.NewWriter(&buf)
	tw.WriteHeader(&tar.Header{Name: "../../up.txt", Typeflag: tar.TypeReg, Mode: 0o644, Size: 1})
	tw.Write([]byte("x"))
	tw.Close()
	dst := t.TempDir()
	if _, err := Untar(&buf, dst, ""); err == nil {
		t.Error("parent traversal was accepted")
	}
	if _, err := os.Stat(filepath.Join(dst, "up.txt")); !os.IsNotExist(err) {
		t.Errorf("traversal entry unexpectedly created a file: %v", err)
	}
}

func TestUntarRejectsFileSymlinkEscapes(t *testing.T) {
	for _, fromArchive := range []bool{false, true} {
		t.Run(map[bool]string{false: "existing", true: "archive"}[fromArchive], func(t *testing.T) {
			for _, kind := range []string{"absolute", "relative", "dangling"} {
				t.Run(kind, func(t *testing.T) {
					dst, outside := t.TempDir(), t.TempDir()
					victim := filepath.Join(outside, "victim")
					if err := os.WriteFile(victim, []byte("original"), 0o600); kind != "dangling" && err != nil {
						t.Fatal(err)
					}
					link := victim
					if kind == "relative" {
						var err error
						link, err = filepath.Rel(dst, victim)
						if err != nil {
							t.Fatal(err)
						}
					} else if kind == "dangling" {
						victim = filepath.Join(outside, "missing")
						link = victim
					}
					var buf bytes.Buffer
					tw := tar.NewWriter(&buf)
					if fromArchive {
						if err := tw.WriteHeader(&tar.Header{Name: "copy", Typeflag: tar.TypeSymlink, Linkname: link}); err != nil {
							t.Fatal(err)
						}
					} else if err := os.Symlink(link, filepath.Join(dst, "copy")); err != nil {
						t.Fatal(err)
					}
					if err := tw.WriteHeader(&tar.Header{Name: "./copy", Typeflag: tar.TypeReg, Mode: 0o600, Size: 7}); err != nil {
						t.Fatal(err)
					}
					if _, err := tw.Write([]byte("changed")); err != nil {
						t.Fatal(err)
					}
					if err := tw.Close(); err != nil {
						t.Fatal(err)
					}
					if _, err := Untar(&buf, dst, ""); err == nil {
						t.Error("write through an escaping file symlink was accepted")
					}
					if kind == "dangling" {
						if _, err := os.Stat(victim); !os.IsNotExist(err) {
							t.Errorf("outside file created through dangling symlink: %v", err)
						}
					} else if data, err := os.ReadFile(victim); err != nil || string(data) != "original" {
						t.Errorf("outside file = %q, %v; want original", data, err)
					}
				})
			}
		})
	}
}

func TestUntarSafeSymlinksAndDirectoryRename(t *testing.T) {
	src, dst := t.TempDir(), t.TempDir()
	if err := os.MkdirAll(filepath.Join(src, "dir", "nested"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "dir", "nested", "file"), []byte("new"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Untar(Tar(filepath.Join(src, "dir"), "dir"), dst, "renamed"); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("renamed/nested/file", filepath.Join(dst, "leaf")); err != nil {
		t.Fatal(err)
	}
	if _, err := Untar(Tar(filepath.Join(src, "dir", "nested", "file"), "leaf"), dst, ""); err != nil {
		t.Fatalf("safe relative file symlink: %v", err)
	}
	if err := os.Symlink("renamed", filepath.Join(dst, "parent")); err != nil {
		t.Fatal(err)
	}
	if _, err := Untar(Tar(filepath.Join(src, "dir", "nested"), "nested"), filepath.Join(dst, "parent"), ""); err != nil {
		t.Fatalf("destination directory symlink: %v", err)
	}
	if data, err := os.ReadFile(filepath.Join(dst, "leaf")); err != nil || string(data) != "new" {
		t.Fatalf("copied file = %q, %v", data, err)
	}
}

func TestUntarRejectsParentDirectorySymlink(t *testing.T) {
	for _, typ := range []byte{tar.TypeReg, tar.TypeDir, tar.TypeSymlink} {
		t.Run(string(typ), func(t *testing.T) {
			outside := t.TempDir()
			dst := filepath.Join(outside, "dst")
			if err := os.Mkdir(dst, 0o700); err != nil {
				t.Fatal(err)
			}
			victim := filepath.Join(outside, "victim")
			if err := os.WriteFile(victim, []byte("original"), 0o600); err != nil {
				t.Fatal(err)
			}
			var buf bytes.Buffer
			tw := tar.NewWriter(&buf)
			if err := tw.WriteHeader(&tar.Header{Name: "up", Typeflag: tar.TypeSymlink, Linkname: "../"}); err != nil {
				t.Fatal(err)
			}
			hdr := &tar.Header{Name: "up/victim", Typeflag: typ, Mode: 0o700, Linkname: "dst"}
			if typ == tar.TypeReg {
				hdr.Size = 7
			} else {
				hdr.Name = "up/created"
			}
			if err := tw.WriteHeader(hdr); err != nil {
				t.Fatal(err)
			}
			if typ == tar.TypeReg {
				if _, err := tw.Write([]byte("changed")); err != nil {
					t.Fatal(err)
				}
			}
			if err := tw.Close(); err != nil {
				t.Fatal(err)
			}
			if _, err := Untar(&buf, dst, ""); err == nil {
				t.Error("parent-directory symlink traversal accepted")
			}
			if data, err := os.ReadFile(victim); err != nil || string(data) != "original" {
				t.Errorf("outside victim = %q, %v", data, err)
			}
			if _, err := os.Lstat(filepath.Join(outside, "created")); !os.IsNotExist(err) {
				t.Errorf("created an entry outside destination: %v", err)
			}
		})
	}
}
