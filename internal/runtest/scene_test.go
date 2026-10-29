package runtest

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/synodlang/compiler/internal/scene"
	"github.com/synodlang/compiler/internal/trace"
)

// checkSceneInvariants checks a scene against the format (plan 5.1): unique
// ids, valid line and group references, colors from the theme, keys strictly
// increasing, slots in order and valid JSON.
func checkSceneInvariants(t testing.TB, sc *trace.Scene, facts []trace.Fact) {
	t.Helper()
	ids := map[string]bool{}
	for _, s := range sc.Shapes {
		if ids[s.ID] {
			t.Errorf("duplicate shape id %s", s.ID)
		}
		ids[s.ID] = true
	}
	theme := map[string]bool{}
	for i, c := range sc.Theme {
		theme[c.Name] = true
		want := []string{"Paper", "Ink", "Line", "Quiet", "Accent", "Good", "Warning", "Critical"}
		if i < len(want) && c.Name != want[i] {
			t.Errorf("theme color %d is %s, want %s", i, c.Name, want[i])
		}
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
		case trace.KindCircle, trace.KindRect, trace.KindLabel:
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
	}
	if facts != nil && len(sc.Slots) != len(facts) {
		t.Errorf("%d slots for %d facts", len(sc.Slots), len(facts))
	}
	for i, e := range sc.Slots {
		if e.Seq != i+1 {
			t.Errorf("slot %d has seq %d", i, e.Seq)
		}
		if i > 0 && e.At < sc.Slots[i-1].At {
			t.Errorf("slot %d goes back in time", i)
		}
	}
	b := encodeScene(t, sc)
	if !json.Valid(b) {
		t.Errorf("scene.json is not valid JSON")
	}
	if !bytes.HasPrefix(b, []byte(`{"format":"synod-scene/1",`)) || bytes.Contains(b, []byte(`"fill":"#`)) {
		t.Errorf("scene.json header or raw colors: %.80s", b)
	}
}

func isLessonNoted(f trace.Fact) bool {
	return f.Ev == "sim.Emitted" && f.Record != nil && f.Record.Type.Pkg.Path == "std/lesson" && f.Record.Type.Name == "Noted"
}

// TestSceneExamples checks the scene of every example at its own seed: the
// invariants, the golden file, Build(facts) == Build(Decode(Encode(facts)))
// and, for the examples drawn by std/look/basic, that it animates them: a
// card for every node, a group per message sent and a caption per note.
func TestSceneExamples(t *testing.T) {
	ex := examples(t)
	for _, n := range names(ex) {
		p := ex[n]
		t.Run(n, func(t *testing.T) {
			r := p.run(nil)
			sc, fl := p.scene(t, r.Facts)
			if fl != nil {
				t.Fatalf("look failed at fact %d: %s", fl.Seq, fl.Fault.Error())
			}
			checkSceneInvariants(t, sc, r.Facts)
			a := encodeScene(t, sc)
			want, err := os.ReadFile(filepath.Join(goldenDir, n, "scene.json"))
			if err != nil || !bytes.Equal(a, want) {
				t.Errorf("scene.json differs from the golden (err %v)", err)
			}

			var buf bytes.Buffer
			if err := trace.Encode(&buf, r.Header, r.Facts, p.opts()); err != nil {
				t.Fatal(err)
			}
			extra := p.Prog.Packages
			if p.Look != nil {
				extra = append(append(extra[:0:0], extra...), p.Look.Packages...)
			}
			_, facts, err := trace.Decode(&buf, p.Prog, extra...)
			if err != nil {
				t.Fatal(err)
			}
			sc2, fl := p.scene(t, facts)
			if fl != nil {
				t.Fatalf("decoded: look failed: %s", fl.Fault.Error())
			}
			if !bytes.Equal(a, encodeScene(t, sc2)) {
				t.Error("Build(facts) differs from Build(Decode(Encode(facts)))")
			}

			if p.Look == nil {
				return // ring and raft bring their own looks
			}
			var sent, noted, added int
			for _, f := range r.Facts {
				switch {
				case f.Ev == "net.Sent":
					sent++
				case f.Ev == "sim.Added":
					added++
				case isLessonNoted(f):
					noted++
				}
			}
			groups, rects, captions := 0, 0, 0
			for _, s := range sc.Shapes {
				switch s.Kind {
				case trace.KindGroup:
					groups++
				case trace.KindRect:
					rects++
				}
			}
			for _, m := range sc.Markers {
				if m.Kind == "caption" {
					captions++
				}
			}
			if rects < added+1 { // a card per node and the border
				t.Errorf("%d rects for %d nodes", rects, added)
			}
			if groups < sent {
				t.Errorf("%d flights for %d messages", groups, sent)
			}
			pauses := 0
			for _, m := range sc.Markers {
				if m.Kind == "pause" {
					pauses++
				}
			}
			if captions == 0 && noted > 0 {
				t.Errorf("no caption for %d notes", noted)
			}
			t.Logf("%d nodes, %d sent, %d notes -> %d rects, %d groups, %d captions, %d pauses", added, sent, noted, rects, groups, captions, pauses)
		})
	}
}

