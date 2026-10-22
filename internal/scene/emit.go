package scene

import (
	"sort"

	"github.com/synodlang/compiler/internal/trace"
	"github.com/synodlang/compiler/internal/value"
)

// themeColors are the eight std/theme colors in catalog order, used when
// std/theme is not in the IR (a test checks them against theme.synod).
var themeColors = []trace.ThemeColor{
	{Name: "Paper", Hex: "#151A2E"},
	{Name: "Ink", Hex: "#EDE9E1"},
	{Name: "Line", Hex: "#3A4366"},
	{Name: "Quiet", Hex: "#8A90A8"},
	{Name: "Accent", Hex: "#F4A340"},
	{Name: "Good", Hex: "#5CC8B8"},
	{Name: "Warning", Hex: "#E8C35A"},
	{Name: "Critical", Hex: "#E86F5A"},
}

// emitInput is what emit needs besides the world.
type emitInput struct {
	// Slots has one entry per fact processed.
	Slots []trace.SlotEntry
	// Markers in any order; emit sorts them by (At, Seq).
	Markers []trace.Marker
	// Cursor is the slot of the last fact (a lower bound of the duration).
	Cursor int64
	// Hex gives the hex value ("#rrggbb") of a color name used by the
	// scene; it is only asked for names outside the eight theme colors. A
	// nil Hex or an unknown name gives "#000000".
	Hex func(name string) (string, bool)
}

// emitted reports whether shape s is written: the center only when a line or
// group refers to it; a shape created and deleted at the same instant only
// when referenced.
func (s *shape) emitted() bool {
	if s.referenced {
		return true
	}
	if s.isCenter {
		return false
	}
	return !(s.deleted && s.end <= s.born)
}

// emit converts the world into a trace.Scene: shape ids assigned in creation
// order, markers sorted, the theme (the eight theme colors in catalog order,
// then other used names sorted by bytes), and duration = max(cursor, last
// key, last end, last marker). Call finalize on the world first.
func (w *world) emit(in emitInput) *trace.Scene {
	var keys []string
	var idx []int
	for _, s := range w.shapes {
		if s.emitted() {
			keys = append(keys, s.key)
			idx = append(idx, s.idx)
		}
	}
	ids := assignIDs(keys)
	idOf := make(map[int]string, len(idx))
	for n, i := range idx {
		idOf[i] = ids[n]
	}
	dur := in.Cursor
	used := map[string]bool{}
	sc := &trace.Scene{Slots: in.Slots, Shapes: make([]trace.SceneShape, 0, len(idx))}
	for _, i := range idx {
		s := w.shapes[i]
		o := trace.SceneShape{ID: idOf[i], Kind: s.kind, End: s.end, HasEnd: s.deleted}
		if s.node > 0 {
			o.Node, o.HasNode = value.NodeName(s.node), true
		}
		if s.hasMsg {
			o.Msg, o.HasMsg = s.msg, true
		}
		switch s.kind {
		case trace.KindCircle:
			o.R = s.r
		case trace.KindRect:
			o.W, o.H, o.Radius = s.w, s.h, RectRadius
		case trace.KindLine:
			o.From, o.To = idOf[s.from], idOf[s.to]
		case trace.KindGroup:
			o.Items = make([]string, 0, len(s.items))
			for _, m := range s.items {
				o.Items = append(o.Items, idOf[m])
			}
		}
		o.Keys = make([]trace.Key, len(s.keys))
		copy(o.Keys, s.keys)
		for k := range o.Keys {
			key := &o.Keys[k]
			if key.Set&trace.PFill != 0 && key.Fill != "" {
				used[key.Fill] = true
			}
			if key.Set&trace.PStroke != 0 && key.Stroke != "" {
				used[key.Stroke] = true
			}
		}
		if n := len(o.Keys); n > 0 && o.Keys[n-1].At > dur {
			dur = o.Keys[n-1].At
		}
		if o.HasEnd && o.End > dur {
			dur = o.End
		}
		sc.Shapes = append(sc.Shapes, o)
	}
	sc.Markers = append([]trace.Marker(nil), in.Markers...)
	sort.SliceStable(sc.Markers, func(a, b int) bool {
		x, y := sc.Markers[a], sc.Markers[b]
		if x.At != y.At {
			return x.At < y.At
		}
		return x.Seq < y.Seq
	})
	for _, m := range sc.Markers {
		if m.At > dur {
			dur = m.At
		}
	}
	sc.Duration = dur
	sc.Theme = append(sc.Theme, themeColors...)
	for _, t := range themeColors {
		delete(used, t.Name)
	}
	var extra []string
	for n := range used {
		extra = append(extra, n)
	}
	sort.Strings(extra)
	for _, n := range extra {
		hex := "#000000"
		if in.Hex != nil {
			if h, ok := in.Hex(n); ok {
				hex = h
			}
		}
		sc.Theme = append(sc.Theme, trace.ThemeColor{Name: n, Hex: hex})
	}
	return sc
}
