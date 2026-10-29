package scene

import (
	"fmt"
	"strings"
	"testing"

	"github.com/synodlang/compiler/internal/trace"
	"github.com/synodlang/compiler/internal/value"
)

func newTW(pace int) *world { return newWorld(newClock(pace, Ordered, 0), worldConfig{}) }

func (w *world) mk(t *testing.T, kind string, now int64) int {
	t.Helper()
	sp := shapeSpec{Kind: kind, R: 6, W: 96, H: 56, Text: "ab"}
	i, err := w.newShape(sp, fmt.Sprintf("k%d", len(w.shapes)), now)
	if err != nil {
		t.Fatal(err)
	}
	return i
}

// desc renders keys compactly: at{x=.. y=.. s=.. o=.. r=.. fill=.. stroke=.. text=.. fx=..}.
func desc(ks []trace.Key) string {
	var parts []string
	for _, k := range ks {
		var f []string
		n := func(name string, bit uint32, v float64) {
			if k.Set&bit != 0 {
				f = append(f, fmt.Sprintf("%s=%s", name, value.FormatNumber(v)))
			}
		}
		n("x", trace.PX, k.X)
		n("y", trace.PY, k.Y)
		n("s", trace.PScale, k.Scale)
		n("o", trace.POpacity, k.Opacity)
		n("r", trace.PRotation, k.Rotation)
		str := func(name string, bit uint32, v string) {
			if k.Set&bit != 0 {
				f = append(f, name+"="+v)
			}
		}
		str("fill", trace.PFill, k.Fill)
		str("stroke", trace.PStroke, k.Stroke)
		str("text", trace.PText, k.Text)
		str("fx", trace.PEffect, k.Effect)
		parts = append(parts, fmt.Sprintf("%d{%s}", k.At, strings.Join(f, " ")))
	}
	return strings.Join(parts, " ")
}

func (w *world) keys(i int) string { return desc(w.shapes[i].keys) }

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func TestNewShapeDefaults(t *testing.T) {
	w := newTW(1000)
	c := w.mk(t, trace.KindCircle, 100)
	l := w.mk(t, trace.KindLabel, 100)
	g, err := w.newShape(shapeSpec{Kind: trace.KindLine, From: c, To: l}, "ln", 100)
	must(t, err)
	if got := w.keys(c); got != "100{"+home+" o=1}" {
		t.Errorf("circle %s", got)
	}
	if got := w.keys(l); got != "100{"+home+" o=1 fill=Ink text=ab}" {
		t.Errorf("label %s", got)
	}
	if got := w.keys(g); got != "100{o=1 stroke=Line}" {
		t.Errorf("line %s", got)
	}
	if _, err := w.newShape(shapeSpec{Kind: trace.KindCircle, R: -1}, "neg", 0); err == nil {
		t.Error("negative radius accepted")
	}
	small := newWorld(newClock(1000, Ordered, 0), worldConfig{MaxShapes: 1})
	small.mk(t, trace.KindCircle, 0)
	if _, err := small.newShape(shapeSpec{Kind: trace.KindCircle}, "b", 0); err == nil || err.(*worldErr).Code != "run-limit" {
		t.Errorf("shape limit: %v", err)
	}
	few := newWorld(newClock(1000, Ordered, 0), worldConfig{MaxKeys: 2})
	_, err = few.newShape(shapeSpec{Kind: trace.KindCircle}, "a", 0)
	must(t, err)
	if err := few.tween(0, trace.PX, pv{N: 1}, 10, 100); err == nil || err.(*worldErr).Code != "run-limit" {
		t.Errorf("key limit: %v", err)
	}
}

func TestTweenHoldAndCoalesce(t *testing.T) {
	w := newTW(1000)
	s := w.mk(t, trace.KindCircle, 0)
	must(t, w.tween(s, trace.POpacity, pv{N: 0}, 1000, 300))
	if got := w.keys(s); got != "0{"+home+" o=1} 1000{o=1} 1300{o=0}" {
		t.Fatalf("fade: %s", got)
	}
	if w.busyUntil(s) != 1300 {
		t.Errorf("busy %d", w.busyUntil(s))
	}
	// x and y with the same times coalesce into one key
	must(t, w.tween(s, trace.PX, pv{N: 100}, 2000, 400))
	must(t, w.tween(s, trace.PY, pv{N: 200}, 2000, 400))
	if got := w.keys(s); got != "0{"+home+" o=1} 1000{o=1} 1300{o=0} 2000{"+home+"} 2400{x=100 y=200}" {
		t.Fatalf("move: %s", got)
	}
}

