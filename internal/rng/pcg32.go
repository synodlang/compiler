// Package rng is the simulator's pseudo-random number generator: PCG32
// (XSH RR, 64-bit state, 32-bit output) after M.E. O'Neill's reference
// implementation. It is deterministic and uses only integer arithmetic for
// its core, so a seed gives the same numbers on every platform.
//
// Every method that draws consumes exactly one Next, whatever its arguments.
package rng

import "math/bits"

const multiplier = 6364136223846793005

// PCG32 is a PCG32 generator on stream 54. The zero value is not seeded; use
// New.
type PCG32 struct{ state, inc uint64 }

// New returns a generator seeded like pcg32_srandom_r(seed, 54).
func New(seed uint64) *PCG32 {
	r := &PCG32{inc: (54 << 1) | 1}
	r.Next()
	r.state += seed
	r.Next()
	return r
}

// Next returns the next 32 random bits.
func (r *PCG32) Next() uint32 {
	old := r.state
	r.state = old*multiplier + r.inc
	x := uint32(((old >> 18) ^ old) >> 27)
	rot := uint32(old >> 59)
	return x>>rot | x<<((-rot)&31)
}

// Float returns a number in [0, 1) with 32 random bits; one draw.
func (r *PCG32) Float() float64 { return float64(r.Next()) / 4294967296 }

// Between returns a + (b-a)*Float(); one draw. The product is rounded to
// float64 before the sum, so the result does not depend on fused
// multiply-add.
func (r *PCG32) Between(a, b float64) float64 {
	return a + float64(float64(b-a)*r.Float())
}

// TimeBetween returns an integer in [a, b) (a when a == b), exactly
// a + floor((b-a)*u/2^32) for the drawn 32-bit u; one draw. The caller
// ensures a <= b.
func (r *PCG32) TimeBetween(a, b int64) int64 {
	d := uint64(b - a)
	hi, lo := bits.Mul64(d, uint64(r.Next()))
	return a + int64(hi<<32|lo>>32)
}

// Index returns an integer in [0, n) (0 when n is 0), exactly
// floor(n*u/2^32); one draw.
func (r *PCG32) Index(n int) int {
	return int((uint64(n) * uint64(r.Next())) >> 32)
}

// Chance reports Float() < p; one draw, for any p.
func (r *PCG32) Chance(p float64) bool { return r.Float() < p }
