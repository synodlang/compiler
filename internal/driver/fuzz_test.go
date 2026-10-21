package driver

import (
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/synodlang/compiler/internal/load"
	"github.com/synodlang/compiler/internal/source"
)

// fuzzModule builds the in-memory module the fuzzer checks: the input as a
// library file, as a main file, and each of the two again behind a header,
// so that the later stages are reached too.
func fuzzModule(src string) load.MapFS {
	return load.MapFS{
		"/m/synod.mod":    "module example.com/fz\nsynod 0.2\n",
		"/m/a/a.synod":    src,
		"/m/b/main.synod": src,
		"/m/c/c.synod":    "# package c\n" + src,
		"/m/d/main.synod": "# package main\n" + src,
	}
}

func fuzzCheck(fsys load.MapFS) []source.Diagnostic {
	targets, err := load.Discover(fsys, "/m")
	if err != nil {
		return nil
	}
	return Check(Options{Load: load.Config{FS: fsys}}, targets).Diags
}

// FuzzCheck checks that the whole pipeline never panics, gives the same
// result twice, and returns sorted diagnostics.
func FuzzCheck(f *testing.F) {
	for _, dir := range []string{"examples", "check", "format"} {
		root := filepath.Join("..", "..", "testdata", dir)
		filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
			if err == nil && !d.IsDir() && strings.HasSuffix(p, ".synod") {
				if src, err := os.ReadFile(p); err == nil && len(src) <= 64<<10 {
					f.Add(string(src))
				}
			}
			return nil
		})
	}
	f.Fuzz(func(t *testing.T, src string) {
		if len(src) > 64<<10 {
			t.Skip()
		}
		fsys := fuzzModule(src)
		a := fuzzCheck(fsys)
		b := fuzzCheck(fsys)
		if !reflect.DeepEqual(a, b) {
			t.Fatalf("two runs differ:\n%s\n---\n%s", render(a), render(b))
		}
		sorted := append([]source.Diagnostic(nil), a...)
		source.Sort(sorted)
		if !reflect.DeepEqual(a, sorted) {
			t.Fatalf("diagnostics not sorted:\n%s", render(a))
		}
		for _, d := range a {
			if d.Line < 1 || d.Col < 1 || d.Message == "" {
				t.Fatalf("malformed diagnostic %+v", d)
			}
		}
	})
}
