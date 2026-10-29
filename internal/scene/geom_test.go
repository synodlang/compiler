package scene

import (
	"bytes"
	"fmt"
	"math"
	"strings"
	"testing"

	"github.com/synodlang/compiler/internal/trace"
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
	x, y := orbitPoint(CenterX, CenterY, 0.25)
	if x != CenterX+OrbitRadius || y != CenterY || math.Signbit(y) {
		t.Fatalf("orbit 0.25 = %v, %v", x, y)
	}
	x, y = orbitPoint(CenterX, CenterY, 0)
	if x != CenterX || y != CenterY-OrbitRadius {
		t.Fatalf("orbit 0 = %v, %v", x, y)
	}
}

func TestSmootherstep(t *testing.T) {
	if smootherstep(0) != 0 || smootherstep(1) != 1 || smootherstep(0.5) != 0.5 {
		t.Fatal("smootherstep endpoints")
	}
}

// home is the key text of a shape at the scene centre.
var home = fmt.Sprintf("x=%v y=%v", CenterX, CenterY)

// TestCanvasInScene: the canvas written by the trace encoder (which cannot
// import this package) is the canvas of the layout constants.
func TestCanvasInScene(t *testing.T) {
	var b bytes.Buffer
	if err := trace.EncodeScene(&b, &trace.Scene{}); err != nil {
		t.Fatal(err)
	}
	want := fmt.Sprintf(`"canvas":{"w":%d,"h":%d}`, CanvasW, CanvasH)
	if !strings.Contains(b.String(), want) {
		t.Errorf("scene %s lacks %s", b.String(), want)
	}
	if CenterX != CanvasW/2 || CenterY != CanvasH/2 {
		t.Errorf("centre (%v, %v) is not the canvas centre", CenterX, CenterY)
	}
}
