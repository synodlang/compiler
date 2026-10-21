package natives

import (
	"math"

	"github.com/synodlang/compiler/internal/ir"
	"github.com/synodlang/compiler/internal/source"
	"github.com/synodlang/compiler/internal/value"
)

func init() {
	Table["math.floor"] = func(h Host, c *ir.Call, args []value.Value) (value.Value, *value.Fault) {
		return value.Number(math.Floor(args[0].Num)), nil
	}
	Table["math.ceil"] = func(h Host, c *ir.Call, args []value.Value) (value.Value, *value.Fault) {
		return value.Number(math.Ceil(args[0].Num)), nil
	}
	Table["math.min"] = func(h Host, c *ir.Call, args []value.Value) (value.Value, *value.Fault) {
		return value.Number(math.Min(args[0].Num, args[1].Num)), nil
	}
	Table["math.max"] = func(h Host, c *ir.Call, args []value.Value) (value.Value, *value.Fault) {
		return value.Number(math.Max(args[0].Num, args[1].Num)), nil
	}
	Table["math.remainder"] = mathRemainder
	Table["math.hash"] = mathHash
}

func mathRemainder(h Host, c *ir.Call, args []value.Value) (value.Value, *value.Fault) {
	a, b := args[0].Num, args[1].Num
	if b == 0 {
		return value.Value{}, fault(c, source.DivisionByZero, "division by zero")
	}
	// The product is rounded before the subtraction (no fused multiply-add).
	r := a - float64(b*math.Floor(a/b))
	if math.IsInf(r, 0) || math.IsNaN(r) {
		return value.Value{}, fault(c, source.NumberRange, "the result is too large for a number")
	}
	return value.Number(r), nil
}

// mathHash is FNV-1a 64 over the UTF-8 bytes, mixed with the MurmurHash3
// 64-bit finalizer so that similar texts spread, scaled to [0, 1) by keeping
// the top 53 bits.
func mathHash(h Host, c *ir.Call, args []value.Value) (value.Value, *value.Fault) {
	const (
		offset = 14695981039346656037
		prime  = 1099511628211
	)
	s := args[0].Str
	sum := uint64(offset)
	for i := 0; i < len(s); i++ {
		sum ^= uint64(s[i])
		sum *= prime
	}
	sum ^= sum >> 33
	sum *= 0xff51afd7ed558ccd
	sum ^= sum >> 33
	sum *= 0xc4ceb9fe1a85ec53
	sum ^= sum >> 33
	return value.Number(float64(sum>>11) / (1 << 53)), nil
}
