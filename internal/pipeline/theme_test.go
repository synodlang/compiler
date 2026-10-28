package pipeline

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/synodlang/compiler/internal/driver"
	"github.com/synodlang/compiler/internal/format"
	"github.com/synodlang/compiler/internal/ir"
	"github.com/synodlang/compiler/internal/load"
	"github.com/synodlang/compiler/internal/sim"
	"github.com/synodlang/compiler/internal/source"
	"github.com/synodlang/compiler/internal/stdlib"
	"github.com/synodlang/compiler/internal/trace"
)

func copyTree(t *testing.T, from, to string) {
	t.Helper()
	err := filepath.WalkDir(from, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(from, p)
		dst := filepath.Join(to, rel)
		if d.IsDir() {
			return os.MkdirAll(dst, 0o755)
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		return os.WriteFile(dst, b, 0o644)
	})
	if err != nil {
		t.Fatal(err)
	}
}

// traceOf checks the folder and runs its program without drawing it, and
// returns the trace and the path of the program's theme.
func traceOf(t *testing.T, folder string) (tr []byte, themePath string) {
	t.Helper()
	res := driver.Check(driver.Options{Load: load.Config{FS: load.OSFS{}}, DefaultLook: true}, []string{filepath.ToSlash(folder)})
	if diags := UserDiags(res); len(diags) > 0 {
		t.Fatalf("%s: %s", folder, diags[0])
	}
	var prog *ir.Program
	for _, p := range res.Graph.Targets {
		if p.IsMain {
			prog = res.Programs[p.Path]
		}
	}
	if prog == nil || prog.Theme == nil {
		t.Fatalf("%s: no program or no theme", folder)
	}
	r := sim.Run(prog, sim.Options{})
	opts := trace.Options{Paths: trace.FilePaths(res.Graph), Qual: trace.Qualifiers(prog)}
	var buf bytes.Buffer
	if err := trace.Encode(&buf, r.Header, r.Facts, opts); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes(), prog.Theme.Path
}

// TestThemeNeverChangesRun: a theme is look-context only, so every example
// run under each fixture theme gives a trace byte-identical to the run under
// the default theme (spec/06 section 6.7, decision 114).
func TestThemeNeverChangesRun(t *testing.T) {
	tmp := t.TempDir()
	ex := filepath.Join(tmp, "examples")
	copyTree(t, abs2(t, "../../testdata/examples"), ex)
	themes, err := os.ReadDir(abs2(t, "../../testdata/themes"))
	if err != nil || len(themes) < 3 {
		t.Fatalf("themes: %v, %d", err, len(themes))
	}
	for _, th := range themes {
		copyTree(t, abs2(t, "../../testdata/themes/"+th.Name()), filepath.Join(ex, "themes", th.Name()))
	}
	var mains []string
	filepath.WalkDir(ex, func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() && strings.HasSuffix(p, ".synod") {
			if b, _ := os.ReadFile(p); bytes.HasPrefix(b, []byte("# package main\n")) {
				mains = append(mains, p)
			}
		}
		return nil
	})
	if len(mains) < 9 {
		t.Fatalf("only %d example mains", len(mains))
	}
	for _, main := range mains {
		orig, err := os.ReadFile(main)
		if err != nil {
			t.Fatal(err)
		}
		folder := filepath.Dir(main)
		name, _ := filepath.Rel(ex, folder)
		t.Run(filepath.ToSlash(name), func(t *testing.T) {
			base, path := traceOf(t, folder)
			if path != stdlib.DefaultThemePath {
				t.Fatalf("a program that imports no theme has the theme %q", path)
			}
			for _, th := range themes {
				imp := "github.com/synodlang/examples/themes/" + th.Name()
				src := strings.Replace(string(orig), "# package main\n", "# package main\nuse "+imp+"\n", 1)
				out, d := format.Format(source.NewFile(main, []byte(src)))
				if d != nil {
					t.Fatal(d)
				}
				if err := os.WriteFile(main, out, 0o644); err != nil {
					t.Fatal(err)
				}
				got, path := traceOf(t, folder)
				if path != imp {
					t.Errorf("theme %s: the program's theme is %q", th.Name(), path)
				}
				if !bytes.Equal(got, base) {
					t.Errorf("theme %s changes the trace of %s", th.Name(), name)
				}
			}
			if err := os.WriteFile(main, orig, 0o644); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func abs2(t *testing.T, p string) string {
	t.Helper()
	a, err := filepath.Abs(p)
	if err != nil {
		t.Fatal(err)
	}
	return a
}
