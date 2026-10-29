// Package scene builds the presentation of a run: a trace.Scene of shapes on
// a timeline of integer milliseconds. This file set is the engine core: the
// replay store (store.go), the presentation clock (clock.go), the shape world
// with timelines, constraints, groups, attachments and flights (world.go),
// layout (layout.go), geometry (geom.go), shape ids (ids.go) and the
// conversion to a trace.Scene (emit.go). The look interpreter builds on them.
//
// Determinism rules for the whole package: presentation time is int64 ms;
// pace is an integer in thousandths; no map iteration reaches an output; no
// math.Sin or math.Cos (their results may differ between platforms); every
// product that is added to something is wrapped in float64(...) so that no
// platform fuses it into an FMA; computed numbers are rounded to 0.01 (rnd).
package scene

import "math"

// Fixed numbers of the scene (spec/09, plan 2.4 and 2.6), in one place.
const (
	GapScale        = 8     // idle gap between facts is multiplied by this
	MaxIdle         = 600   // ms: longest idle presentation gap (before pace)
	MinFlight       = 400   // ms: a landing is this long after its send
	MinStep         = 30    // ms: least distance between successive slots
	Morph           = 300   // ms: morph and fade duration
	TimerTail       = 2000  // ms: timers run this long after the last fact
	ProportionalLen = 30000 // ms: length of a proportional scene
	CanvasW         = 1040
	CanvasH         = 650
	CenterX         = 520.0
	CenterY         = 325.0
	OrbitRadius     = 200.0
	LayoutCY        = 315.0 // ring, row and grid centre: state rows hang below the cards
	RingRX          = 380.0 // ring ellipse radii at layout scale 1 and growth 1
	RingRY          = 170.0
	RingGrowMax     = 1.4 // largest ring growth factor g
	PlaceGap        = 8.0
	LabelCharW      = 8.0
	LabelH          = 18.0
	StateRowMax     = 24 // characters (runes) of one sim.StateRows line
)

// rnd rounds a computed number to 0.01 and never returns -0.
func rnd(v float64) float64 {
	r := math.Round(float64(v*100)) / 100
	if r == 0 {
		return 0
	}
	return r
}

// smootherstep is the house easing curve of u in [0, 1].
func smootherstep(u float64) float64 {
	inner := float64(u*float64(float64(u*6)-15)) + 10
	return float64(float64(u*u) * float64(u*inner))
}

const quarterOctant = 0.78539816339744830961566084581988 // pi/4

// Taylor coefficients (alternating signs included) in a*a, exact constants.
var sinCoef = [...]float64{
	-1.0 / 6, 1.0 / 120, -1.0 / 5040, 1.0 / 362880,
	-1.0 / 39916800, 1.0 / 6227020800, -1.0 / 1307674368000, 1.0 / 355687428096000,
}
var cosCoef = [...]float64{
	-1.0 / 2, 1.0 / 24, -1.0 / 720, 1.0 / 40320,
	-1.0 / 3628800, 1.0 / 479001600, -1.0 / 87178291200, 1.0 / 20922789888000,
}

// sinPoly and cosPoly are sin(a) and cos(a) for a in [0, pi/4].
func sinPoly(a float64) float64 {
	a2 := float64(a * a)
	p := sinCoef[len(sinCoef)-1]
	for i := len(sinCoef) - 2; i >= 0; i-- {
		p = sinCoef[i] + float64(a2*p)
	}
	return a + float64(a*float64(a2*p))
}

func cosPoly(a float64) float64 {
	a2 := float64(a * a)
	p := cosCoef[len(cosCoef)-1]
	for i := len(cosCoef) - 2; i >= 0; i-- {
		p = cosCoef[i] + float64(a2*p)
	}
	return 1 + float64(a2*p)
}

// sinCosTurn returns sin and cos of 2*pi*f, f a fraction of a turn. The
// reduction f - floor(f) and the octant split are exact in float64.
func sinCosTurn(f float64) (sin, cos float64) {
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return 0, 1
	}
	r := f - math.Floor(f)
	r8 := float64(r * 8)
	n := int(math.Floor(r8))
	if n > 7 {
		n = 7
	}
	frac := r8 - float64(n)
	a := float64(frac * quarterOctant)
	b := float64(float64(1-frac) * quarterOctant)
	switch n {
	case 0:
		return sinPoly(a), cosPoly(a)
	case 1:
		return cosPoly(b), sinPoly(b)
	case 2:
		return cosPoly(a), -sinPoly(a)
	case 3:
		return sinPoly(b), -cosPoly(b)
	case 4:
		return -sinPoly(a), -cosPoly(a)
	case 5:
		return -cosPoly(b), -sinPoly(b)
	case 6:
		return -cosPoly(a), sinPoly(a)
	default:
		return -sinPoly(b), cosPoly(b)
	}
}

// sinTurn is sin(2*pi*f); cosTurn is cos(2*pi*f).
func sinTurn(f float64) float64 { s, _ := sinCosTurn(f); return s }
func cosTurn(f float64) float64 { _, c := sinCosTurn(f); return c }

// orbitPoint is the point at fraction f of a turn on the circle of radius
// OrbitRadius around (cx, cy); 0 is 12 o'clock, clockwise. Rounded to 0.01.
func orbitPoint(cx, cy, f float64) (x, y float64) { return orbitPointR(cx, cy, f, OrbitRadius) }

// orbitPointR is orbitPoint on a circle of the given radius.
func orbitPointR(cx, cy, f, radius float64) (x, y float64) {
	s, c := sinCosTurn(f)
	return rnd(cx + float64(radius*s)), rnd(cy - float64(radius*c))
}

// ellipsePoint is the point at fraction f of a turn on the ellipse of radii
// rx and ry around (cx, cy); 0 is 12 o'clock, clockwise. Rounded to 0.01.
func ellipsePoint(cx, cy, f, rx, ry float64) (x, y float64) {
	s, c := sinCosTurn(f)
	return rnd(cx + float64(rx*s)), rnd(cy - float64(ry*c))
}
