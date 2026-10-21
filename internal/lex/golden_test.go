package lex

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/synodlang/compiler/internal/source"
)

var update = flag.Bool("update", false, "rewrite the golden files in testdata/lex")

// dump renders tokens one per line as `L:C KIND "text"`, followed by the
// flags `blank` and `own` when set.
func dump(toks []Token) string {
	var b strings.Builder
	for _, t := range toks {
		fmt.Fprintf(&b, "%d:%d %s %s", t.Pos.Line, t.Pos.Col, t.Kind, strconv.Quote(t.Text))
		if t.Blank {
			b.WriteString(" blank")
		}
		if t.Own {
			b.WriteString(" own")
		}
		b.WriteByte('\n')
	}
	return b.String()
}

// goldenFiles lists the source files of testdata/lex.
func goldenFiles(t testing.TB) []string {
	t.Helper()
	files, err := filepath.Glob(filepath.Join("..", "..", "testdata", "lex", "*.synod"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no golden inputs found: %v", err)
	}
	return files
}

// TestGolden lexes every testdata/lex/<name>.synod and compares the result
// with <name>.tokens, or with <name>.err when lexing must fail.
func TestGolden(t *testing.T) {
	for _, path := range goldenFiles(t) {
		name := strings.TrimSuffix(filepath.Base(path), ".synod")
		t.Run(name, func(t *testing.T) {
			src, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			toks, d := Lex(source.NewFile(name+".synod", src))
			base := strings.TrimSuffix(path, ".synod")
			got, ext, other := dump(toks), ".tokens", ".err"
			if d != nil {
				got, ext, other = d.String()+"\n", ".err", ".tokens"
			}
			if *update {
				if err := os.WriteFile(base+ext, []byte(got), 0o644); err != nil {
					t.Fatal(err)
				}
				if err := os.Remove(base + other); err != nil && !os.IsNotExist(err) {
					t.Fatal(err)
				}
				return
			}
			want, err := os.ReadFile(base + ext)
			if err != nil {
				t.Fatalf("missing golden (run with -update): %v", err)
			}
			if got != string(want) {
				t.Errorf("mismatch for %s%s\n--- got ---\n%s--- want ---\n%s", name, ext, got, want)
			}
			if _, err := os.Stat(base + other); err == nil {
				t.Errorf("stale golden %s%s", name, other)
			}
		})
	}
}

// TestErrGoldensFail makes sure files named err-* really are errors and the
// others are not, so a golden cannot silently change sides.
func TestErrGoldensFail(t *testing.T) {
	for _, path := range goldenFiles(t) {
		name := strings.TrimSuffix(filepath.Base(path), ".synod")
		src, _ := os.ReadFile(path)
		_, d := Lex(source.NewFile(name, src))
		if isErr := strings.HasPrefix(name, "err-"); isErr != (d != nil) {
			t.Errorf("%s: error = %v, want %v", name, d != nil, isErr)
		}
	}
}
