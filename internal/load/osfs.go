package load

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// OSFS reads the real disk. Names are slash paths; on Windows an absolute
// name looks like "C:/dir/file".
type OSFS struct{}

// ReadFile reads the file name.
func (OSFS) ReadFile(name string) ([]byte, error) { return os.ReadFile(filepath.FromSlash(name)) }

// ReadDir lists the directory name, sorted by name.
func (OSFS) ReadDir(name string) ([]fs.DirEntry, error) { return os.ReadDir(filepath.FromSlash(name)) }

// Abs returns name as an absolute slash path.
func (OSFS) Abs(name string) (string, error) {
	a, err := filepath.Abs(filepath.FromSlash(name))
	return filepath.ToSlash(a), err
}

// Display shows name relative to the working directory when it lies inside
// it, and as it is otherwise.
func (OSFS) Display(name string) string {
	cwd, err := os.Getwd()
	if err != nil {
		return name
	}
	rel, err := filepath.Rel(cwd, filepath.FromSlash(name))
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return name
	}
	return filepath.ToSlash(rel)
}
