package driver

import (
	"strings"
	"testing"

	"github.com/synodlang/compiler/internal/load"
	"github.com/synodlang/compiler/internal/source"
)

// Two module folders under a folder without synod.mod declare the same
// module path ("synod check" with no arguments there). Their packages have
// the same import path, so they cannot be checked together: the driver
// reports one diagnostic that names both folders and never lowers (it used
// to panic while lowering the second package with the first one's Info).
func TestTwoModulesWithOnePath(t *testing.T) {
	fsys := load.MapFS{
		"/w/counter/synod.mod": `module counter
synod 0.3
`,
		"/w/counter/counter/counter.synod": `# package counter
use std/time

Node has
  set Count to 0

define [n: Node] start
  time.Every 1s
    set n.Count to (n.Count plus 1)
`,
		"/w/counter/demo/main.synod": `# package main
use counter/counter
use std/sim

define main
  sim.Seed 1
  set nodes to (sim.Cluster of 3 running counter)
  after 2s
    sim.Crash (item 0 of nodes)
  sim.StopAt 5s
`,
		"/w/proj/counter/synod.mod": `module counter
synod 0.3
`,
		"/w/proj/counter/counter/counter.synod": `# package counter
use std/time

Node has
  set Count to 0

# Count up once a second
define [n: Node] start
  time.Every 1s
    set n.Count to (n.Count plus 1)
`,
		"/w/proj/counter/counter/demo.synod": `# package main
use counter/counter
use std/sim

define main
  set first to (sim.Spawn counter)
  set second to (sim.Spawn counter)
  set third to (sim.Spawn counter)
  set nodes to {first, second, third}
  after 2s
    sim.Crash (item 1 of nodes)
  sim.StopAt 5s
`,
	}
	res := Check(Options{Load: load.Config{FS: fsys}}, []string{"/w/counter/counter", "/w/counter/demo", "/w/proj/counter/counter"})
	if len(res.Diags) != 1 {
		t.Fatalf("diagnostics = %v, want one", res.Diags)
	}
	d := res.Diags[0]
	if d.Code != source.DuplicateName || d.File != "/w/proj/counter/synod.mod" ||
		!strings.Contains(d.Message, `module "counter" is declared by two folders, /w/counter and /w/proj/counter`) {
		t.Errorf("diagnostic = %v", d)
	}
	if !strings.Contains(d.Hint, "synod check /w/proj/counter") {
		t.Errorf("hint = %q", d.Hint)
	}
	if res.Programs != nil {
		t.Errorf("a program was lowered")
	}
}
