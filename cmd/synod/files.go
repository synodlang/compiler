package main

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// collect expands paths (default ".") into the list of files to process.
// Directories are walked recursively for *.synod files, skipping directories
// whose names start with "." or "_" and directories named testdata. Explicit
// files are taken whatever their extension. The returned paths are cleaned
// and slash-separated.
func collect(paths []string) ([]string, error) {
	if len(paths) == 0 {
		paths = []string{"."}
	}
	var out []string
	for _, p := range paths {
		info, err := os.Stat(p)
		if err != nil {
			return nil, err
		}
		if !info.IsDir() {
			out = append(out, filepath.ToSlash(filepath.Clean(p)))
			continue
		}
		err = filepath.WalkDir(p, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			name := d.Name()
			if d.IsDir() {
				if path != p && (strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_") || name == "testdata") {
					return filepath.SkipDir
				}
				return nil
			}
			if strings.HasSuffix(name, ".synod") {
				out = append(out, filepath.ToSlash(filepath.Clean(path)))
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	if len(out) == 0 {
		return nil, errors.New("no .synod files found")
	}
	return out, nil
}

// writeFileAtomic replaces path with data, keeping its mode, by writing a
// temporary file in the same directory and renaming it.
func writeFileAtomic(path string, data []byte) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".synod-fmt-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	fail := func(err error) error {
		tmp.Close()
		os.Remove(name)
		return fmt.Errorf("%s: %w", path, err)
	}
	if _, err := tmp.Write(data); err != nil {
		return fail(err)
	}
	if err := tmp.Chmod(info.Mode().Perm()); err != nil {
		return fail(err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(name)
		return err
	}
	if err := os.Rename(name, path); err != nil {
		os.Remove(name)
		return err
	}
	return nil
}
