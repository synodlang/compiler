package sim

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/synodlang/compiler/internal/trace"
)

// compactCase is the case whose trace is too large to keep (more than half a
// megabyte): its golden is the number of sim.Fired facts, the last fact and
// the SHA-256 of the encoded trace.
const compactCase = "every-ticks"

func compactSummary(r *Result, trace string) string {
	fired := 0
	for _, f := range r.Facts {
		if f.Ev == "sim.Fired" {
			fired++
		}
	}
	ls := strings.Split(strings.TrimSuffix(trace, "\n"), "\n")
	return fmt.Sprintf("facts %d\nfired %d\nlast %s\nsha256 %x\n", len(r.Facts), fired, ls[len(ls)-1], sha256.Sum256([]byte(trace)))
}

func TestCases(t *testing.T) { runDirs(t, "../../testdata/run/cases", false) }

func TestErrors(t *testing.T) { runDirs(t, "../../testdata/run/errors", true) }

// runDirs runs every case directory below root (one level for cases, two
// for errors: <code>/<case>). Each case is run twice and must give identical
// facts; the golden is expect.txt (the dump).
func runDirs(t *testing.T, root string, nested bool) {
	var dirs []string
	ents, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range ents {
		if !e.IsDir() {
			continue
		}
		if !nested {
			dirs = append(dirs, e.Name())
			continue
		}
		sub, err := os.ReadDir(filepath.Join(root, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		for _, s := range sub {
			if s.IsDir() {
				dirs = append(dirs, e.Name()+"/"+s.Name())
			}
		}
	}
	if len(dirs) == 0 {
		t.Fatal("no cases")
	}
	for _, d := range dirs {
		dir := filepath.Join(root, filepath.FromSlash(d))
		t.Run(d, func(t *testing.T) {
			prog, g := compileGraph(t, dir)
			r1 := Run(prog, Options{})
			r2 := Run(prog, Options{})
			g1, g2 := encode(t, prog, g, r1), encode(t, prog, g, r2)
			if g1 != g2 {
				t.Fatalf("not deterministic:\n%s\n---\n%s", g1, g2)
			}
			checkJSONL(t, g1)
			if nested {
				code := filepath.Dir(filepath.FromSlash(d))
				if r1.Failed == nil || string(r1.Failed.Code) != code {
					t.Fatalf("want failure %s, got %v", code, r1.Failed)
				}
				last := r1.Facts[len(r1.Facts)-1]
				if last.Ev != "sim.Failed" {
					t.Fatalf("last fact %s", last.Ev)
				}
			}
			if d == compactCase {
				checkGolden(t, filepath.Join(dir, "expect.txt"), compactSummary(r1, g1))
				return
			}
			checkGolden(t, filepath.Join(dir, "trace.jsonl"), g1)
			if nested {
				ls := strings.Split(strings.TrimSuffix(g1, "\n"), "\n")
				o := trace.Options{Paths: trace.FilePaths(g), Qual: trace.Qualifiers(prog)}
				out := fmt.Sprintf("%s\n%s\n", ls[len(ls)-1], trace.FailedLine(r1.Failed, r1.End, o))
				checkGolden(t, filepath.Join(dir, "expect.txt"), out)
			}
		})
	}
}

// checkJSONL checks that every line is a JSON object, that seq counts from 1
// and that sim.Failed, if present, is the last line.
func checkJSONL(t *testing.T, s string) {
	t.Helper()
	ls := strings.Split(strings.TrimSuffix(s, "\n"), "\n")
	for i, l := range ls {
		var m map[string]any
		if err := json.Unmarshal([]byte(l), &m); err != nil {
			t.Fatalf("line %d: %v: %s", i+1, err, l)
		}
		if i > 0 && m["seq"] != float64(i) {
			t.Fatalf("line %d: seq %v", i+1, m["seq"])
		}
		if m["ev"] == "sim.Failed" && i != len(ls)-1 {
			t.Fatalf("sim.Failed is not last")
		}
	}
}
