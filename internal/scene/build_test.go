package scene

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/synodlang/compiler/internal/ir"
	"github.com/synodlang/compiler/internal/stdlib"
	"github.com/synodlang/compiler/internal/trace"
	"github.com/synodlang/compiler/internal/types"
)

// checkScene verifies the invariants every scene obeys (plan 5.1 and 5.5):
// valid references, strictly increasing keys, known colors, slots in order,
// sorted markers, and a valid scene.json.
func checkScene(t testing.TB, sc *trace.Scene) {
	t.Helper()
	ids := map[string]bool{}
	for _, s := range sc.Shapes {
		if ids[s.ID] {
			t.Errorf("duplicate shape id %s", s.ID)
		}
		ids[s.ID] = true
	}
	theme := map[string]bool{}
	for _, c := range sc.Theme {
		theme[c.Name] = true
	}
	for _, s := range sc.Shapes {
		switch s.Kind {
		case trace.KindLine:
			if !ids[s.From] || !ids[s.To] {
				t.Errorf("line %s refers to a missing shape", s.ID)
			}
		case trace.KindGroup:
			for _, m := range s.Items {
				if !ids[m] {
					t.Errorf("group %s refers to a missing shape %s", s.ID, m)
				}
			}
		case trace.KindCircle, trace.KindRect, trace.KindLabel, trace.KindIcon, trace.KindCounter:
		default:
			t.Errorf("shape %s has kind %q", s.ID, s.Kind)
		}
		for i, k := range s.Keys {
			if i > 0 && k.At <= s.Keys[i-1].At {
				t.Errorf("shape %s: keys not strictly increasing at %d", s.ID, k.At)
			}
			if k.Set&trace.PFill != 0 && !theme[k.Fill] {
				t.Errorf("shape %s: fill %q is not in the theme", s.ID, k.Fill)
			}
			if k.Set&trace.PStroke != 0 && !theme[k.Stroke] {
				t.Errorf("shape %s: stroke %q is not in the theme", s.ID, k.Stroke)
			}
			if k.At > sc.Duration {
				t.Errorf("shape %s: key at %d after the duration %d", s.ID, k.At, sc.Duration)
			}
		}
		if s.HasEnd && s.End > sc.Duration {
			t.Errorf("shape %s ends at %d after the duration %d", s.ID, s.End, sc.Duration)
		}
	}
	for i, e := range sc.Slots {
		if e.Seq != i+1 {
			t.Errorf("slot %d has seq %d", i, e.Seq)
		}
		if i > 0 && e.At < sc.Slots[i-1].At {
			t.Errorf("slot %d goes back in time", i)
		}
	}
	for i := 1; i < len(sc.Markers); i++ {
		if sc.Markers[i].At < sc.Markers[i-1].At {
			t.Errorf("markers not sorted at %d", i)
		}
	}
	var b bytes.Buffer
	if err := trace.EncodeScene(&b, sc); err != nil {
		t.Errorf("scene does not encode: %v", err)
		return
	}
	if !json.Valid(b.Bytes()) {
		t.Errorf("scene.json is not valid JSON")
	}
}

// options reads the optional "options" file of a case: lines "pace 2000",
// "mode proportional", "maxtimers 50".
func options(t *testing.T, dir string) Options {
	var o Options
	data, err := os.ReadFile(filepath.Join(dir, "options"))
	if err != nil {
		return o
	}
	for _, ln := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		f := strings.Fields(ln)
		if len(f) > 0 && f[0] == "theme" {
			continue // see themeOptions
		}
		if len(f) != 2 {
			t.Fatalf("bad options line %q", ln)
		}
		n, _ := strconv.Atoi(f[1])
		switch f[0] {
		case "pace":
			o.Pace = n
		case "mode":
			if f[1] == "proportional" {
				o.Mode = Proportional
			}
		case "maxtimers":
			o.MaxTimersPerInstant = n
		case "maxshapes":
			o.MaxShapes = n
		default:
			t.Fatalf("unknown option %q", f[0])
		}
	}
	return o
}

// themeOptions applies the "theme Name value" lines of the options file of a
// case to the theme of prog (the rest of the theme being the default): a
// color is a #hex, a number a number, any other value a text (the rest of
// the line).
func themeOptions(t *testing.T, dir string, prog *ir.Program) {
	data, err := os.ReadFile(filepath.Join(dir, "options"))
	if err != nil {
		return
	}
	set := map[string]string{}
	for _, ln := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if f := strings.SplitN(ln, " ", 3); len(f) == 3 && f[0] == "theme" {
			set[f[1]] = f[2]
		}
	}
	if len(set) == 0 {
		return
	}
	th := &ir.Theme{Path: "test"}
	for _, n := range stdlib.ThemeNames {
		v := ir.ThemeValue{Name: n.Name, Type: n.Type}
		lit, over := set[n.Name]
		switch {
		case n.Type == types.TColor:
			v.V = defaultColors[n.Name]
			if over {
				v.V = lit
			}
		case n.Type == types.TNumber:
			v.V = defaultNums[n.Name]
			if over {
				f, err := strconv.ParseFloat(lit, 64)
				if err != nil {
					t.Fatal(err)
				}
				v.V = f
			}
		default:
			v.V = defaultTexts[n.Name]
			if over {
				v.V = lit
			}
		}
		delete(set, n.Name)
		th.Values = append(th.Values, v)
	}
	if len(set) > 0 {
		t.Fatalf("unknown theme names %v", set)
	}
	prog.Theme = th
}