func TestTweenNoOp(t *testing.T) {
	w := newTW(1000)
	s := w.mk(t, trace.KindCircle, 0)
	before := w.keys(s)
	must(t, w.tween(s, trace.POpacity, pv{N: 1}, 500, 300))
	must(t, w.tween(s, trace.PX, pv{N: CenterX}, 500, 0))
	must(t, w.tween(s, trace.PScale, pv{N: 1}, 500, 300))
	must(t, w.tween(s, trace.PRotation, pv{N: 0}, 500, 300))
	if w.keys(s) != before {
		t.Fatalf("no-op tweens wrote keys: %s", w.keys(s))
	}
	// scale has an implicit default: a real tween writes a default hold
	must(t, w.tween(s, trace.PScale, pv{N: 2}, 500, 300))
	if got := w.keys(s); got != "0{"+home+" o=1} 500{s=1} 800{s=2}" {
		t.Fatalf("scale: %s", got)
	}
}

func TestTweenSupersedeNumber(t *testing.T) {
	w := newTW(1000)
	s := w.mk(t, trace.KindCircle, 0)
	must(t, w.tween(s, trace.PX, pv{N: CenterX + 100}, 1000, 400))
	// cut halfway: smootherstep(0.5) = 0.5, so CenterX + 50
	must(t, w.tween(s, trace.PX, pv{N: CenterX}, 1200, 100))
	if got := w.keys(s); got != fmt.Sprintf("0{"+home+" o=1} 1000{x=%v} 1200{x=%v} 1300{x=%v}", CenterX, CenterX+50, CenterX) {
		t.Fatalf("supersede: %s", got)
	}
	// a cut value is rounded to 0.01
	w2 := newTW(1000)
	s2 := w2.mk(t, trace.KindCircle, 0)
	must(t, w2.tween(s2, trace.PX, pv{N: 1000}, 0, 300))
	must(t, w2.tween(s2, trace.PX, pv{N: 0}, 100, 0))
	ks := w2.shapes[s2].keys
	for _, k := range ks {
		if k.X != rnd(k.X) {
			t.Errorf("unrounded %v", k.X)
		}
	}
	if got := w2.keys(s2); !strings.HasPrefix(got, "0{"+home+" o=1} 99{x=") {
		t.Fatalf("instant change after a cut: %s", got)
	}
}

func TestTweenSupersedeColorAndText(t *testing.T) {
	w := newTW(1000)
	s := w.mk(t, trace.KindLabel, 0)
	// no earlier fill key on a circle: snaps
	c := w.mk(t, trace.KindCircle, 0)
	must(t, w.tween(c, trace.PFill, pv{S: "Accent"}, 100, 300))
	if got := w.keys(c); got != "0{"+home+" o=1} 100{fill=Accent}" {
		t.Fatalf("first fill snaps: %s", got)
	}
	must(t, w.tween(s, trace.PFill, pv{S: "Accent"}, 1000, 400))
	must(t, w.tween(s, trace.PFill, pv{S: "Good"}, 1100, 400))
	// the cut tween snaps to its target (Accent) at 1100
	if got := w.keys(s); got != "0{"+home+" o=1 fill=Ink text=ab} 1000{fill=Ink} 1100{fill=Accent} 1500{fill=Good}" {
		t.Fatalf("color cut: %s", got)
	}
	must(t, w.tween(s, trace.PText, pv{S: "cd"}, 2000, 0))
	if got := w.shapes[s].keys[len(w.shapes[s].keys)-1]; got.At != 2000 || got.Text != "cd" {
		t.Fatalf("text %+v", got)
	}
	if err := w.tween(c, trace.PText, pv{S: "x"}, 0, 0); err == nil {
		t.Error("text on a circle")
	}
}

