package get

import (
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// file is one file of a module tree: its slash path relative to the module
// root and its place on disk.
type file struct {
	rel, abs string
}

// moduleFiles lists the files of the module rooted at root, sorted by their
// slash path in byte order. With exclude, ".git" and every folder that holds
// its own synod.mod (a nested module) are not part of the module (decision
// 95); without it, everything is listed. A symbolic link or any other file
// that is not a regular file is an error.
func moduleFiles(root string, exclude bool) ([]file, error) {
	var out []file
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if p == root {
			return nil
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if exclude && d.Name() == ".git" {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		switch {
		case d.Type()&fs.ModeSymlink != 0:
			return fmt.Errorf("%s is a symbolic link", rel)
		case d.IsDir():
			if exclude {
				if _, err := os.Lstat(filepath.Join(p, "synod.mod")); err == nil {
					return filepath.SkipDir
				}
			}
			return nil
		case !d.Type().IsRegular():
			return fmt.Errorf("%s is not a regular file", rel)
		case strings.ContainsAny(rel, "\n\r"):
			return fmt.Errorf("%q has a line break in its name", rel)
		}
		out = append(out, file{rel, p})
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(out, func(i, j int) bool { return out[i].rel < out[j].rel })
	return out, nil
}

// TreeHash returns the h1 hash of the module tree rooted at dir: Go's dirhash
// Hash1, the base64 of the SHA-256 of one line "<sha256 hex>  <path>\n" per
// file, sorted by path.
func TreeHash(dir string) (string, error) { return treeHash(dir, true) }

func treeHash(dir string, exclude bool) (string, error) {
	files, err := moduleFiles(dir, exclude)
	if err != nil {
		return "", err
	}
	return hashFiles(files)
}

func hashFiles(files []file) (string, error) {
	sum := sha256.New()
	for _, f := range files {
		data, err := os.ReadFile(f.abs)
		if err != nil {
			return "", err
		}
		fmt.Fprintf(sum, "%x  %s\n", sha256.Sum256(data), f.rel)
	}
	return "h1:" + base64.StdEncoding.EncodeToString(sum.Sum(nil)), nil
}
