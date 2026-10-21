package rng

import (
	"math/big"
	"math/rand"
	"testing"
)

// ref is a straightforward transcription of the reference C code.
type ref struct{ state, inc uint64 }

func (r *ref) random() uint32 {
	oldstate := r.state
	r.state = oldstate*6364136223846793005 + r.inc
	xorshifted := uint32(((oldstate >> 18) ^ oldstate) >> 27)
	rot := uint32(oldstate >> 59)
	return (xorshifted >> rot) | (xorshifted << ((^rot + 1) & 31))
}

func refSeed(initstate, initseq uint64) *ref {
	r := &ref{}
	r.state = 0
	r.inc = (initseq << 1) | 1
	r.random()
	r.state += initstate
	r.random()
	return r
}

func TestMatchesReference(t *testing.T) {
	for _, seed := range []uint64{0, 1, 42, 7, 1 << 53, 1<<64 - 1} {
		a, b := New(seed), refSeed(seed, 54)
		for i := 0; i < 1000; i++ {
			if x, y := a.Next(), b.random(); x != y {
				t.Fatalf("seed %d draw %d: %#x want %#x", seed, i, x, y)
			}
		}
	}
}

func TestVector(t *testing.T) {
	want := []uint32{0xa15c02b7, 0x7b47f409, 0xba1d3330, 0x83d2f293, 0xbfa4784b, 0xcbed606e}
	r, c := New(42), refSeed(42, 54)
	for i, w := range want {
		if g := r.Next(); g != w {
			t.Errorf("draw %d: %#x want %#x", i, g, w)
		}
		if g := c.random(); g != w {
			t.Errorf("reference draw %d: %#x want %#x", i, g, w)
		}
	}
}

func TestExactness(t *testing.T) {
	g := rand.New(rand.NewSource(1))
	a, probe := New(9), New(9)
	for i := 0; i < 20000; i++ {
		lo := g.Int63n(1<<40) - 1<<39
		d := g.Int63n(1 << 40)
		if i%7 == 0 {
			d = g.Int63()
			lo = 0
		}
		u := uint64(probe.Next())
		want := new(big.Int).Mul(big.NewInt(d), new(big.Int).SetUint64(u))
		want.Rsh(want, 32)
		got := a.TimeBetween(lo, lo+d)
		if got-lo != want.Int64() {
			t.Fatalf("TimeBetween(%d,%d) u=%d: %d want offset %d", lo, lo+d, u, got, want)
		}
		if d > 0 && (got < lo || got >= lo+d) {
			t.Fatalf("out of range")
		}
	}
	a, probe = New(5), New(5)
	for i := 0; i < 20000; i++ {
		n := int(g.Int63n(1 << 31))
		u := uint64(probe.Next())
		want := new(big.Int).Mul(big.NewInt(int64(n)), new(big.Int).SetUint64(u))
		want.Rsh(want, 32)
		got := a.Index(n)
		if int64(got) != want.Int64() || got < 0 || (n > 0 && got >= n) {
			t.Fatalf("Index(%d) = %d want %d", n, got, want)
		}
	}
	if New(1).Index(0) != 0 || New(1).TimeBetween(5, 5) != 5 {
		t.Error("degenerate ranges")
	}
}

func TestOneDrawEach(t *testing.T) {
	calls := []func(r *PCG32){
		func(r *PCG32) { r.Float() },
		func(r *PCG32) { r.Between(3, 3) },
		func(r *PCG32) { r.TimeBetween(4, 4) },
		func(r *PCG32) { r.Index(0) },
		func(r *PCG32) { r.Chance(0) },
		func(r *PCG32) { r.Chance(2) },
	}
	for i, f := range calls {
		r, c := New(3), New(3)
		f(r)
		c.Next()
		if r.Next() != c.Next() {
			t.Errorf("call %d did not consume exactly one draw", i)
		}
	}
	r := New(8)
	for i := 0; i < 1000; i++ {
		if f := r.Float(); f < 0 || f >= 1 {
			t.Fatal(f)
		}
		if v := r.Between(2, 5); v < 2 || v >= 5 {
			t.Fatal(v)
		}
	}
	if !New(1).Chance(1.5) || New(1).Chance(0) {
		t.Error("chance bounds")
	}
}