func TestTweenValidation(t *testing.T) {
	w := newTW(1000)
	s := w.mk(t, trace.KindCircle, 0)
	for _, tc := range []struct {
		p uint32
		v pv
		d int64
	}{
		{trace.POpacity, pv{N: 1.5}, 0}, {trace.POpacity, pv{N: -0.1}, 0},
		{trace.PScale, pv{N: -1}, 0}, {trace.PX, pv{N: 1}, -5},
	} {
		if err := w.tween(s, tc.p, tc.v, 0, tc.d); err == nil || err.(*worldErr).Code != "bad-argument" {
			t.Errorf("%v: %v", tc, err)
		}
	}
	if w.keys(s) != "0{"+home+" o=1}" {
		t.Fatal("rejected tweens changed the shape")
	}
}

func TestMorphRule(t *testing.T) {
	for _, pace := range []int{1000, 2000} {
		w := newTW(pace)
		d := int64(300000 / pace)
		fresh := w.mk(t, trace.KindRect, 5000)
		old := w.mk(t, trace.KindRect, 1000)
		must(t, w.set(fresh, trace.PFill, pv{S: "Accent"}, 5000))
		must(t, w.set(old, trace.PFill, pv{S: "Accent"}, 5000))
		must(t, w.set(old, trace.PFill, pv{S: "Good"}, 6000))
		if got := w.keys(fresh); got != "5000{"+home+" o=1 fill=Accent}" {
			t.Errorf("fresh: %s", got)
		}
		// old has no fill yet: snaps at 5000; then morphs over 300/pace
		want := fmt.Sprintf("1000{"+home+" o=1} 5000{fill=Accent} 6000{fill=Accent} %d{fill=Good}", 6000+d)
		if got := w.keys(old); got != want {
			t.Errorf("pace %d old: %s want %s", pace, got, want)
		}
		if w.busyUntil(old) != 6000+d {
			t.Errorf("busy %d", w.busyUntil(old))
		}
	}
}

func TestEffect(t *testing.T) {
	w := newTW(2000)
	s := w.mk(t, trace.KindCircle, 0)
	must(t, w.effect(s, "glow", 1000))
	must(t, w.effect(s, "glow", 1100))
	if got := w.keys(s); got != "0{"+home+" o=1} 1000{fx=glow} 1100{fx=glow} 1400{fx=none}" {
		t.Fatalf("effect: %s", got)
	}
	if w.busyUntil(s) != 0 {
		t.Error("effects do not count as busy")
	}
}

func TestTombstone(t *testing.T) {
	w := newTW(1000)
	a := w.mk(t, trace.KindCircle, 0)
	b := w.mk(t, trace.KindCircle, 0)
	must(t, w.tween(a, trace.PX, pv{N: 0}, 100, 500))
	w.delete(a, 300)
	if got := w.keys(a); got != fmt.Sprintf("0{"+home+" o=1} 100{x=%v}", CenterX) {
		t.Fatalf("keys after the end are dropped: %s", got)
	}
	if !w.shapes[a].deleted || w.shapes[a].end != 300 || w.busyUntil(a) != 0 {
		t.Fatal("not deleted")
	}
	before := w.keys(a)
	must(t, w.tween(a, trace.POpacity, pv{N: 0}, 400, 10))
	must(t, w.set(a, trace.PFill, pv{S: "Good"}, 400))
	must(t, w.effect(a, "glow", 400))
	must(t, w.place(a, "above", b, 400))
	must(t, w.orbit(a, b, 0.5, 400))
	if w.keys(a) != before {
		t.Fatal("tombstone changed")
	}
	// still attached
	must(t, w.attach("card", nodeKey(1), a, 0))
	w.delete(a, 500) // idempotent
	if w.shapes[a].end != 300 {
		t.Fatal("end moved")
	}
	if i, ok := w.lookup("card", nodeKey(1)); !ok || i != a {
		t.Fatal("tombstone not attached")
	}
	if got := w.attachedTo(nodeKey(1)); len(got) != 0 {
		t.Fatalf("attachedTo lists a tombstone: %v", got)
	}
}

