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
	root, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return 0, err
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
		name := strings.TrimPrefix(path.Clean("/"+hdr.Name), "/")
		if name == "" {
			continue
		}
		if rename != "" {
			parts := strings.SplitN(name, "/", 2)
			parts[0] = rename
			name = strings.Join(parts, "/")
		}
		target := filepath.Join(root, filepath.FromSlash(name))
		if err := within(root, filepath.Dir(target)); err != nil {
			return n, err
		}
		mode := fs.FileMode(hdr.Mode).Perm()
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, mode|0o700); err != nil {
				return n, err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return n, err
			}
			if err := writeFile(target, tr, mode); err != nil {
				return n, err
			}
		case tar.TypeSymlink:
			_ = os.Remove(target)
			if err := os.Symlink(hdr.Linkname, target); err != nil {
				return n, err
			}
		default:
			// Devices, FIFOs and hard links are not copied.
			continue
		}
		n++
	}
}

// within checks that dir, with symlinks resolved, is root or inside it, so
// that a symlink in the archive cannot redirect later entries.
func within(root, dir string) error {
	resolved := dir
	// The directory may not exist yet; resolve its closest existing parent.
	for {
		r, err := filepath.EvalSymlinks(resolved)
		if err == nil {
			rest, _ := filepath.Rel(resolved, dir)
			resolved = filepath.Join(r, rest)
			break
		}
		parent := filepath.Dir(resolved)
		if parent == resolved {
			break
		}
		resolved = parent
	}
	rel, err := filepath.Rel(root, resolved)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("archive entry outside of %s", root)
	}
	return nil
}

func writeFile(name string, r io.Reader, mode fs.FileMode) error {
	f, err := os.OpenFile(name, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode|0o600)
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, r); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}
