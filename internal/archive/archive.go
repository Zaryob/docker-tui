// Copyright (C) 2026 Suleyman Poyraz
// SPDX-License-Identifier: GPL-3.0-or-later

// Package archive packs local files into tar streams and unpacks them, the
// format the Engine API uses to copy files to and from containers.
package archive

import (
	"archive/tar"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// Tar returns a tar stream of src, a file or a directory, whose top-level
// entry is named root. Errors are returned by Read.
func Tar(src, root string) io.ReadCloser {
	pr, pw := io.Pipe()
	go func() {
		tw := tar.NewWriter(pw)
		err := filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			rel, err := filepath.Rel(src, p)
			if err != nil {
				return err
			}
			name := root
			if rel != "." {
				name = path.Join(root, filepath.ToSlash(rel))
			}
			info, err := d.Info()
			if err != nil {
				return err
			}
			link := ""
			if info.Mode()&fs.ModeSymlink != 0 {
				if link, err = os.Readlink(p); err != nil {
					return err
				}
			}
			hdr, err := tar.FileInfoHeader(info, link)
			if err != nil {
				return err
			}
			hdr.Name = name
			if info.IsDir() {
				hdr.Name += "/"
			}
			if err := tw.WriteHeader(hdr); err != nil {
				return err
			}
			if !info.Mode().IsRegular() {
				return nil
			}
			f, err := os.Open(p)
			if err != nil {
				return err
			}
			defer f.Close()
			_, err = io.Copy(tw, f)
			return err
		})
		if err == nil {
			err = tw.Close()
		}
		pw.CloseWithError(err)
	}()
	return pr
}

// Untar unpacks a tar stream into the directory dir. When rename is not
// empty, the top-level entry of the archive is renamed to it, e.g. to copy
// a file under another name. Entries that would land outside dir are
// rejected. Untar returns the number of files and directories written.
func Untar(r io.Reader, dir, rename string) (int, error) {
	root, err := os.OpenRoot(dir)
	if err != nil {
		return 0, err
	}
	defer root.Close()
	if rename != "" && (!filepath.IsLocal(rename) || filepath.Base(rename) != rename || rename == ".") {
		return 0, fmt.Errorf("invalid archive destination name %q", rename)
	}
	tr := tar.NewReader(r)
	n := 0
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return n, nil
		}
		if err != nil {
			return n, err
		}
		name := filepath.FromSlash(path.Clean(hdr.Name))
		if !filepath.IsLocal(name) {
			return n, fmt.Errorf("archive entry %q is outside the destination", hdr.Name)
		}
		if rename != "" {
			parts := strings.SplitN(name, "/", 2)
			parts[0] = rename
			name = strings.Join(parts, "/")
		}
		mode := fs.FileMode(hdr.Mode).Perm()
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := mkdirAll(root, name, mode|0o700); err != nil {
				return n, err
			}
		case tar.TypeReg:
			if err := mkdirAll(root, filepath.Dir(name), 0o755); err != nil {
				return n, err
			}
			if err := writeFile(root, name, tr, mode); err != nil {
				return n, err
			}
		case tar.TypeSymlink:
			if err := symlink(root, hdr.Linkname, name); err != nil {
				return n, err
			}
		default:
			// Devices, FIFOs and hard links are not copied.
			continue
		}
		n++
	}
}

// mkdirAll uses only rooted operations, including for existing symlinks.
// Root.MkdirAll is not available on the minimum supported Go 1.24.
func mkdirAll(root *os.Root, name string, mode fs.FileMode) error {
	if info, err := root.Stat(name); err == nil {
		if info.IsDir() {
			return nil
		}
		return fmt.Errorf("archive directory %q is not a directory", name)
	} else if !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	if parent := filepath.Dir(name); parent != name {
		if err := mkdirAll(root, parent, mode); err != nil {
			return err
		}
	}
	if err := root.Mkdir(name, mode); err != nil {
		if info, statErr := root.Stat(name); statErr == nil && info.IsDir() {
			return nil
		}
		return err
	}
	return nil
}

func writeFile(root *os.Root, name string, r io.Reader, mode fs.FileMode) error {
	f, err := root.OpenFile(name, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode|0o600)
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, r); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}
