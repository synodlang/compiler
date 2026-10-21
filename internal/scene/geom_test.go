package scene

import (
	"math"
	"testing"
)

// The golden table pins the exact output of sinTurn and cosTurn so that the
// scene bytes cannot change between platforms or releases.
var trigGolden = []struct{ f, sin, cos float64 }{
	{0, 0, 1},
	{0.0625, 0.3826834323650898, 0.9238795325112867},
	{0.125, 0.7071067811865476, 0.7071067811865475},
	{0.1875, 0.9238795325112867, 0.3826834323650898},
	{0.25, 1, 0},
	{0.3125, 0.9238795325112867, -0.3826834323650898},
	{0.375, 0.7071067811865475, -0.7071067811865476},
	{0.4375, 0.3826834323650898, -0.9238795325112867},
	{0.5, 0, -1},
	{0.5625, -0.3826834323650898, -0.9238795325112867},
	{0.625, -0.7071067811865476, -0.7071067811865475},
	{0.6875, -0.9238795325112867, -0.3826834323650898},
	{0.75, -1, 0},
	{0.8125, -0.9238795325112867, 0.3826834323650898},
	{0.875, -0.7071067811865475, 0.7071067811865476},
	{0.9375, -0.3826834323650898, 0.9238795325112867},
	{1, 0, 1},
	{0.1, 0.5877852522924731, 0.8090169943749475},
	{0.2, 0.9510565162951536, 0.30901699437494734},
	{0.3, 0.9510565162951536, -0.30901699437494734},
	{1.0 / 3, 0.8660254037844387, -0.49999999999999983},
	{0.7, -0.9510565162951535, -0.3090169943749477},
	{0.9, -0.587785252292473, 0.8090169943749475},
	{-0.25, -1, 0},
	{2.75, -1, 0},
	{0.123456, 0.7002138034600163, 0.7139332107725888},
}

func TestTrigGolden(t *testing.T) {
	for _, g := range trigGolden {
		s, c := sinCosTurn(g.f)
		if s != g.sin || c != g.cos {
			t.Errorf("f=%v: got (%v, %v), want (%v, %v)", g.f, s, c, g.sin, g.cos)
		}
	}
}

func TestTrigAccuracy(t *testing.T) {
	worst := 0.0
	for k := -4000; k <= 8000; k++ {
		f := float64(k) / 3989 // not a nice fraction
		s, c := sinCosTurn(f)
		ws, wc := math.Sin(2*math.Pi*(f-math.Floor(f))), math.Cos(2*math.Pi*(f-math.Floor(f)))
		worst = math.Max(worst, math.Max(math.Abs(s-ws), math.Abs(c-wc)))
	}
	if worst > 1e-12 {
		t.Fatalf("worst error %g", worst)
	}
}

func TestRoundNoNegativeZero(t *testing.T) {
	if r := rnd(-0.001); math.Signbit(r) || r != 0 {
		t.Fatalf("rnd(-0.001) = %v", r)
	}
	x, y := orbitPoint(800, 500, 0.25)
	if x != 1100 || y != 500 || math.Signbit(y) {
		t.Fatalf("orbit 0.25 = %v, %v", x, y)
	}
	x, y = orbitPoint(800, 500, 0)
	if x != 800 || y != 200 {
		t.Fatalf("orbit 0 = %v, %v", x, y)
	}
}

func TestSmootherstep(t *testing.T) {
	if smootherstep(0) != 0 || smootherstep(1) != 1 || smootherstep(0.5) != 0.5 {
		t.Fatal("smootherstep endpoints")
	}
}
