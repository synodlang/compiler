package format

import (
	"flag"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/synodlang/compiler/internal/source"
)

var update = flag.Bool("update", false, "rewrite the golden files in testdata/format")

var testdata = filepath.Join("..", "..", "testdata")

// walkSynod calls fn for every .synod file below dir; it fails the test when
// there is none.
func walkSynod(t testing.TB, dir string, fn func(path string, src []byte)) {
	t.Helper()
	n := 0
	err := filepath.WalkDir(dir, func(path string, e fs.DirEntry, err error) error {
		if err != nil || e.IsDir() || !strings.HasSuffix(path, ".synod") {
			return err
		}
		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		n++
		fn(path, src)
		return nil
	})
	if err != nil || n == 0 {
		t.Fatalf("no .synod files below %s: %v", dir, err)
	}
}

// golden compares got with the file path, or rewrites it under -update.
func golden(t *testing.T, path string, got []byte) {
	t.Helper()
	if *update {
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v (run with -update)", err)
	}
	if string(want) != string(got) {
		t.Errorf("%s differs (run with -update)\n--- want\n%s--- got\n%s", path, want, got)
	}
}

// must formats src and fails the test on a diagnostic.
func must(t testing.TB, path string, src []byte) []byte {
	t.Helper()
	out, d := Format(source.NewFile(path, src))
	if d != nil {
		t.Fatalf("%s: %s", path, d)
	}
	return out
}

func mustRead(t testing.TB, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
