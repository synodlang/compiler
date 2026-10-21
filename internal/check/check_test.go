package check_test

import (
	"strings"
	"testing"

	"github.com/synodlang/compiler/internal/driver"
	"github.com/synodlang/compiler/internal/ir"
	"github.com/synodlang/compiler/internal/load"
	"github.com/synodlang/compiler/internal/types"
)

func run(t *testing.T, files map[string]string, targets ...string) driver.Result {
	t.Helper()
	m := load.MapFS{"/m/synod.mod": "module example.com/t\nsynod 0.2\n"}
	for p, c := range files {
		m["/m/"+p] = c
	}
	var ts []string
	for _, tg := range targets {
		ts = append(ts, "/m/"+tg)
	}
	return driver.Check(driver.Options{Load: load.Config{FS: m}}, ts)
}

func fieldTypes(res driver.Result, pkg string) map[string]string {
	out := map[string]string{}
	for _, rp := range res.Packages {
		if rp.Name != pkg || rp.IsStd {
			continue
		}
		in := res.Infos[rp.Path]
		if rp.Node != nil {
			for _, f := range rp.Node.Fields {
				out[f.Name] = types.Format(in.FieldTy[f], rp.Path, nil)
			}
		}
		for _, r := range rp.Records {
			for _, f := range r.Fields {
				out[r.Name+"."+f.Name] = types.Format(in.FieldTy[f], rp.Path, nil)
			}
		}
	}
	return out
}

func TestInference(t *testing.T) {
	res := run(t, map[string]string{"a/a.synod": `# package a
use std/lists
use std/maps

Item has
  set tags to {}
  set weight to 0

Node has
  set queue to {}
  set index to {}
  set slots to {}
  set last to (message Item with weight 1)
  set total to 0

define [n: Node] Go [k: text] and [m: node]
  set n.queue to (lists.Append (message Item with tags {k}) to n.queue)
  set item k of n.index to {}
  set item k of n.index to (lists.Append m to (item k of n.index))
  set item 3 of n.slots to k
  set n.total to (maps.Size n.index)
  set n.last to (item 0 of n.queue)
`}, "a")
	if len(res.Diags) > 0 {
		t.Fatalf("%s", res.Diags[0])
	}
	want := map[string]string{
		"Item.tags":   "list of text",
		"queue":       "list of Item",
		"index":       "map of text to list of node",
		"slots":       "list of text",
		"last":        "Item",
		"total":       "number",
		"Item.weight": "number",
	}
	got := fieldTypes(res, "a")
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s: got %q, want %q", k, got[k], v)
		}
	}
}

func TestMapDecidedByUse(t *testing.T) {
	// A number key does not decide list or map; maps.Without does.
	res := run(t, map[string]string{"a/a.synod": `# package a
use std/maps

Node has
  set pending to {}
  set other to {}

define [n: Node] Go [id: number]
  set item id of n.pending to "x"
  set n.pending to (maps.Without id from n.pending)
  set item id of n.other to "y"
`}, "a")
	if len(res.Diags) > 0 {
		t.Fatalf("%s", res.Diags[0])
	}
	got := fieldTypes(res, "a")
	if got["pending"] != "map of number to text" || got["other"] != "list of text" {
		t.Errorf("got %v", got)
	}
}

func TestDeterministic(t *testing.T) {
	files := map[string]string{"a/a.synod": `# package a
Node has
  set x to 0

define [n: Node] Go [t: text]
  set n.x to t
  set n.x to {}
`}
	a, b := run(t, files, "a"), run(t, files, "a")
	if len(a.Diags) == 0 || len(a.Diags) != len(b.Diags) {
		t.Fatalf("diagnostics: %d and %d", len(a.Diags), len(b.Diags))
	}
	for i := range a.Diags {
		if a.Diags[i] != b.Diags[i] {
			t.Errorf("differs at %d", i)
		}
	}
	if a.Programs != nil {
		t.Errorf("programs lowered despite errors")
	}
}

func TestLowerLibraryAndMain(t *testing.T) {
	files := map[string]string{
		"a/a.synod": `# package a
use std/sim

Ping has
  set seq to 0

Node has
  set count to 0

define [n: Node] Bump
  set n.count to (n.count plus 1)

when [n: Node] (sim.Delivered Ping)
  n.Bump
`,
		"a/main.synod": `# package main
use example.com/t/a
use std/sim

define main
  set n to (sim.Spawn a)
  deliver (message a.Ping with seq 1) to n
`}
	res := run(t, files, "a")
	if len(res.Diags) > 0 {
		t.Fatalf("%s", res.Diags[0])
	}
	lib := res.Programs["example.com/t/a"]
	mainp := res.Programs["example.com/t/a#main"]
	if lib == nil || mainp == nil || lib.Main != nil || mainp.Main == nil {
		t.Fatalf("programs: %v", res.Programs)
	}
	var pkg *ir.Package
	for _, p := range mainp.Packages {
		if p.Path == "example.com/t/a" {
			pkg = p
		}
	}
	if pkg == nil || len(pkg.NodeFields) != 1 || len(pkg.NodeHandlers) != 1 || len(pkg.Phrases) != 1 {
		t.Fatalf("package: %+v", pkg)
	}
	h := pkg.NodeHandlers[0]
	if h.Event != "sim.delivered" || !h.NodeEvent || h.Receiver == nil || h.Slots != len(h.Vars) {
		t.Errorf("handler: %+v", h)
	}
	if !strings.Contains(ir.Dump(mainp), `(deliver (message a.Ping (seq 1)) n#0)`) {
		t.Errorf("dump:\n%s", ir.Dump(mainp))
	}
	if again := run(t, files, "a"); ir.Dump(again.Programs["example.com/t/a#main"]) != ir.Dump(mainp) {
		t.Errorf("dump is not deterministic")
	}
	// A package that does not check lowers to nothing.
	bad := run(t, map[string]string{"a/a.synod": "# package a\nNode has\n  set x to 0\n\ndefine [n: Node] Go\n  set n.x to \"t\"\n"}, "a")
	if len(bad.Diags) == 0 || bad.Programs != nil {
		t.Errorf("want diagnostics and no programs")
	}
}