func TestGroup(t *testing.T) {
	w := newTW(1000)
	ball := w.mk(t, trace.KindCircle, 0)
	tag := w.mk(t, trace.KindLabel, 0)
	must(t, w.place(tag, "above", ball, 0))
	// label ab: 16x18; circle r=6: 12x12; dy = -(6+8+9) = -23
	if got := w.keys(tag); !strings.HasPrefix(got, fmt.Sprintf("0{x=%v y=%v o=1", CenterX, CenterY-23)) {
		t.Fatalf("place: %s", got)
	}
	g, err := w.group([]int{ball, tag}, "grp", 0)
	must(t, err)
	if got := w.keys(g); got != "0{"+home+" o=1}" {
		t.Fatalf("group: %s", got)
	}
	if got := w.keys(tag); !strings.HasPrefix(got, "0{x=0 y=-23 o=1") {
		t.Fatalf("member relative: %s", got)
	}
	if !w.shapes[ball].referenced || w.shapes[ball].group != g {
		t.Fatal("member not linked")
	}
	if _, err := w.group([]int{ball}, "g2", 0); err == nil || err.(*worldErr).Code != "bad-argument" {
		t.Fatalf("second group: %v", err)
	}
	// moving the group moves the world position of the members
	must(t, w.tween(g, trace.PX, pv{N: 1000}, 100, 0))
	if x, y := w.worldPos(tag, 100); x != 1000 || y != CenterY-23 {
		t.Fatalf("world pos %v %v", x, y)
	}
	// fill on the group skips labels
	must(t, w.set(g, trace.PFill, pv{S: "Accent"}, 0))
	if strings.Contains(w.keys(tag), "Accent") || !strings.Contains(w.keys(ball), "fill=Accent") {
		t.Fatalf("group fill: ball %s tag %s", w.keys(ball), w.keys(tag))
	}
	// deleting the group deletes the members
	w.delete(g, 200)
	if !w.shapes[ball].deleted || !w.shapes[tag].deleted || w.shapes[ball].end != 200 {
		t.Fatal("group delete")
	}
	// a group of a shape drawn earlier keeps its place in the world
	w2 := newTW(1000)
	a := w2.mk(t, trace.KindCircle, 0)
	must(t, w2.tween(a, trace.PX, pv{N: 300}, 0, 0))
	g2, _ := w2.group([]int{a}, "g", 10)
	if x, _ := w2.worldPos(a, 5); x != 300 {
		t.Fatalf("history shifted: %v", x)
	}
	if got := w2.keys(g2); got != fmt.Sprintf("10{x=300 y=%v o=1}", CenterY) {
		t.Fatalf("group at first item: %s", got)
	}
}

func TestPlaceFollowsAndBreaks(t *testing.T) {
	w := newTW(1000)
	card := w.mk(t, trace.KindRect, 0)
	row := w.mk(t, trace.KindLabel, 0)
	must(t, w.place(row, "below", card, 0))
	// rect 56 high, label 18: dy = 28 + 8 + 9 = 45
	if got := w.keys(row); !strings.HasPrefix(got, fmt.Sprintf("0{x=%v y=%v o=1", CenterX, CenterY+45)) {
		t.Fatalf("below: %s", got)
	}
	must(t, w.tween(card, trace.PX, pv{N: 400}, 1000, 300))
	must(t, w.tween(card, trace.PY, pv{N: 100}, 1000, 300))
	cx, rx := w.shapes[card].keys, w.shapes[row].keys
	last := func(ks []trace.Key) trace.Key { return ks[len(ks)-1] }
	if last(cx).At != 1300 || last(rx).At != 1300 || last(rx).X != 400 || last(rx).Y != 145 {
		t.Fatalf("follower: card %s row %s", desc(cx), desc(rx))
	}
	// the row holds at 1000 like the card does
	if !strings.Contains(desc(rx), fmt.Sprintf("1000{x=%v y=%v}", CenterX, CenterY+45)) {
		t.Fatalf("row hold: %s", desc(rx))
	}
	// beside: dx = 48 + 8 + 8
	b := w.mk(t, trace.KindLabel, 2000)
	must(t, w.place(b, "beside", card, 2000))
	if x, _ := w.worldPos(b, 2000); x != 400+64 {
		t.Fatalf("beside x %v", x)
	}
	// moving the row itself breaks the constraint
	must(t, w.tween(row, trace.PX, pv{N: 50}, 3000, 0))
	must(t, w.tween(card, trace.PX, pv{N: 0}, 4000, 0))
	if last(w.shapes[row].keys).X != 50 {
		t.Fatalf("broken constraint still follows: %s", w.keys(row))
	}
	if !w.shapes[row].pinned {
		t.Error("tween of x does not pin")
	}
	// a cycle is bad-argument
	if err := w.place(card, "above", b, 5000); err == nil {
		t.Error("cycle accepted")
	}
	if err := w.place(card, "above", card, 5000); err == nil {
		t.Error("self place accepted")
	}
}

