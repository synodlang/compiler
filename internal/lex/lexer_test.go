package lex

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/synodlang/compiler/internal/source"
)

// TestExamplesLex checks that every vendored example lexes cleanly.
func TestExamplesLex(t *testing.T) {
	root := filepath.Join("..", "..", "testdata", "examples")
	n := 0
	err := filepath.WalkDir(root, func(path string, e fs.DirEntry, err error) error {
		if err != nil || e.IsDir() || !strings.HasSuffix(path, ".synod") {
			return err
		}
		n++
		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if _, d := Lex(source.NewFile(path, src)); d != nil {
			t.Errorf("%s", d)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if n < 20 {
		t.Errorf("only %d examples found under %s", n, root)
	}
}
