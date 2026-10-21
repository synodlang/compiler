package scene

import (
	"math"
	"sort"

	"github.com/synodlang/compiler/internal/trace"
)

// This file is the keyframe timeline of one shape: reading a property at a
// time, and the tween algorithm (supersede, hold keys, coalescing, no-ops).
// It has no knowledge of constraints, groups or layout (world.go).

// pv is a property value: N for the numeric properties (x, y, scale,
// opacity, rotation), S for fill, stroke, text and effect (color names and
// text).
type pv struct {
	N float64
	S string
}

const numericProps = trace.PX | trace.PY | trace.PScale | trace.POpacity | trace.PRotation

func isNum(p uint16) bool { return p&numericProps != 0 }

// interpolates reports whether the player interpolates p between keys
// (numbers and colors); text and effect hold.
func interpolates(p uint16) bool { return p != trace.PText && p != trace.PEffect }

func defaultPV(p uint16) pv {
	switch p {
	case trace.PScale, trace.POpacity:
		return pv{N: 1}
	}
	return pv{}
}

func getPV(k *trace.Key, p uint16) pv {
	switch p {
	case trace.PX:
		return pv{N: k.X}
	case trace.PY:
		return pv{N: k.Y}
	case trace.PScale:
		return pv{N: k.Scale}
	case trace.POpacity:
		return pv{N: k.Opacity}
	case trace.PRotation:
		return pv{N: k.Rotation}
	case trace.PFill:
		return pv{S: k.Fill}
	case trace.PStroke:
		return pv{S: k.Stroke}
	case trace.PText:
		return pv{S: k.Text}
	}
	return pv{S: k.Effect}
}

func setPV(k *trace.Key, p uint16, v pv) {
	k.Set |= p
	switch p {
	case trace.PX:
		k.X = v.N
	case trace.PY:
		k.Y = v.N
	case trace.PScale:
		k.Scale = v.N
	case trace.POpacity:
		k.Opacity = v.N
	case trace.PRotation:
		k.Rotation = v.N
	case trace.PFill:
		k.Fill = v.S
	case trace.PStroke:
		k.Stroke = v.S
	case trace.PText:
		k.Text = v.S
	default:
		k.Effect = v.S
	}
}

func clearPV(k *trace.Key, p uint16) {
	k.Set &^= p
	switch p {
	case trace.PX:
		k.X = 0
	case trace.PY:
		k.Y = 0
	case trace.PScale:
		k.Scale = 0
	case trace.POpacity:
		k.Opacity = 0
	case trace.PRotation:
		k.Rotation = 0
	case trace.PFill:
		k.Fill = ""
	case trace.PStroke:
		k.Stroke = ""
	case trace.PText:
		k.Text = ""
	default:
		k.Effect = ""
	}
}

func equalPV(a, b pv) bool { return a.N == b.N && a.S == b.S }

// firstAfter is the index of the first key with At > t.
func firstAfter(ks []trace.Key, t int64) int {
	return sort.Search(len(ks), func(i int) bool { return ks[i].At > t })
}

// prevIdx is the index of the last key with p and At <= t, or -1.
func prevIdx(ks []trace.Key, p uint16, t int64) int {
	for i := firstAfter(ks, t) - 1; i >= 0; i-- {
		if ks[i].Set&p != 0 {
			return i
		}
	}
	return -1
}

// nextIdx is the index of the first key with p and At > t, or -1.
func nextIdx(ks []trace.Key, p uint16, t int64) int {
	for i := firstAfter(ks, t); i < len(ks); i++ {
		if ks[i].Set&p != 0 {
			return i
		}
	}
	return -1
}

// valueAt is the value of p at time t: numbers and colors interpolated with
// smootherstep between the surrounding keys (numbers rounded to 0.01); text
// and effect step. Before any key it is the first key's value; with no key
// at all, the default.
func valueAt(ks []trace.Key, p uint16, t int64) pv {
	pi := prevIdx(ks, p, t)
	ni := nextIdx(ks, p, t)
	if pi < 0 {
		if ni >= 0 {
			return getPV(&ks[ni], p)
		}
		return defaultPV(p)
	}
	a := getPV(&ks[pi], p)
	if ni < 0 || !isNum(p) || ks[pi].At == t {
		return a
	}
	b := getPV(&ks[ni], p)
	u := float64(t-ks[pi].At) / float64(ks[ni].At-ks[pi].At)
	e := smootherstep(u)
	return pv{N: rnd(a.N + float64(float64(b.N-a.N)*e))}
}