// TestGoldenCases builds the scene of every case of testdata/scene and
// compares it with scene.json. A case that must fail has failure.txt with the
// sequence number, code and message of the look fault.
func TestGoldenCases(t *testing.T) {
	dirs, err := filepath.Glob("testdata/scene/*/synod.mod")
	if err != nil || len(dirs) == 0 {
		t.Fatalf("no cases: %v", err)
	}
	for _, m := range dirs {
		dir := filepath.Dir(m)
		name := filepath.Base(dir)
		t.Run(name, func(t *testing.T) {
			ps := checkDir(t, dir, func(s string) string { return s })
			if len(ps) != 1 {
				t.Fatalf("%d main programs", len(ps))
			}
			var p *prog
			for _, x := range ps {
				p = x
			}
			r := p.run()
			if _, err := os.Stat(filepath.Join(dir, "expect-sim-failed")); (r.Failed != nil) != (err == nil) {
				t.Fatalf("the simulation failed: %v (expect-sim-failed: %v)", r.Failed, err == nil)
			}
			o := options(t, dir)
			themeOptions(t, dir, p.Prog)
			sc, fl := Build(p.input(r.Facts), o)
			checkScene(t, sc)
			checkSlots(t, r.Facts, sc, o)
			got := encodeScene(t, sc)
			golden(t, filepath.Join(dir, "scene.json"), got)
			failPath := filepath.Join(dir, "failure.txt")
			if fl != nil {
				text := fmt.Sprintf("fact %d: error[%s]: %s\n", fl.Seq, fl.Fault.Code, fl.Fault.Message)
				golden(t, failPath, []byte(text))
				if !strings.Contains(string(got), `"kind":"failed"`) {
					t.Error("a look fault leaves no failed marker")
				}
			} else if _, err := os.Stat(failPath); err == nil {
				if *update {
					os.Remove(failPath)
				} else {
					t.Error("expected a look fault (failure.txt)")
				}
			}
			// the same scene when built again and from the decoded trace
			sc2, _ := Build(p.input(p.decoded(t, r.Header, r.Facts)), o)
			if !bytes.Equal(got, encodeScene(t, sc2)) {
				t.Error("Build(facts) differs from Build(Decode(Encode(facts)))")
			}
			sc3, _ := Build(p.input(r.Facts), o)
			if !bytes.Equal(got, encodeScene(t, sc3)) {
				t.Error("two builds differ")
			}
		})
	}
}

// TestNativeCoverage: scene.Table and natives.Table together implement every
// phrase native of std, and the look-only natives are exactly scene.Table.
func TestNativeCoverage(t *testing.T) {
	for id, s := range stdlib.Natives {
		if s.Kind != stdlib.Phrase {
			continue
		}
		_, inScene := Table[id]
		_, inNatives := nativesTable[id]
		switch {
		case !inScene && !inNatives:
			t.Errorf("native %s is implemented nowhere", id)
		case inScene && inNatives:
			t.Errorf("native %s is implemented twice", id)
		case inScene != (s.Contexts == stdlib.Look):
			t.Errorf("native %s (contexts %s) is in the wrong table", id, s.Contexts)
		}
	}
	for id := range Table {
		if s, ok := stdlib.Natives[id]; !ok || s.Kind != stdlib.Phrase {
			t.Errorf("scene.Table has %s, which is not a phrase native of std", id)
		}
	}
}

// benchProg is the ping5000 benchmark program.
func benchProg(tb testing.TB) (*prog, []trace.Fact) {
	tb.Helper()
	ps := checkDir(tb, "../../testdata/run/bench/ping5000", func(s string) string { return s })
	var p *prog
	for _, x := range ps {
		p = x
	}
	r := p.run()
	if r.Failed != nil {
		tb.Fatalf("failed: %v", r.Failed)
	}
	return p, r.Facts
}

// BenchmarkBuildPing5000 builds and encodes the scene of the ping5000
// program (plan 5.6: at most 100 ms).
func BenchmarkBuildPing5000(b *testing.B) {
	p, facts := benchProg(b)
	in := p.input(facts)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sc, fl := Build(in, Options{})
		if fl != nil {
			b.Fatal(fl.Fault)
		}
		var buf bytes.Buffer
		if err := trace.EncodeScene(&buf, sc); err != nil {
			b.Fatal(err)
		}
	}
}
