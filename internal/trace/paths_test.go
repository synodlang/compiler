package trace

import (
	"path"
	"reflect"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/synodlang/compiler/internal/ir"
	"github.com/synodlang/compiler/internal/load"
)

func stdTree() fstest.MapFS {
	return fstest.MapFS{
		"synod.mod":     {Data: []byte("module github.com/synodlang/std\nsynod 0.2\n")},
		"net/net.synod": {Data: []byte("# package net\n")},
	}
}

func tree(root string) load.MapFS {
	return load.MapFS{
		root + "/synod.mod": "module example.com/t\nsynod 0.2\nrequire github.com/other/raft v1.2.0\n",
		root + "/a/a.synod": "# package a\nuse std/net\nuse github.com/other/raft\n",
		root + "/vendor/github.com/other/raft@v1.2.0/synod.mod":     "module github.com/other/raft\nsynod 0.2\n",
		root + "/vendor/github.com/other/raft@v1.2.0/raft.synod":    "# package raft\n",
		root + "/vendor/github.com/other/raft@v1.2.0/sub/sub.synod": "# package sub\n",
	}
}

func loadWith(t *testing.T, root, cwd string) *load.Graph {
	t.Helper()
	cfg := load.Config{FS: tree(root), Std: stdTree(), Display: func(p string) string {
		if p == cwd {
			return "."
		}
		if strings.HasPrefix(p, cwd+"/") {
			return p[len(cwd)+1:]
		}
		// Parent of the root: "/w" with root "/w/m" and cwd "/w".
		return path.Clean(p)
	}}
	g, ds := load.Load(cfg, []string{root + "/a"})
	if len(ds) != 0 {
		t.Fatalf("diagnostics: %v", ds)
	}
	return g
}

func TestFilePathsIndependentOfCwd(t *testing.T) {
	want := map[string]bool{
		"a/a.synod": true, "std/net/net.synod": true,
		"github.com/other/raft@v1.2.0/raft.synod": true,
	}
	var first []string
	for _, cwd := range []string{"/w/m", "/w/m/a", "/w", "/elsewhere"} {
		g := loadWith(t, "/w/m", cwd)
		m := FilePaths(g)
		var vals []string
		for disp, tp := range m {
			if !want[tp] {
				t.Errorf("cwd %s: unexpected trace path %q for %q", cwd, tp, disp)
			}
			vals = append(vals, tp)
		}
		sortStrings(vals)
		if first == nil {
			first = vals
		} else if !reflect.DeepEqual(first, vals) {
			t.Errorf("cwd %s: %v != %v", cwd, vals, first)
		}
		if len(vals) != 3 {
			t.Errorf("cwd %s: %d paths: %v", cwd, len(vals), m)
		}
	}
}

func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}

func TestFilePathsDisplayKeys(t *testing.T) {
	g := loadWith(t, "/w/m", "/w/m/a")
	m := FilePaths(g)
	if m["a.synod"] != "a/a.synod" {
		t.Errorf("display key a.synod: %v", m)
	}
	g = loadWith(t, "/w/m", "/w")
	if FilePaths(g)["m/a/a.synod"] != "a/a.synod" {
		t.Errorf("parent cwd: %v", FilePaths(g))
	}
}

func TestFilePathsNoModule(t *testing.T) {
	cfg := load.Config{FS: load.MapFS{"/w/demo/x.synod": "# package demo\n"}}
	g, ds := load.Load(cfg, []string{"/w/demo"})
	if len(ds) != 0 {
		t.Fatal(ds)
	}
	m := FilePaths(g)
	if len(m) != 1 || m["/w/demo/x.synod"] != "demo/x.synod" {
		t.Errorf("got %v", m)
	}
}

func TestPosAndRule(t *testing.T) {
	o := Options{Paths: map[string]string{"x.synod": "p/x.synod"}}
	if got := o.pos(ir.Pos{File: "x.synod", Line: 3, Col: 4}); got != "p/x.synod:3:4" {
		t.Error(got)
	}
	if got := o.pos(ir.Pos{File: "other", Line: 1, Col: 1}); got != "other:1:1" {
		t.Error(got)
	}
	if got := o.rule("x.synod:9: always A", ir.Pos{File: "x.synod"}); got != "p/x.synod:9: always A" {
		t.Error(got)
	}
}
