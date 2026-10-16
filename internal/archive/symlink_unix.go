// Copyright (C) 2026 Suleyman Poyraz
// SPDX-License-Identifier: GPL-3.0-or-later

//go:build unix

package archive

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
)

// symlink anchors creation to an opened parent directory. Using os.Symlink
// with a checked pathname would allow a parent symlink swap to escape root.
func symlink(root *os.Root, oldname, name string) error {
	parent, err := root.Open(filepath.Dir(name))
	if err != nil {
		return err
	}
	defer parent.Close()
	if err := root.Remove(name); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	if err := unix.Symlinkat(oldname, int(parent.Fd()), filepath.Base(name)); err != nil {
		return &os.LinkError{Op: "symlink", Old: oldname, New: name, Err: err}
	}
	return nil
}
