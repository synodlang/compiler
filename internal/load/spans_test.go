package load

import (
	"strings"
	"testing"

	"github.com/synodlang/compiler/internal/source"
)

// find returns the diagnostic whose message starts with msg.
func find(t *testing.T, ds []source.Diagnostic, msg string) source.Diagnostic {
	t.Helper()
	for _, d := range ds {
		if strings.HasPrefix(d.Message, msg) {
			return d
		}
	}
	t.Fatalf("no diagnostic %q in %v", msg, ds)
	return source.Diagnostic{}
}

func spanOf(d source.Diagnostic) [4]int {
	return [4]int{d.Line, d.Col, d.End.Line, d.End.Col}
}

// TestModSpans checks the spans, secondary labels and notes of the
// diagnostics of synod.mod. Every span starts at column 1, where the
// diagnostics have always pointed.
func TestModSpans(t *testing.T) {
	src := "module a.com/x\nmodule b.com/y\nsynod 0.9\nsynod 0.2\nrequire a.com/b v1.0.0\nrequire a.com/a v1.0.0\nrequire a.com/c\nbogus 1\n  require ! v1\n"
	_, ds := parseMod("synod.mod", []byte(src))
	tests := []struct {
		msg  string
		span [4]int
		also string
		note bool
		help bool
	}{
		{`a second "module"`, [4]int{2, 1, 2, 7}, "first here", false, false},
		{"module needs Synod", [4]int{3, 1, 3, 10}, "", true, false},
		{`a second "synod"`, [4]int{4, 1, 4, 6}, "first here", false, false},
		{"require lines are not sorted", [4]int{6, 1, 6, 23}, "", false, true},
		{`"require" takes`, [4]int{7, 1, 7, 16}, "", false, true},
		{"unknown directive", [4]int{8, 1, 8, 6}, "", false, true},
		{"bad module path", [4]int{9, 1, 9, 12}, "", false, false},
	}
	for _, tc := range tests {
		d := find(t, ds, tc.msg)
		if spanOf(d) != tc.span {
			t.Errorf("%s: span %v, want %v", tc.msg, spanOf(d), tc.span)
		}
		if tc.also != "" && (len(d.Labels) != 1 || d.Labels[0].Text != tc.also || d.Labels[0].File != "synod.mod" || d.Labels[0].Line != 1+map[string]int{`a second "module"`: 0, `a second "synod"`: 2}[tc.msg]) {
			t.Errorf("%s: labels %+v", tc.msg, d.Labels)
		}
		if (len(d.Notes) > 0) != tc.note || (len(d.Help) > 0) != tc.help {
			t.Errorf("%s: notes %v, help %v", tc.msg, d.Notes, d.Help)
		}
	}
	_, ds = parseMod("synod.mod", []byte("# nothing\n"))
	if d := find(t, ds, `missing "module"`); spanOf(d) != [4]int{1, 1, 1, 10} {
		t.Errorf("missing module: %v", spanOf(d))
	}
	_, ds = parseMod("synod.mod", nil)
	if d := find(t, ds, `missing "synod"`); spanOf(d) != [4]int{1, 1, 1, 1} {
		t.Errorf("empty file: %v", spanOf(d))
	}
}

// TestImportSpans checks the spans and edits of the import diagnostics, the
// labels of an import cycle, and the labels of a second main file.
func TestImportSpans(t *testing.T) {
	fsys := MapFS{
		"/m/synod.mod":      "module e.com/m\nsynod 0.2\n",
		"/m/a/a.synod":      "# package a\n\nuse e.com/m/b\nuse std/net@v1.0.0 as nn\nuse github.com/synodlang/std/net\n",
		"/m/b/b.synod":      "# package b\n\nuse e.com/m/c\n",
		"/m/c/c.synod":      "# package c\n\nuse e.com/m/a\n",
		"/m/d/main.synod":   "# package main\n",
		"/m/d/second.synod": "# package main\n",
	}
	std := stdFS(t)
	g, ds := Load(Config{FS: fsys, Std: std}, []string{"/m/a", "/m/d"})

	d := find(t, ds, `"std/net@v1.0.0" takes no version`)
	if spanOf(d) != [4]int{4, 5, 4, 19} {
		t.Errorf("version: %v", spanOf(d))
	}
	if len(d.Help) != 1 || len(d.Help[0].Edits) != 1 || d.Help[0].Edits[0].New != "std/net" || d.Help[0].Edits[0].Col != 5 || d.Help[0].Edits[0].End.Col != 19 {
		t.Errorf("version help: %+v", d.Help)
	}
	d = find(t, ds, `"github.com/synodlang/std/net" cannot`)
	if spanOf(d) != [4]int{5, 5, 5, 33} || len(d.Help) != 1 || d.Help[0].Edits[0].New != "std/net" {
		t.Errorf("std module path: %v %+v", spanOf(d), d.Help)
	}

	d = find(t, ds, "import cycle")
	if d.File != "/m/a/a.synod" || spanOf(d) != [4]int{3, 5, 3, 14} {
		t.Errorf("cycle: %s %v", d.File, spanOf(d))
	}
	if len(d.Labels) != 2 || d.Labels[0].File != "/m/b/b.synod" || d.Labels[0].Text != "b imports c here" ||
		d.Labels[1].File != "/m/c/c.synod" || d.Labels[1].Text != "c imports a here" || d.Labels[1].End.Col != 14 {
		t.Errorf("cycle labels: %+v", d.Labels)
	}

	d = find(t, ds, "a second main file")
	if d.File != "/m/d/second.synod" || spanOf(d) != [4]int{1, 1, 1, 15} || len(d.Labels) != 1 ||
		d.Labels[0].File != "/m/d/main.synod" || d.Labels[0].Text != "first main file" {
		t.Errorf("extra main: %+v", d)
	}
	if len(g.Extra) != 1 || g.Extra[0].Src.Path != "/m/d/second.synod" {
		t.Errorf("Extra: %v", g.Extra)
	}

	// The import records its spans.
	var imp *Import
	for _, p := range g.Packages {
		for _, i := range p.Imports {
			if i.Alias == "nn" {
				imp = i
			}
		}
	}
	if imp == nil {
		t.Fatal("no import with an alias")
	}
	if imp.PathSpan != (source.Span{Start: source.Pos{Line: 4, Col: 5}, End: source.Pos{Line: 4, Col: 19}}) ||
		imp.AliasSpan != (source.Span{Start: source.Pos{Line: 4, Col: 20}, End: source.Pos{Line: 4, Col: 25}}) ||
		imp.LineSpan != (source.Span{Start: source.Pos{Line: 4, Col: 1}, End: source.Pos{Line: 4, Col: 25}}) {
		t.Errorf("spans %+v", imp)
	}
}

// TestHeaderSpans checks the bad-header diagnostics that the loader adds.
func TestHeaderSpans(t *testing.T) {
	fsys := MapFS{
		"/m/synod.mod":     "module e.com/m\nsynod 0.2\n",
		"/m/Abc/Abc.synod": "# package Abc\n",
		"/m/use/use.synod": "# package use\n",
	}
	_, ds := Load(Config{FS: fsys, Std: stdFS(t)}, []string{"/m/Abc", "/m/use"})
	for _, msg := range []string{`package name "Abc"`, `package name "use"`} {
		d := find(t, ds, msg)
		if spanOf(d) != [4]int{1, 1, 1, 14} || len(d.Help) != 0 {
			t.Errorf("%s: %v %+v", msg, spanOf(d), d.Help)
		}
	}
}
