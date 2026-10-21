package scene

import (
	"testing"

	"github.com/synodlang/compiler/internal/trace"
)

type lw struct {
	w  *world
	st *store
}

func newLW(pace int) *lw { return &lw{newTW(pace), newStore()} }

func (l *lw) addNode(t *testing.T, n int, now int64) int {
	t.Helper()
	l.st.apply(trace.Fact{Ev: "sim.Added", Node: n})
	i := l.w.mk(t, trace.KindRect, now)
	must(t, l.w.attach("card", nodeKey(n), i, now))
	return i
}

func (l *lw) pos(i int, now int64) [2]float64 {
	x, y := l.w.worldPos(i, now)
	return [2]float64{x, y}
}

func TestLayoutKinds(t *testing.T) {
	l := newLW(1000)
	if err := l.w.setLayout("spiral"); err == nil || err.(*worldErr).Code != "bad-argument" {
		t.Fatalf("bad kind: %v", err)
	}
	var cards []int
	for n := 1; n <= 4; n++ {
		cards = append(cards, l.addNode(t, n, 0))
	}
	must(t, l.w.setLayout("ring"))
	must(t, l.w.reflow(0, l.st))
	// brand-new shapes are placed at once
	want := [][2]float64{{800, 200}, {1100, 500}, {800, 800}, {500, 500}}
	for i, c := range cards {
		if got := l.pos(c, 0); got != want[i] {
			t.Errorf("ring %d: %v want %v", i, got, want[i])
		}
		if len(l.w.shapes[c].keys) != 1 {
			t.Errorf("ring %d: new card should not tween: %s", i, l.w.keys(c))
		}
	}

	// row of 4: x = 800 + (2i-3)*80
	must(t, l.w.setLayout("row"))
	must(t, l.w.reflow(5000, l.st))
	for i, c := range cards {
		if got := l.pos(c, 5300); got != [2]float64{800 + float64(2*i-3)*80, 500} {
			t.Errorf("row %d: %v", i, got)
		}
	}
	// the move is a morph over 300
	if got := l.w.keys(cards[0]); got != "0{x=800 y=200 o=1} 5000{x=800 y=200} 5300{x=560 y=500}" {
		t.Errorf("row morph: %s", got)
	}

	// grid of 5: c = 3, r = 2
	l.addNode(t, 5, 9000)
	must(t, l.w.setLayout("grid"))
	must(t, l.w.reflow(9000, l.st))
	exp := [][2]float64{{600, 420}, {800, 420}, {1000, 420}, {600, 580}, {800, 580}}
	for k, n := range []int{1, 2, 3, 4, 5} {
		i, _ := l.w.lookup("card", nodeKey(n))
		// morphing cards arrive at +300; the new one is placed at once
		at := int64(9300)
		if got := l.pos(i, at); got != exp[k] {
			t.Errorf("grid %d: %v want %v", n, got, exp[k])
		}
	}
}

func TestLayoutGridShapes(t *testing.T) {
	for n, cr := range map[int][2]int{1: {1, 1}, 2: {2, 1}, 4: {2, 2}, 9: {3, 3}, 10: {4, 3}} {
		// count distinct positions
		seen := map[[2]float64]bool{}
		for i := 0; i < n; i++ {
			x, y := slotPos("grid", i, n)
			seen[[2]float64{x, y}] = true
		}
		if len(seen) != n {
			t.Errorf("grid %d: %d distinct slots (c=%d r=%d)", n, len(seen), cr[0], cr[1])
		}
	}
	if x, y := slotPos("grid", 0, 1); x != 800 || y != 500 {
		t.Errorf("grid of one: %v %v", x, y)
	}
}

