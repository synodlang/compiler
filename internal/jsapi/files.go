package jsapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path"
	"sort"
	"strings"

	"github.com/synodlang/compiler/internal/load"
)

// root is the virtual folder the caller's files live under in the MapFS.
const root = "/w"

// tree is the caller's files: slash paths to source text.
type tree struct {
	files map[string]string
	dirs  map[string]bool // every folder that holds a file, at any depth
	keys  []string        // sorted in the order `synod` walks a folder
}

// decodeFiles reads the files argument: a JSON object of slash paths to
// source text.
func decodeFiles(raw string) (*tree, error) {
	trimmed := strings.TrimSpace(raw)
	if !strings.HasPrefix(trimmed, "{") {
		return nil, errors.New("files must be an object mapping paths to source text")
	}
	dec := json.NewDecoder(strings.NewReader(trimmed))
	dec.UseNumber()
	var m map[string]json.RawMessage
	if err := dec.Decode(&m); err != nil {
		return nil, fmt.Errorf("files is not valid: %v", err)
	}
	if err := noTrailing(dec); err != nil {
		return nil, fmt.Errorf("files is not valid: %v", err)
	}
	t := &tree{files: make(map[string]string, len(m)), dirs: map[string]bool{}}
	for k, v := range m {
		if err := validKey(k); err != nil {
			return nil, err
		}
		var s string
		if len(v) == 0 || v[0] != '"' || json.Unmarshal(v, &s) != nil {
			return nil, fmt.Errorf("file %q must be a string", k)
		}
		t.files[k] = s
	}
	for k := range t.files {
		t.keys = append(t.keys, k)
		for d := path.Dir(k); d != "."; d = path.Dir(d) {
			if _, clash := t.files[d]; clash {
				return nil, fmt.Errorf("%q is both a file and a folder", d)
			}
			t.dirs[d] = true
		}
	}
	sort.Slice(t.keys, func(i, j int) bool { return walkLess(t.keys[i], t.keys[j]) })
	return t, nil
}

// noTrailing reports data after the first JSON value.
func noTrailing(dec *json.Decoder) error {
	if _, err := dec.Token(); err != io.EOF {
		return errors.New("unexpected data after the value")
	}
	return nil
}

// validKey checks a file path: relative, slash separated, no empty, "." or
// ".." segment.
func validKey(k string) error {
	bad := k == "" || strings.HasPrefix(k, "/") || strings.Contains(k, `\`) || strings.ContainsRune(k, 0)
	if !bad {
		for _, seg := range strings.Split(k, "/") {
			if seg == "" || seg == "." || seg == ".." {
				bad = true
				break
			}
		}
	}
	if bad {
		return fmt.Errorf("invalid file path %q: use relative paths with / and no empty, . or .. segments", k)
	}
	return nil
}

// walkLess orders paths as filepath.WalkDir visits them: segment by segment,
// so a folder's files come at the place of the folder.
func walkLess(a, b string) bool {
	as, bs := strings.Split(a, "/"), strings.Split(b, "/")
	for i := 0; i < len(as) && i < len(bs); i++ {
		if as[i] != bs[i] {
			return as[i] < bs[i]
		}
	}
	return len(as) < len(bs)
}

// loadConfig is the loader configuration: the files under /w/, shown without
// that prefix, as the command line shows paths relative to the tree root.
func (t *tree) loadConfig() load.Config {
	m := make(load.MapFS, len(t.files))
	for k, v := range t.files {
		m[root+"/"+k] = v
	}
	return load.Config{FS: m, Display: display}
}

// display turns a loader path into the path the caller used.
func display(p string) string {
	switch {
	case p == root:
		return "."
	case strings.HasPrefix(p, root+"/"):
		return p[len(root)+1:]
	}
	return p
}

// loadPath is the loader path of a clean key or ".".
func loadPath(p string) string {
	if p == "." {
		return root
	}
	return root + "/" + p
}

func (t *tree) isDir(p string) bool {
	if p == "." {
		return len(t.files) > 0
	}
	return t.dirs[p]
}

// skipDir reports the folders `synod check` and `synod fmt` do not enter.
func skipDir(name string) bool {
	return strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_") || name == "testdata" || name == "vendor"
}

// collect expands paths (default ".") like the command line: folders are
// walked for *.synod files, skipping folders that start with "." or "_" and
// folders named testdata or vendor below the given one; explicit files are
// taken whatever their extension. It returns the files to process and the
// loader targets.
func (t *tree) collect(paths []string) (files, targets []string, err error) {
	if len(paths) == 0 {
		paths = []string{"."}
	}
	seen := map[string]bool{}
	addTarget := func(p string) {
		if !seen[p] {
			seen[p] = true
			targets = append(targets, loadPath(p))
		}
	}
	for _, orig := range paths {
		p := path.Clean(orig)
		if orig == "" {
			return nil, nil, errors.New("no such file or folder: ")
		}
		if _, ok := t.files[p]; ok {
			files = append(files, p)
			addTarget(p)
			continue
		}
		if p != "." && !t.isDir(p) {
			return nil, nil, fmt.Errorf("no such file or folder: %s", orig)
		}
		prefix := ""
		if p != "." {
			prefix = p + "/"
		}
	keys:
		for _, k := range t.keys {
			if !strings.HasPrefix(k, prefix) {
				continue
			}
			segs := strings.Split(k[len(prefix):], "/")
			for _, s := range segs[:len(segs)-1] {
				if skipDir(s) {
					continue keys
				}
			}
			if strings.HasSuffix(k, ".synod") {
				files = append(files, k)
				addTarget(path.Dir(k))
			}
		}
	}
	if len(files) == 0 {
		return nil, nil, errors.New("no .synod files found")
	}
	return files, targets, nil
}

// compactJSON is not needed by callers outside this package; it trims the
// trailing newline of an encoder's output.
func compactJSON(v any) (string, error) {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return "", err
	}
	return strings.TrimSuffix(b.String(), "\n"), nil
}
