package get

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
)

// copyFiles writes the files into dst, creating folders (0755) and files
// (0644) as needed. Nothing is linked: the copy shares no storage with the
// source.
func copyFiles(files []file, dst string) error {
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return err
	}
	for _, f := range files {
		to := filepath.Join(dst, filepath.FromSlash(f.rel))
		if err := os.MkdirAll(filepath.Dir(to), 0o755); err != nil {
			return err
		}
		data, err := os.ReadFile(f.abs)
		if err != nil {
			return err
		}
		if err := os.WriteFile(to, data, 0o644); err != nil {
			return err
		}
	}
	return nil
}

// makeReadOnly makes the tree at root read-only: files 0444, folders 0555.
func makeReadOnly(root string) error {
	return filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		switch {
		case d.Type()&fs.ModeSymlink != 0:
			return nil
		case d.IsDir():
			return os.Chmod(p, 0o555)
		}
		return os.Chmod(p, 0o444)
	})
}

// forceRemoveAll removes path although parts of it are read-only. A folder
// without write permission cannot lose entries on Unix, and on Windows a
// file with the read-only attribute cannot be removed at all, so every folder
// and file is made writable first (os.Chmod clears the attribute there).
func forceRemoveAll(path string) error {
	if _, err := os.Lstat(path); errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	_ = filepath.WalkDir(path, func(p string, d fs.DirEntry, err error) error {
		if d == nil || d.Type()&fs.ModeSymlink != 0 {
			return nil
		}
		if d.IsDir() {
			_ = os.Chmod(p, 0o755)
		} else {
			_ = os.Chmod(p, 0o644)
		}
		return nil
	})
	return os.RemoveAll(path)
}

// writeAtomic writes data to name through a temporary file in the same
// folder, so a reader never sees half of it.
func writeAtomic(name string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(name), ".tmp-")
	if err != nil {
		return err
	}
	_, werr := tmp.Write(data)
	cerr := tmp.Close()
	if err := errors.Join(werr, cerr); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	_ = os.Chmod(tmp.Name(), 0o644)
	if err := os.Rename(tmp.Name(), name); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	return nil
}