func TestReflowOnAddRemoveAndPin(t *testing.T) {
	l := newLW(1000)
	must(t, l.w.setLayout("row"))
	c1 := l.addNode(t, 1, 0)
	must(t, l.w.reflow(0, l.st))
	if p := l.pos(c1, 0); p != [2]float64{800, 500} {
		t.Fatalf("single: %v", p)
	}
	c2 := l.addNode(t, 2, 1000)
	must(t, l.w.reflow(1000, l.st))
	if p := l.pos(c1, 1300); p != [2]float64{720, 500} {
		t.Errorf("c1 after add: %v", p)
	}
	if p := l.pos(c2, 1000); p != [2]float64{880, 500} {
		t.Errorf("c2 placed at once: %v", p)
	}
	// nothing changed: no new keys
	n := len(l.w.shapes[c1].keys)
	must(t, l.w.reflow(2000, l.st))
	if len(l.w.shapes[c1].keys) != n {
		t.Error("idle reflow wrote keys")
	}
	// pinning takes c2 out for good
	must(t, l.w.tween(c2, trace.PX, pv{N: 100}, 3000, 0))
	must(t, l.w.reflow(3000, l.st))
	if p := l.pos(c1, 3300); p != [2]float64{800, 500} {
		t.Errorf("c1 after pin: %v", p)
	}
	c3 := l.addNode(t, 3, 4000)
	must(t, l.w.reflow(4000, l.st))
	if p := l.pos(c3, 4300); p != [2]float64{880, 500} {
		t.Errorf("c3: %v", p)
	}
	// removing node 1 re-flows the rest even though its card is not deleted yet
	l.st.apply(trace.Fact{Ev: "sim.Removed", Node: 1})
	must(t, l.w.reflow(5000, l.st))
	if p := l.pos(c3, 5300); p != [2]float64{800, 500} {
		t.Errorf("c3 after remove: %v", p)
	}
	// a deleted card leaves the layout
	l.w.delete(c3, 6000)
	c4 := l.addNode(t, 4, 6000)
	must(t, l.w.reflow(6000, l.st))
	if p := l.pos(c4, 6000); p != [2]float64{800, 500} {
		t.Errorf("c4: %v", p)
	}
}

func TestPlaceFollowsLayoutMove(t *testing.T) {
	l := newLW(1000)
	must(t, l.w.setLayout("row"))
	c1 := l.addNode(t, 1, 0)
	row := l.w.mk(t, trace.KindLabel, 0)
	must(t, l.w.place(row, "below", c1, 0))
	must(t, l.w.reflow(0, l.st))
	l.addNode(t, 2, 1000)
	must(t, l.w.reflow(1000, l.st))
	// the card moved 80 to the left over 300; the row moves with it
	if p := l.pos(c1, 1300); p != [2]float64{720, 500} {
		t.Fatalf("card %v", p)
	}
	if p := l.pos(row, 1300); p != [2]float64{720, 545} {
		t.Fatalf("row %v", p)
	}
	if p := l.pos(row, 1150); p != l.posOffset(c1, 1150, 45) {
		t.Fatalf("row mid %v", p)
	}
}

func (l *lw) posOffset(i int, now int64, dy float64) [2]float64 {
	p := l.pos(i, now)
	return [2]float64{p[0], rnd(p[1] + dy)}
}

func TestGroupedCardsAreNotLaidOut(t *testing.T) {
	l := newLW(1000)
	must(t, l.w.setLayout("row"))
	l.st.apply(trace.Fact{Ev: "sim.Added", Node: 1})
	box := l.w.mk(t, trace.KindRect, 0)
	name := l.w.mk(t, trace.KindLabel, 0)
	g, err := l.w.group([]int{box, name}, "g", 0)
	must(t, err)
	must(t, l.w.attach("card", nodeKey(1), g, 0))
	must(t, l.w.attach("card", textKey("x"), box, 0)) // not a node key: ignored
	must(t, l.w.reflow(0, l.st))
	if got := l.w.cards(l.st); len(got) != 1 || got[0] != g {
		t.Fatalf("cards %v", got)
	}
}
