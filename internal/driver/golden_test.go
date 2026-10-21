package driver

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/synodlang/compiler/internal/load"
	"github.com/synodlang/compiler/internal/source"
)

var update = flag.Bool("update", false, "rewrite the expect.txt files of testdata/check")

// render is the text output of synod check for the diagnostics.
func render(ds []source.Diagnostic) string {
	var b strings.Builder
	for _, d := range ds {
		b.WriteString(d.String())
		b.WriteByte('\n')
	}
	return b.String()
}

// TestGolden runs every case of testdata/check/<code>/<case>/: a module
// with package folders, and expect.txt, the output of synod check with
// paths relative to the case. The case must report its folder's code.
func TestGolden(t *testing.T) {
	root := "../../testdata/check"
	codes, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, c := range codes {
		if !c.IsDir() {
			continue
		}
		cases, err := os.ReadDir(filepath.Join(root, c.Name()))
		if err != nil {
			t.Fatal(err)
		}
		for _, cs := range cases {
			if !cs.IsDir() {
				continue
			}
			dir := filepath.Join(root, c.Name(), cs.Name())
			code := source.Code(c.Name())
			t.Run(c.Name()+"/"+cs.Name(), func(t *testing.T) {
				n++
				res := checkDir(t, dir, false)
				var ds []source.Diagnostic
				found := false
				for _, d := range res.Diags {
					if strings.HasPrefix(d.File, "std/") {
						t.Errorf("diagnostic in std: %s", d)
						continue
					}
					ds = append(ds, d)
					found = found || d.Code == code
				}
				if !found {
					t.Errorf("no diagnostic with code %s in:\n%s", code, render(ds))
				}
				got := render(ds)
				path := filepath.Join(dir, "expect.txt")
				if *update {
					if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
						t.Fatal(err)
					}
					return
				}
				want, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				if string(want) != got {
					t.Errorf("output differs\n--- got\n%s--- want\n%s", got, want)
				}
				// The result is the same on a second run.
				if again := render(checkDir(t, dir, false).Diags); again != render(res.Diags) {
					t.Errorf("not deterministic")
				}
			})
		}
	}
	_ = load.StdModule
}