func TestPlaceMidTweenCopiesFutureKeys(t *testing.T) {
	w := newTW(1000)
	a := w.mk(t, trace.KindRect, 0)
	must(t, w.tween(a, trace.PX, pv{N: 1000}, 100, 500)) // runs until 600
	s := w.mk(t, trace.KindRect, 200)
	must(t, w.place(s, "beside", a, 200))
	ks := w.shapes[s].keys
	if k := ks[len(ks)-1]; k.At != 600 || k.X != 1000+104 {
		t.Fatalf("follower did not copy the running tween: %s", desc(ks))
	}
}

func TestOrbit(t *testing.T) {
	w := newTW(1000)
	c, err := w.center()
	must(t, err)
	if c2, _ := w.center(); c2 != c {
		t.Fatal("center is a singleton")
	}
	s := w.mk(t, trace.KindCircle, 0)
	must(t, w.orbit(s, c, 0.25, 0))
	if got := w.keys(s); got != fmt.Sprintf("0{x=%v y=%v o=1}", CenterX+OrbitRadius, CenterY) {
		t.Fatalf("orbit: %s", got)
	}
	if !w.shapes[s].pinned {
		t.Error("orbit pins")
	}
	// morph rule on an older shape
	must(t, w.orbit(s, c, 0.5, 2000))
	if got := w.keys(s); got != fmt.Sprintf("0{x=%v y=%v o=1} 2000{x=%v y=%v} 2300{x=%v y=%v}", CenterX+OrbitRadius, CenterY, CenterX+OrbitRadius, CenterY, CenterX, CenterY+OrbitRadius) {
		t.Fatalf("orbit morph: %s", got)
	}
}

func TestFlightLandAndDrop(t *testing.T) {
	for _, mode := range []string{"land", "drop", "unlanded", "already"} {
		w := newTW(1000)
		a, b := w.mk(t, trace.KindRect, 0), w.mk(t, trace.KindRect, 0)
		must(t, w.tween(a, trace.PX, pv{N: 100}, 0, 0))
		must(t, w.tween(a, trace.PY, pv{N: 100}, 0, 0))
		must(t, w.tween(b, trace.PX, pv{N: 500}, 0, 0))
		must(t, w.tween(b, trace.PY, pv{N: 300}, 0, 0))
		dot := w.mk(t, trace.KindCircle, 1000)
		switch mode {
		case "already":
			w.land(7, 900, false)
		}
		must(t, w.flightStart(dot, a, b, 7, 1000))
		if got := w.keys(dot); mode != "already" && got != "1000{x=100 y=100 o=1}" {
			t.Fatalf("%s start: %s", mode, got)
		}
		switch mode {
		case "land":
			w.land(7, 1600, false)
		case "drop":
			w.land(7, 1600, true)
		case "unlanded":
			w.finalize(1800)
		}
		got := w.keys(dot)
		want := map[string]string{
			"land":     "1000{x=100 y=100 o=1} 1600{x=500 y=300}",
			"drop":     "1000{x=100 y=100 o=1} 1600{x=300 y=200}",
			"unlanded": "1000{x=100 y=100 o=1} 1800{x=300 y=200}",
			"already":  "1000{x=500 y=300 o=1}",
		}[mode]
		if got != want {
			t.Errorf("%s: %s want %s", mode, got, want)
		}
		if len(w.pending) != 0 {
			t.Errorf("%s: pending flights left", mode)
		}
	}
}

func TestFlightCancelledAndDeleted(t *testing.T) {
	w := newTW(1000)
	a, b := w.mk(t, trace.KindRect, 0), w.mk(t, trace.KindRect, 0)
	d1, d2 := w.mk(t, trace.KindCircle, 0), w.mk(t, trace.KindCircle, 0)
	must(t, w.flightStart(d1, a, b, 1, 0))
	must(t, w.flightStart(d2, a, b, 2, 0))
	must(t, w.tween(d1, trace.PX, pv{N: 5}, 10, 0)) // cancels flight 1
	w.delete(d2, 20)                                // drops flight 2
	w.land(1, 100, false)
	w.land(2, 100, false)
	w.finalize(200)
	if k := w.shapes[d1].keys; k[len(k)-1].At != 10 || len(w.shapes[d2].keys) != 1 {
		t.Fatalf("d1 %s d2 %s", w.keys(d1), w.keys(d2))
	}
}

