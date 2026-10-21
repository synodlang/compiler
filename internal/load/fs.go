package load

import (
	"io/fs"
	"path"
	"sort"
	"strings"
	"time"
)

// FS is the file system the loader reads. All names are slash separated.
// OSFS reads the real disk and MapFS an in-memory tree; nothing else in the
// compiler needs the os package.
type FS interface {
	ReadFile(name string) ([]byte, error)
	ReadDir(name string) ([]fs.DirEntry, error)
}

// MapFS is an in-memory FS: a map from slash separated file path to content.
// Directories exist implicitly, as the parents of files. Paths are cleaned
// with path.Clean, so "/m/x" and "/m//x/" name the same entry. ReadDir
// returns entries sorted by name.
type MapFS map[string]string

// ReadFile returns the content of the file name.
func (m MapFS) ReadFile(name string) ([]byte, error) {
	name = path.Clean(name)
	for k, v := range m {
		if path.Clean(k) == name {
			return []byte(v), nil
		}
	}
	if m.isDir(name) {
		return nil, &fs.PathError{Op: "read", Path: name, Err: fs.ErrInvalid}
	}
	return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
}

func (m MapFS) isDir(name string) bool {
	prefix := dirPrefix(name)
	for k := range m {
		if strings.HasPrefix(path.Clean(k), prefix) {
			return true
		}
	}
	return false
}

// dirPrefix returns the prefix that every path below dir starts with.
func dirPrefix(dir string) string {
	dir = path.Clean(dir)
	switch dir {
	case ".":
		return ""
	case "/":
		return "/"
	}
	return dir + "/"
}

// ReadDir lists the directory name, sorted by name.
func (m MapFS) ReadDir(name string) ([]fs.DirEntry, error) {
	prefix := dirPrefix(name)
	kinds := map[string]bool{} // entry name → is a directory
	for k := range m {
		k = path.Clean(k)
		if !strings.HasPrefix(k, prefix) || k == prefix {
			continue
		}
		rest := k[len(prefix):]
		if first, _, nested := strings.Cut(rest, "/"); nested {
			kinds[first] = true
		} else if _, seen := kinds[rest]; !seen {
			kinds[rest] = false
		}
	}
	if len(kinds) == 0 {
		return nil, &fs.PathError{Op: "readdir", Path: name, Err: fs.ErrNotExist}
	}
	names := make([]string, 0, len(kinds))
	for n := range kinds {
		names = append(names, n)
	}
	sort.Strings(names)
	out := make([]fs.DirEntry, len(names))
	for i, n := range names {
		out[i] = fs.FileInfoToDirEntry(mapInfo{name: n, dir: kinds[n]})
	}
	return out, nil
}

// mapInfo is the fs.FileInfo of a MapFS entry.
type mapInfo struct {
	name string
	dir  bool
}

func (i mapInfo) Name() string { return i.name }
func (i mapInfo) Size() int64  { return 0 }
func (i mapInfo) Mode() fs.FileMode {
	if i.dir {
		return fs.ModeDir | 0o755
	}
	return 0o644
}
func (i mapInfo) ModTime() time.Time { return time.Time{} }
func (i mapInfo) IsDir() bool        { return i.dir }
func (i mapInfo) Sys() any           { return nil }

// join joins slash paths; "." is the empty directory.
func join(dir, name string) string {
	switch {
	case dir == "" || dir == ".":
		return name
	case strings.HasSuffix(dir, "/"):
		return dir + name
	}
	return dir + "/" + name
}

// parent returns the directory above dir, or "" at the top.
func parent(dir string) string {
	if dir == "/" || dir == "." || dir == "" {
		return ""
	}
	if len(dir) == 2 && dir[1] == ':' { // a Windows drive such as "C:"
		return ""
	}
	p := path.Dir(dir)
	if p == dir {
		return ""
	}
	if p == "." && strings.HasPrefix(dir, "./") {
		return ""
	}
	return p
}

// isSynodName reports whether name is a source file the loader reads:
// "*.synod" not starting with "." or "_".
func isSynodName(name string) bool {
	return strings.HasSuffix(name, ".synod") && !strings.HasPrefix(name, ".") && !strings.HasPrefix(name, "_")
}

// Discover lists, in order, every folder at or below root that holds source
// files, as slash paths. Folders named ".x", "_x", "testdata" or "vendor" are skipped,
// and so is a folder with its own synod.mod below root (a nested module).
func Discover(fsys FS, root string) ([]string, error) {
	var out []string
	var walk func(dir string) error
	walk = func(dir string) error {
		ents, err := fsys.ReadDir(dir)
		if err != nil {
			return err
		}
		has := false
		for _, e := range ents {
			if !e.IsDir() && isSynodName(e.Name()) {
				has = true
			}
		}
		if has {
			out = append(out, dir)
		}
		for _, e := range ents {
			n := e.Name()
			if !e.IsDir() || strings.HasPrefix(n, ".") || strings.HasPrefix(n, "_") || n == "testdata" || n == "vendor" {
				continue
			}
			sub := join(dir, n)
			if _, err := fsys.ReadFile(join(sub, "synod.mod")); err == nil {
				continue
			}
			if err := walk(sub); err != nil {
				return err
			}
		}
		return nil
	}
	if err := walk(root); err != nil {
		return nil, err
	}
	return out, nil
}