// finalVal is the value of p after its last key (its resting value).
func finalVal(ks []trace.Key, p uint16) (pv, bool) {
	for i := len(ks) - 1; i >= 0; i-- {
		if ks[i].Set&p != 0 {
			return getPV(&ks[i], p), true
		}
	}
	return defaultPV(p), false
}

// keyAt returns the key at time at, inserting an empty one in order when
// there is none (keys at the same time merge).
func (w *world) keyAt(s *shape, at int64) *trace.Key {
	ks := s.keys
	n := len(ks)
	if n == 0 || ks[n-1].At < at {
		s.keys = append(s.keys, trace.Key{At: at})
		w.addKey()
		return &s.keys[n]
	}
	i := sort.Search(n, func(i int) bool { return ks[i].At >= at })
	if ks[i].At == at {
		return &s.keys[i]
	}
	s.keys = append(s.keys, trace.Key{})
	copy(s.keys[i+1:], s.keys[i:])
	s.keys[i] = trace.Key{At: at}
	w.addKey()
	return &s.keys[i]
}

func (w *world) putProp(s *shape, at int64, p uint16, v pv) {
	setPV(w.keyAt(s, at), p, v)
}

// removeAfter clears p from every key after t0, dropping keys left empty.
func (w *world) removeAfter(s *shape, p uint16, t0 int64) {
	ks := s.keys
	j := firstAfter(ks, t0)
	out := ks[:j]
	for i := j; i < len(ks); i++ {
		k := ks[i]
		clearPV(&k, p)
		if k.Set == 0 {
			w.nkeys--
			continue
		}
		out = append(out, k)
	}
	s.keys = out
}

// writeProp is tween(p, v, t0, d) of plan 2.6 on one shape; d is already in
// presentation ms (scaled). It
//   - writes nothing for a tween to the current resting value when no tween
//     of p is running;
//   - writes the value of p at t0 as a hold key (the interpolated value
//     rounded to 0.01 for a numeric property cut in the middle, the target of
//     the running tween for a color or text) after removing p from every key
//     after t0 (supersede);
//   - writes v at t0+d (at t0, overwriting, when d is 0);
//   - for an instant change of an interpolated property, adds a hold key one
//     ms before t0 so the player does not glide from an earlier key;
//   - snaps (writes v at t0) when p has no key yet.
//
// It does not touch constraints or followers (see world.moveProp).
func (w *world) writeProp(s *shape, p uint16, v pv, t0, d int64) {
	if s.deleted {
		return
	}
	if d < 0 {
		d = 0
	}
	ks := s.keys
	pi := prevIdx(ks, p, t0)
	ni := nextIdx(ks, p, t0)
	if pi < 0 && ni < 0 {
		if p != trace.PScale && p != trace.PRotation {
			// fill and stroke have no value before their first key: snap
			w.putProp(s, t0, p, v)
			return
		}
		// scale and rotation implicitly hold their default
		if equalPV(defaultPV(p), v) {
			return
		}
		w.putProp(s, t0, p, defaultPV(p))
		ks = s.keys
	}
	if ni < 0 {
		if cur, _ := finalVal(ks, p); equalPV(cur, v) {
			return
		}
	}
	var hold pv
	if ni >= 0 {
		hold = valueAt(ks, p, t0)
		if !isNum(p) {
			hold = getPV(&ks[ni], p)
		}
		w.removeAfter(s, p, t0)
	} else {
		hold, _ = finalVal(ks, p)
	}
	ks = s.keys
	hasAt := false
	if i := prevIdx(ks, p, t0); i >= 0 && ks[i].At == t0 {
		hasAt = true
	}
	if d == 0 {
		if interpolates(p) && !equalPV(hold, v) {
			// Instant change: pin the previous value just before t0 so the
			// player does not glide from an earlier key.
			if i := prevIdx(ks, p, t0-1); i >= 0 && ks[i].At < t0-1 {
				w.putProp(s, t0-1, p, hold)
			}
		}
		w.putProp(s, t0, p, v)
		return
	}
	if !hasAt {
		w.putProp(s, t0, p, hold)
	}
	w.putProp(s, t0+d, p, v)
	if t0+d > s.busy {
		s.busy = t0 + d
	}
}

func finite(f float64) bool { return !math.IsNaN(f) && !math.IsInf(f, 0) }
