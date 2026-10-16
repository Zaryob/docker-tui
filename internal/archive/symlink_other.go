// Copyright (C) 2026 Suleyman Poyraz
// SPDX-License-Identifier: GPL-3.0-or-later

//go:build !unix

package archive

import (
	"errors"
	"os"
)

func symlink(_ *os.Root, _, name string) error {
	return &os.PathError{Op: "symlink", Path: name, Err: errors.New("safe archive symlink extraction is unsupported on this platform")}
}
