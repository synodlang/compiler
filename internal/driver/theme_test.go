package driver

import (
	"io/fs"
	"strings"
	"testing"

	"github.com/synodlang/compiler/internal/format"
	"github.com/synodlang/compiler/internal/ir"
	"github.com/synodlang/compiler/internal/stdlib"
	"github.com/synodlang/compiler/internal/types"
)

// themeSource is a complete theme named name: the defaults, with the
// overrides.
func themeSource(t *testing.T, name string, over map[string]string) string {
	t.Helper()
	data, err := fs.ReadFile(stdlib.Root(), "theme/default/default.synod")
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.Replace(string(data), "# theme default", "# theme "+name, 1), "\n")
	cur := ""
	for i, l := range lines {
		if strings.HasPrefix(l, "define ") {
			cur = strings.TrimPrefix(l, "define ")
		} else if strings.HasPrefix(l, "  result ") {
			if v, ok := over[cur]; ok {
				lines[i] = "  result " + v
			}
		}
	}
	return strings.Join(lines, "\n")
}

func programOf(t *testing.T, res Result, path string) *ir.Program {
	t.Helper()
	for _, d := range res.Diags {
		t.Errorf("%s", d)
	}
	p := res.Programs[path]
	if p == nil {
		t.Fatalf("no program %s", path)
	}
	return p
}

const themeMain = "# package main\nuse std/sim\n\ndefine main\n  sim.Seed 1\n"

// TestThemeValues: a main file that imports a theme gets its values in
// Program.Theme, in table order, with the types of the table; the theme is no
// package of the program; a main file that imports none gets the default.
func TestThemeValues(t *testing.T) {
	warm := themeSource(t, "warm", map[string]string{"Paper": "#2B1F1A", "Glow": "12", "NodeShape": `"circle"`, "Tracking": "-0.02"})
	res := checkMap(map[string]string{
		"warm/warm.synod": warm,
		"p/main.synod":    strings.Replace(themeMain, "use std/sim\n", "use example.com/t/warm\nuse std/sim\n", 1),
		"q/main.synod":    themeMain,
	}, "warm", "p", "q")
	p := programOf(t, res, "example.com/t/p#main")
	if p.Theme == nil || p.Theme.Path != "example.com/t/warm" || len(p.Theme.Values) != len(stdlib.ThemeNames) {
		t.Fatalf("theme = %+v", p.Theme)
	}
	for i, n := range stdlib.ThemeNames {
		v := p.Theme.Values[i]
		if v.Name != n.Name || !types.Equal(v.Type, n.Type) {
			t.Errorf("value %d is %s %s, table has %s %s", i, v.Name, v.Type, n.Name, n.Type)
		}
	}
	for name, want := range map[string]any{"Paper": "#2B1F1A", "Glow": 12.0, "NodeShape": "circle", "Tracking": -0.02, "Ink": "#EDE9E1", "Easing": "smooth"} {
		if v, ok := p.Theme.Get(name); !ok || v.V != want {
			t.Errorf("Get(%s) = %v, want %v", name, v.V, want)
		}
	}
	for _, pk := range p.Packages {
		if strings.Contains(pk.Path, "warm") || strings.HasPrefix(pk.Path, "std/theme/") {
			t.Errorf("the theme %s is a package of the program", pk.Path)
		}
	}
	q := programOf(t, res, "example.com/t/q#main")
	if q.Theme == nil || q.Theme.Path != stdlib.DefaultThemePath {
		t.Fatalf("a program without a theme has %+v", q.Theme)
	}
	if v, _ := q.Theme.Get("Paper"); v.V != "#151A2E" {
		t.Errorf("default Paper = %v", v.V)
	}
}

// TestThemeExportsNothing: terminal.Paper is unknown-phrase, and the import
// that gives a program its theme is used even when nothing names it.
func TestThemeExportsNothing(t *testing.T) {
	warm := themeSource(t, "warm", nil)
	res := checkMap(map[string]string{
		"warm/warm.synod": warm,
		"p/main.synod":    "# package main\nuse example.com/t/warm\nuse std/sim\n\ndefine main\n  sim.Seed warm.Paper\n",
	}, "warm", "p")
	if got := codes(res.Diags); !strings.Contains(got, "unknown-phrase") || strings.Contains(got, "unused-import") {
		t.Errorf("diagnostics = %q, want unknown-phrase and no unused-import", got)
	}
}

// TestDefaultThemeImportIsRemovedByFmt: importing only std/theme/default is
// non-canonical and the fix removes the line; importing it with another theme
// is extra-theme.
func TestDefaultThemeImportIsRemovedByFmt(t *testing.T) {
	main := "# package main\nuse std/sim\nuse std/theme/default\n\ndefine main\n  sim.Seed 1\n"
	res := checkMap(map[string]string{"p/main.synod": main}, "p")
	if got := codes(res.Diags); got != "non-canonical" {
		t.Fatalf("diagnostics = %q, want non-canonical", got)
	}
	for _, p := range res.Packages {
		for _, f := range p.Files {
			if fx := res.Fixes[f.Path]; len(fx) > 0 {
				out, d := format.FormatWith(f.Load.Src, fx)
				if d != nil || strings.Contains(string(out), "std/theme/default") || !strings.Contains(string(out), "use std/sim\n\ndefine main") {
					t.Errorf("fixed file:\n%s", out)
				}
				return
			}
		}
	}
	t.Error("no fix")
}

// TestThemeInLibrary: a theme import in a library file is theme-outside-main
// and stays out of the program.
func TestThemeInLibrary(t *testing.T) {
	res := checkMap(map[string]string{"p/p.synod": "# package p\nuse std/theme/default\n\ndefine One\n  result 1\n"}, "p")
	if got := codes(res.Diags); got != "theme-outside-main" {
		t.Errorf("diagnostics = %q", got)
	}
}