func TestReplacement(t *testing.T) {
	w := newTW(1000)
	n := nodeKey(3)
	old := w.mk(t, trace.KindCircle, 0)
	must(t, w.attach("dot", n, old, 0))
	must(t, w.tween(old, trace.PX, pv{N: 200}, 100, 0))
	must(t, w.tween(old, trace.PY, pv{N: 300}, 100, 0))
	must(t, w.tween(old, trace.PScale, pv{N: 2}, 100, 0))
	must(t, w.set(old, trace.PFill, pv{S: "Good"}, 100))
	follower := w.mk(t, trace.KindLabel, 100)
	must(t, w.place(follower, "above", old, 100))
	w.shapes[old].pinned = true

	// replaced later: old fades out over 300 then is deleted, new fades in
	nw := w.mk(t, trace.KindCircle, 1000)
	must(t, w.attach("dot", n, nw, 1000))
	if got := w.keys(nw); got != "1000{x=200 y=300 s=2 o=0 fill=Good} 1300{o=1}" {
		t.Fatalf("new: %s", got)
	}
	if !w.shapes[old].deleted || w.shapes[old].end != 1300 {
		t.Fatalf("old end %d deleted=%v", w.shapes[old].end, w.shapes[old].deleted)
	}
	if got := w.keys(old); !strings.HasSuffix(got, "1000{o=1} 1300{o=0}") {
		t.Fatalf("old fade: %s", got)
	}
	if i, _ := w.lookup("dot", n); i != nw {
		t.Fatal("attachment not moved")
	}
	// constraints and pinning moved
	if w.shapes[follower].anchor != nw || len(w.shapes[nw].followers) != 1 || !w.shapes[nw].pinned {
		t.Fatal("constraint hand-over")
	}
	// the new shape is not brand-new: Fill morphs
	must(t, w.set(nw, trace.PFill, pv{S: "Accent"}, 1000))
	if got := w.keys(nw); !strings.HasSuffix(got, "1300{o=1 fill=Accent}") || strings.Contains(got, "1000{fill=Accent") {
		t.Fatalf("morph after replacement: %s", got)
	}

	// replaced in the same instant: old is deleted silently, new at once
	o2 := w.mk(t, trace.KindCircle, 2000)
	must(t, w.attach("x", n, o2, 2000))
	n2 := w.mk(t, trace.KindCircle, 2000)
	must(t, w.attach("x", n, n2, 2000))
	if got := w.keys(n2); got != "2000{"+home+" o=1}" {
		t.Fatalf("same instant: %s", got)
	}
	if w.shapes[o2].end != 2000 || w.shapes[o2].emitted() {
		t.Fatal("silent delete")
	}
	// the first draw of a name never replaces anything
	if got := w.attachedTo(n); len(got) != 2 {
		t.Fatalf("attachedTo %v", got)
	}
}

func TestReplacementKeepsOwnFillWhenNew(t *testing.T) {
	w := newTW(1000)
	k := textKey("k")
	old := w.mk(t, trace.KindLabel, 0)
	must(t, w.attach("l", k, old, 0))
	nw := w.mk(t, trace.KindLabel, 100)
	must(t, w.attach("l", k, nw, 100))
	// text inherited from old ("ab") equals own ("ab"): nothing extra
	if got := w.keys(nw); got != "100{"+home+" o=0 fill=Ink text=ab} 400{o=1}" {
		t.Fatalf("label replace: %s", got)
	}
}

func TestIDsViaEmit(t *testing.T) {
	w := newTW(1000)
	s := w.mk(t, trace.KindCircle, 0)
	w.mk(t, trace.KindCircle, 0)
	sc := w.emit(emitInput{})
	if len(sc.Shapes) != 2 || sc.Shapes[0].ID == sc.Shapes[1].ID {
		t.Fatal("ids")
	}
	if sc.Shapes[0].ID != hashID("k0") {
		t.Fatalf("id %s", sc.Shapes[0].ID)
	}
	_ = s
}
