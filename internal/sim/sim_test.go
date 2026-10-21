package sim

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/synodlang/compiler/internal/source"
)

const everyLib = `use std/time

Node has
  set Ticks to 0

define [n: Node] start
  time.Every 1ms
    set n.Ticks to (n.Ticks plus 1)`

const everyMain = `use example.com/t/p
use std/sim

define main
  set a to (sim.Spawn p)
  set a.p.Ticks to 0`

// TestEveryEtaReduction runs time.Every 1ms until the 10-minute limit: 600,000
// nested ticks must not build nested closures.
func TestEveryEtaReduction(t *testing.T) {
	prog := compileSrc(t, everyMain, everyLib)
	start := time.Now()
	r := Run(prog, Options{})
	d := time.Since(start)
	if r.Failed == nil || r.Failed.Code != source.TimeLimit {
		t.Fatalf("want time-limit, got %+v (end %s)", r.Failed, r.EndReason)
	}
	if d > 5*time.Second {
		t.Errorf("took %s", d)
	}
	if r.End != 600_000_000 {
		t.Errorf("end %d", r.End)
	}
}

func TestSeedOption(t *testing.T) {
	prog := compileSrc(t, `use example.com/t/p
use std/sim

define main
  sim.Seed 5
  deliver (message p.Tick with k 1) to (sim.Spawn p)`, `use std/random
use std/sim

Tick has
  set k to 0

Node has
  set X to 0

when [n: Node] (sim.Delivered Tick)
  set n.X to (random.Between 0 and 1000)`)
	r := Run(prog, Options{})
	if r.Header.Seed != 5 {
		t.Errorf("seed %d", r.Header.Seed)
	}
	seven := uint64(7)
	r7 := Run(prog, Options{Seed: &seven})
	if r7.Header.Seed != 7 {
		t.Errorf("seed %d", r7.Header.Seed)
	}
	if dump(r) == dump(r7) {
		t.Errorf("seed 7 gave the facts of seed 5")
	}
	if dump(r) != dump(Run(prog, Options{})) {
		t.Errorf("not deterministic")
	}
}

func TestMaxSteps(t *testing.T) {
	prog := compileSrc(t, everyMain, everyLib)
	r := Run(prog, Options{MaxSteps: 50})
	if r.Failed == nil || r.Failed.Code != source.RunLimit {
		t.Fatalf("got %+v", r.Failed)
	}
	if last := r.Facts[len(r.Facts)-1]; last.Ev != "sim.Failed" {
		t.Errorf("last fact %s", last.Ev)
	}
}

// TestNoForbiddenImports keeps the simulator portable and deterministic.
func TestNoForbiddenImports(t *testing.T) {
	files, _ := filepath.Glob("*.go")
	bad := map[string]bool{"os": true, "time": true, "math/rand": true, "sync": true}
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		src, _ := os.ReadFile(f)
		if strings.Contains(string(src), "go func") {
			t.Errorf("%s starts a goroutine", f)
		}
		af, err := parser.ParseFile(token.NewFileSet(), f, src, parser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}
		for _, im := range af.Imports {
			if bad[strings.Trim(im.Path.Value, `"`)] {
				t.Errorf("%s imports %s", f, im.Path.Value)
			}
		}
	}
}

// TestExamplesSmoke runs every main program of the vendored examples twice:
// no run-time error and identical facts.
func TestExamplesSmoke(t *testing.T) {
	progs, g := compileAll(t, "../../testdata/examples")
	if len(progs) < 9 {
		t.Fatalf("%d programs", len(progs))
	}
	var keys []string
	for k := range progs {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		t.Run(k, func(t *testing.T) {
			p := progs[k]
			r1 := Run(p, Options{})
			if r1.Failed != nil {
				t.Fatalf("failed: %v", r1.Failed)
			}
			if encode(t, p, g, r1) != encode(t, p, g, Run(p, Options{})) {
				t.Errorf("not deterministic")
			}
			t.Logf("%d facts, %d steps, end %d", len(r1.Facts), len(r1.Steps), r1.End)
		})
	}
}