// TestSceneSeeds builds the scene of every example at 20 seeds: the
// invariants hold and no look fails.
func TestSceneSeeds(t *testing.T) {
	ex := examples(t)
	for _, n := range names(ex) {
		p := ex[n]
		t.Run(n, func(t *testing.T) {
			for s := uint64(1); s <= 20; s++ {
				r := p.run(u64(s))
				sc, fl := p.scene(t, r.Facts)
				if fl != nil {
					t.Fatalf("seed %d: look failed at fact %d: %s", s, fl.Seq, fl.Fault.Error())
				}
				checkSceneInvariants(t, sc, r.Facts)
			}
		})
	}
}

// TestScenePaceMode: pace speeds a scene up and slows it down, the
// proportional mode takes about 30 seconds over pace.
func TestScenePaceMode(t *testing.T) {
	p := examples(t)["gossip"]
	r := p.run(nil)
	dur := func(o scene.Options) int64 {
		sc, fl := p.sceneWith(t, r.Facts, o)
		if fl != nil {
			t.Fatal(fl.Fault.Error())
		}
		return sc.Duration
	}
	d1, d2, d4, dq := dur(scene.Options{Pace: 1000}), dur(scene.Options{Pace: 2000}), dur(scene.Options{Pace: 4000}), dur(scene.Options{Pace: 250})
	t.Logf("gossip: pace 0.25 %dms, 1 %dms, 2 %dms, 4 %dms", dq, d1, d2, d4)
	if !(dq > d1 && d1 > d2 && d2 > d4) {
		t.Error("the duration does not fall with the pace")
	}
	if diff := d1/2 - d2; diff < -d1/20 || diff > d1/20 {
		t.Errorf("pace 2 gives %d, want about half of %d", d2, d1)
	}
	for _, pace := range []int{1000, 2000} {
		got := dur(scene.Options{Pace: pace, Mode: scene.Proportional})
		want := int64(30000 * 1000 / pace)
		if got < want-want/10 || got > want+want/5 {
			t.Errorf("proportional at pace %d: %dms, want about %dms", pace, got, want)
		}
	}
}

// TestSceneBrokenBorder patches the adopt loop out of Paxos and finds a seed
// that breaks the invariant: the scene shows the border at sim.Broken.
func TestSceneBrokenBorder(t *testing.T) {
	dir := copyTree(t, examplesDir)
	breakPaxos(t, dir)
	demo := filepath.Join(dir, "paxos", "demo.synod")
	b, err := os.ReadFile(demo)
	if err != nil {
		t.Fatal(err)
	}
	patched := strings.Replace(string(b), "10ms and 40ms", "1ms and 40ms", 1)
	if err := os.WriteFile(demo, []byte(patched), 0o644); err != nil {
		t.Fatal(err)
	}
	p := check(t, dir)["paxos"]
	for s := uint64(1); s <= 200; s++ {
		r := p.run(u64(s))
		at := -1
		for i, f := range r.Facts {
			if f.Ev == "sim.Broken" {
				at = i
				break
			}
		}
		if at < 0 {
			continue
		}
		sc, fl := p.scene(t, r.Facts)
		if fl != nil {
			t.Fatal(fl.Fault.Error())
		}
		checkSceneInvariants(t, sc, r.Facts)
		slot := sc.Slots[at].At
		for _, sh := range sc.Shapes {
			if sh.Kind != trace.KindRect || sh.W != 1032 || sh.H != 642 {
				continue
			}
			lit, glow := false, false
			for _, k := range sh.Keys {
				if k.At >= slot && k.Set&trace.POpacity != 0 && k.Opacity == 1 {
					lit = true
				}
				if k.At >= slot && k.Set&trace.PEffect != 0 && k.Effect == "glow" {
					glow = true
				}
			}
			if !lit || !glow {
				t.Errorf("seed %d: border not lit (%v) or glowing (%v) at %dms", s, lit, glow, slot)
			}
			return
		}
		t.Fatalf("seed %d: no border shape", s)
	}
	t.Fatal("no seed breaks the invariant")
}
