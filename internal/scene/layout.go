package scene

import (
	"math"

	"github.com/synodlang/compiler/internal/trace"
)

// layoutMode is the current layout kind: "", "ring", "row" or "grid".
func (w *world) layoutMode() string { return w.layout }

// setLayout is look.Layout: kind must be "ring", "row" or "grid" (else
// bad-argument). The next reflow arranges the cards.
func (w *world) setLayout(kind string) error {
	switch kind {
	case "ring", "row", "grid":
	default:
		return badArg("look.Layout needs \"ring\", \"row\" or \"grid\", not %q", kind)
	}
	w.layout = kind
	w.layoutDirty = true
	return nil
}

// cards lists the layout members: shapes attached under "card" to nodes that
// are not removed, not deleted, not pinned and not in a group, in node id
// order.
func (w *world) cards(st *store) []int {
	var out []int
	for _, n := range st.nodes() {
		i, ok := w.lookup("card", nodeKey(n))
		if !ok {
			continue
		}
		s := w.shapes[i]
		if s.deleted || s.pinned || s.group >= 0 {
			continue
		}
		out = append(out, i)
	}
	return out
}

// slotPos is the position of member i of n in the layout kind at layout
// scale 1.
func slotPos(kind string, i, n int) (x, y float64) { return slotPosF(kind, i, n, 1) }

// slotPosF is slotPos with the layout scale f (theme.Fill): the ring radius
// and the spacing of rows and grids are multiplied by f.
func slotPosF(kind string, i, n int, f float64) (x, y float64) {
	switch kind {
	case "row":
		return rnd(CenterX + float64(float64(2*i-(n-1))*float64(80*f))), CenterY
	case "grid":
		c := 1
		for c*c < n {
			c++
		}
		r := (n + c - 1) / c
		col, row := i%c, i/c
		return rnd(CenterX + float64(float64(2*col-(c-1))*float64(100*f))),
			rnd(CenterY + float64(float64(2*row-(r-1))*float64(80*f)))
	}
	// ring: an ellipse turned half a slot for an even n, grown with n
	u := float64(i) / float64(n)
	if n%2 == 0 {
		u = float64(2*i+1) / float64(2*n)
	}
	g := 0.0
	if n > 1 {
		g = math.Min(RingGrowMax, math.Max(1, float64(n-3)/3))
	}
	return ellipsePoint(CenterX, CenterY, u, float64(float64(RingRX*f)*g), float64(float64(RingRY*f)*g))
}

// reflow arranges the node cards for the current layout. Call it after each
// activation (after the observers and reactive re-runs of a fact, and after
// each timer); it does nothing unless the layout kind, the membership of
// nodes (st.memberVer), the cards or their pinning changed. New cards are
// placed at once, others morph (the morph rule).
func (w *world) reflow(now int64, st *store) error {
	if w.layout == "" || (!w.layoutDirty && st.memberVer == w.memberSeen) {
		return nil
	}
	w.layoutDirty = false
	w.memberSeen = st.memberVer
	ms := w.cards(st)
	for k, i := range ms {
		x, y := slotPosF(w.layout, k, len(ms), w.th.Fill)
		if math.IsNaN(x) {
			continue
		}
		if err := w.set(i, trace.PX, pv{N: x}, now); err != nil {
			return err
		}
		if err := w.set(i, trace.PY, pv{N: y}, now); err != nil {
			return err
		}
	}
	return w.takeErr()
}
