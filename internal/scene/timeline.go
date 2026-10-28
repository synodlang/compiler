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
// opacity, rotation, curve, trail, draw, halo, count), S for the textual ones
// (fill, stroke, text, effect, icon, role, face: color names, text and
// names). The dash is the pair (N, M): on and off.
type pv struct {
	N, M float64
	S    string
}

// numericProps are the properties the reader interpolates as numbers.
const numericProps = trace.PX | trace.PY | trace.PScale | trace.POpacity | trace.PRotation |
	trace.PDraw | trace.PHalo | trace.PCount

// stepProps hold their value until the next key (they are never tweened).
const stepProps = trace.PText | trace.PEffect | trace.PCurve | trace.PTrail | trace.PDash | trace.PRole | trace.PFace

// easedProps follow the motion curve of the theme (smooth or spring); every
// other numeric property uses smootherstep.
const easedProps = trace.PX | trace.PY | trace.PScale | trace.PRotation

func isNum(p uint32) bool { return p&numericProps != 0 }

// interpolates reports whether the player interpolates p between keys
// (numbers, colors and icons); the step properties hold.
func interpolates(p uint32) bool { return p&stepProps == 0 }

// implicit reports whether p holds its default before its first key, so a
// tween to the default of a shape that never set it writes nothing.
func implicit(p uint32) bool {
	switch p {
	case trace.PScale, trace.PRotation, trace.PDraw, trace.PHalo:
		return true
	}
	return false
}

func defaultPV(p uint32) pv {
	switch p {
	case trace.PScale, trace.POpacity, trace.PDraw:
		return pv{N: 1}
	case trace.PRole:
		return pv{S: "label"}
	case trace.PFace:
		return pv{S: "sans"}
	}
	return pv{}
}

func getPV(k *trace.Key, p uint32) pv {
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
	case trace.PCurve:
		return pv{N: k.Curve}
	case trace.PTrail:
		return pv{N: k.Trail}
	case trace.PDash:
		return pv{N: k.DashOn, M: k.DashOff}
	case trace.PDraw:
		return pv{N: k.Draw}
	case trace.PHalo:
		return pv{N: k.Halo}
	case trace.PIcon:
		return pv{S: k.Icon}
	case trace.PCount:
		return pv{N: k.Count}
	case trace.PRole:
		return pv{S: k.Role}
	case trace.PFace:
		return pv{S: k.Face}
	}
	return pv{S: k.Effect}
}

func setPV(k *trace.Key, p uint32, v pv) {
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
	case trace.PCurve:
		k.Curve = v.N
	case trace.PTrail:
		k.Trail = v.N
	case trace.PDash:
		k.DashOn, k.DashOff = v.N, v.M
	case trace.PDraw:
		k.Draw = v.N
	case trace.PHalo:
		k.Halo = v.N
	case trace.PIcon:
		k.Icon = v.S
	case trace.PCount:
		k.Count = v.N
	case trace.PRole:
		k.Role = v.S
	case trace.PFace:
		k.Face = v.S
	default:
		k.Effect = v.S
	}
}

func clearPV(k *trace.Key, p uint32) {
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
	case trace.PCurve:
		k.Curve = 0
	case trace.PTrail:
		k.Trail = 0
	case trace.PDash:
		k.DashOn, k.DashOff = 0, 0
	case trace.PDraw:
		k.Draw = 0
	case trace.PHalo:
		k.Halo = 0
	case trace.PIcon:
		k.Icon = ""
	case trace.PCount:
		k.Count = 0
	case trace.PRole:
		k.Role = ""
	case trace.PFace:
		k.Face = ""
	default:
		k.Effect = ""
	}
}

func equalPV(a, b pv) bool { return a.N == b.N && a.M == b.M && a.S == b.S }

// firstAfter is the index of the first key with At > t.
func firstAfter(ks []trace.Key, t int64) int {
	return sort.Search(len(ks), func(i int) bool { return ks[i].At > t })
}

// prevIdx is the index of the last key with p and At <= t, or -1.
func prevIdx(ks []trace.Key, p uint32, t int64) int {
	for i := firstAfter(ks, t) - 1; i >= 0; i-- {
		if ks[i].Set&p != 0 {
			return i
		}
	}
	return -1
}

// nextIdx is the index of the first key with p and At > t, or -1.
func nextIdx(ks []trace.Key, p uint32, t int64) int {
	for i := firstAfter(ks, t); i < len(ks); i++ {
		if ks[i].Set&p != 0 {
			return i
		}
	}
	return -1
}

// ease is the motion curve E of the theme: smootherstep, or the back-out
// spring E(u) = 1 + (c+1)(u-1)^3 + c(u-1)^2 with c = 1.70158.
func ease(spring bool, u float64) float64 {
	if !spring {
		return smootherstep(u)
	}
	if u <= 0 {
		return 0
	}
	if u >= 1 {
		return 1
	}
	const c = 1.70158
	v := u - 1
	return 1 + float64(float64(c+1)*float64(float64(v*v)*v)) + float64(c*float64(v*v))
}

// valueAt is the value of p at time t: numbers and colors interpolated
// between the surrounding keys (numbers rounded to 0.01) with the curve of
// the theme for x, y, scale and rotation and smootherstep for the others;
// text, effect and the step properties hold. Before any key it is the first
// key's value; with no key at all, the default.
func (w *world) valueAt(ks []trace.Key, p uint32, t int64) pv {
	return valueAtE(ks, p, t, w.spring)
}

func valueAtE(ks []trace.Key, p uint32, t int64, spring bool) pv {
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
	e := ease(spring && p&easedProps != 0, u)
	return pv{N: rnd(a.N + float64(float64(b.N-a.N)*e))}
}

// finalVal is the value of p after its last key (its resting value).
func finalVal(ks []trace.Key, p uint32) (pv, bool) {
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

func (w *world) putProp(s *shape, at int64, p uint32, v pv) {
	setPV(w.keyAt(s, at), p, v)
}

// removeAfter clears p from every key after t0, dropping keys left empty.
func (w *world) removeAfter(s *shape, p uint32, t0 int64) {
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
func (w *world) writeProp(s *shape, p uint32, v pv, t0, d int64) {
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
		if !implicit(p) {
			// fill and stroke have no value before their first key: snap
			w.putProp(s, t0, p, v)
			return
		}
		// scale, rotation, draw and halo implicitly hold their default
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
		hold = w.valueAt(ks, p, t0)
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
