package driver

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/synodlang/compiler/internal/format"
	"github.com/synodlang/compiler/internal/load"
	"github.com/synodlang/compiler/internal/source"
)

// checkMap checks the module held in files (paths below /m).
func checkMap(files map[string]string, targets ...string) Result {
	m := load.MapFS{"/m/synod.mod": "module example.com/t\nsynod 0.2\n"}
	for p, c := range files {
		m["/m/"+p] = c
	}
	var ts []string
	for _, t := range targets {
		ts = append(ts, "/m/"+t)
	}
	return Check(Options{Load: load.Config{FS: m}}, ts)
}

func codes(ds []source.Diagnostic) string {
	var out []string
	for _, d := range ds {
		out = append(out, string(d.Code))
	}
	return strings.Join(out, " ")
}

// TestResolveStd resolves every std package with no diagnostics.
func TestResolveStd(t *testing.T) {
	uses := ""
	for _, p := range []string{"disk", "lesson", "lists", "look", "look/basic", "maps", "math", "net", "random", "sim", "text", "theme", "time"} {
		uses += "use std/" + p + "\n"
	}
	res := checkMap(map[string]string{"a/a.synod": "# package a\n" + uses + "\ndefine One\n  result 1\n"}, "a")
	n := 0
	for _, d := range res.Diags {
		if strings.HasPrefix(d.File, "std/") {
			t.Errorf("%s", d)
		}
	}
	for _, p := range res.Packages {
		if p.IsStd {
			n++
		}
	}
	if n < 13 {
		t.Errorf("only %d std packages resolved", n)
	}
}

// TestContextDiagnostics covers the context errors the resolver reports.
func TestContextDiagnostics(t *testing.T) {
	tests := []struct {
		name, src, want string
	}{
		{"file root", "# package a\n\nset x to 1\n", "context"},
		{"declaration in block", "# package a\n\ndefine Go\n  define Foo\n    set x to 1\n", "context"},
		{"result in main", "# package main\n\ndefine main\n  result 1\n", "context"},
		{"result in after", "# package a\n\ndefine Go\n  after 1s\n    result 1\n  result 2\n", "context"},
		{"non-constant initial", "# package a\n\nNode has\n  set x to time.Now\n", "unknown-phrase"},
		{"constant initial call", "# package a\nuse std/time\n\nNode has\n  set x to time.Now\n", "context"},
		{"has with other line", "# package a\n\nNode has\n  if true\n    set x to 1\n", "context"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			file := "a/a.synod"
			if strings.Contains(tc.src, "package main") {
				file = "a/main.synod"
			}
			res := checkMap(map[string]string{file: tc.src}, "a")
			var got []source.Diagnostic
			for _, d := range res.Diags {
				if !strings.HasPrefix(d.File, "std/") {
					got = append(got, d)
				}
			}
			found := false
			for _, d := range got {
				found = found || string(d.Code) == tc.want
			}
			if !found {
				t.Errorf("want %s, got %q:\n%s", tc.want, codes(got), render(got))
			}
		})
	}
}

// TestFixesRoundTrip applies the fixes of the non-canonical cases and checks
// that the result is canonical and resolves cleanly.
func TestFixesRoundTrip(t *testing.T) {
	cases := []string{"settings-order", "own-qualifier", "names", "default-theme"}
	for _, c := range cases {
		dir := filepath.Join("../../testdata/check/non-canonical", c)
		res := checkDir(t, dir, false)
		files := map[string]string{}
		for _, p := range res.Packages {
			for _, f := range p.Files {
				fx := res.Fixes[f.Path]
				if len(fx) == 0 {
					continue
				}
				out, d := format.FormatWith(f.Load.Src, fx)
				if d != nil {
					t.Fatalf("%s: %s", f.Path, d)
				}
				files[f.Path] = string(out)
			}
		}
		if len(files) == 0 {
			t.Errorf("%s: no fixes", c)
			continue
		}
		// Re-check a copy of the case with the fixed files.
		mods := map[string]string{}
		err := filepath.Walk(dir, func(p string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() || filepath.Base(p) == "expect.txt" {
				return err
			}
			rel, _ := filepath.Rel(dir, p)
			b, err := os.ReadFile(p)
			mods[filepath.ToSlash(rel)] = string(b)
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
		for p, s := range files {
			mods[p] = s
		}
		var targets []string
		for p := range mods {
			if strings.HasSuffix(p, ".synod") {
				targets = append(targets, filepath.ToSlash(filepath.Dir(p)))
			}
		}
		again := checkMap(mods, targets...)
		for _, d := range again.Diags {
			if d.Code == source.NonCanonical {
				// names with no free lower-case name keep their error.
				if c == "names" && strings.Contains(d.File, "/taken/") {
					continue
				}
				if c == "attachment" {
					continue
				}
				t.Errorf("%s: still %s", c, d)
			}
		}
	}
}

// TestResolvedShape spot-checks the resolved tables of the examples.
func TestResolvedShape(t *testing.T) {
	res := checkDir(t, "../../testdata/examples", false)
	by := map[string]int{}
	for _, p := range res.Packages {
		by[p.Name+"/handlers"] += len(p.Handlers)
		by[p.Name+"/inv"] += len(p.Invariants)
	}
	if by["raft/handlers"] != 3 || by["raft/inv"] != 1 || by["paxos/handlers"] != 5 || by["raftlook/handlers"] != 2 {
		t.Errorf("unexpected tables: %v", by)
	}
	for _, p := range res.Packages {
		if p.Name != "cassandra" {
			continue
		}
		if p.Node == nil || len(p.Node.Fields) != 8 || p.Start == nil {
			t.Errorf("cassandra tables wrong")
		}
		if len(p.Records) != 9 {
			t.Errorf("cassandra has %d records", len(p.Records))
		}
		toks := p.Tokens[p.Files[0].Load]
		if len(toks) < 100 {
			t.Errorf("only %d tokens", len(toks))
		}
		for _, h := range p.Handlers {
			if h.Receiver == nil || h.Body == nil {
				t.Errorf("handler without receiver or body")
			}
		}
	}
	for _, d := range res.Diags {
		if !t4Codes[d.Code] {
			t.Errorf("%s", d)
		}
	}
}
