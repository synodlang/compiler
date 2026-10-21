package value

import (
	"fmt"
	"math"

	"github.com/synodlang/compiler/internal/ir"
	"github.com/synodlang/compiler/internal/source"
)

// NewFault returns a fault with a formatted message at p.
func NewFault(code source.Code, p ir.Pos, format string, a ...any) *Fault {
	return &Fault{Code: code, Message: fmt.Sprintf(format, a...), Pos: p, HasPos: true}
}

// MapSize is the note that tells how many keys a map has.
func MapSize(n int) string {
	switch n {
	case 0:
		return "the map is empty"
	case 1:
		return "the map has 1 key"
	}
	return fmt.Sprintf("the map has %d keys", n)
}

// IndexFault adds the labels of an index out of range: the index at p, and
// the valid indexes of a list of n items.
func IndexFault(f *Fault, p ir.Pos, k Value, n int) *Fault {
	f.WithAlso(p, "index "+FormatNumber(k.Num))
	switch n {
	case 0:
		return f.WithNote("the list is empty")
	case 1:
		return f.WithNote("the only valid index is 0")
	}
	return f.WithNote(fmt.Sprintf("valid indexes are 0 to %d", n-1))
}

// Plural is "1 item" or "n items".
func Plural(n int) string {
	if n == 1 {
		return "1 item"
	}
	return fmt.Sprintf("%d items", n)
}

// NumResult is the number f, or a number-range fault at p when f is not
// finite.
func NumResult(f float64, p ir.Pos) (Value, *Fault) {
	if math.IsInf(f, 0) || math.IsNaN(f) {
		return Value{}, NewFault(source.NumberRange, p, "the result is too large for a number").WithLabel("too large")
	}
	return Number(f), nil
}

// TimeResult is the time of f microseconds (floored), or a number-range
// fault at p when it does not fit.
func TimeResult(f float64, p ir.Pos) (Value, *Fault) {
	f = math.Floor(f)
	if !(f >= -9.223372036854775808e18 && f < 9.223372036854775808e18) {
		return Value{}, NewFault(source.NumberRange, p, "the result is too large for a number").WithLabel("too large")
	}
	return Time(int64(f)), nil
}

// Index reports k as an index into n items.
func Index(k Value, n int) (int, bool) {
	if k.Num != math.Floor(k.Num) || k.Num < 0 || k.Num >= float64(n) {
		return 0, false
	}
	return int(k.Num), true
}

// Arith applies op to a and b. p is the position of the expression and bp
// that of its right operand.
func Arith(op ir.ArithOp, a, b Value, p, bp ir.Pos) (Value, *Fault) {
	switch {
	case a.K == KNumber && b.K == KNumber:
		switch op {
		case ir.Plus:
			return NumResult(a.Num+b.Num, p)
		case ir.Minus:
			return NumResult(a.Num-b.Num, p)
		case ir.Times:
			return NumResult(a.Num*b.Num, p)
		}
		if b.Num == 0 {
			return Value{}, NewFault(source.DivisionByZero, p, "division by zero").WithAlso(bp, "this is 0")
		}
		return NumResult(a.Num/b.Num, p)
	case a.K == KTime && b.K == KTime:
		switch op {
		case ir.Plus:
			r := a.Int + b.Int
			if (a.Int > 0 && b.Int > 0 && r < 0) || (a.Int < 0 && b.Int < 0 && r >= 0) {
				return Value{}, NewFault(source.NumberRange, p, "the result is too large for a number").WithLabel("too large")
			}
			return Time(r), nil
		case ir.Minus:
			r := a.Int - b.Int
			if (a.Int >= 0 && b.Int < 0 && r < 0) || (a.Int < 0 && b.Int > 0 && r >= 0) {
				return Value{}, NewFault(source.NumberRange, p, "the result is too large for a number").WithLabel("too large")
			}
			return Time(r), nil
		case ir.Divide:
			if b.Int == 0 {
				return Value{}, NewFault(source.DivisionByZero, p, "division by zero").WithAlso(bp, "this is 0s")
			}
			return NumResult(float64(a.Int)/float64(b.Int), p)
		}
	case a.K == KTime && b.K == KNumber:
		switch op {
		case ir.Times:
			return TimeResult(float64(float64(a.Int)*b.Num), p)
		case ir.Divide:
			if b.Num == 0 {
				return Value{}, NewFault(source.DivisionByZero, p, "division by zero").WithAlso(bp, "this is 0")
			}
			return TimeResult(float64(float64(a.Int)/b.Num), p)
		}
	case a.K == KNumber && b.K == KTime && op == ir.Times:
		return TimeResult(float64(a.Num*float64(b.Int)), p)
	}
	panic("value: ill-typed arithmetic")
}

// ItemOf is "item k of c" for a list or (isMap) a map. p is the position of
// the expression and kp that of the key.
func ItemOf(c, k Value, isMap bool, p, kp ir.Pos) (Value, *Fault) {
	if isMap {
		v, ok := c.Map().Get(k)
		if !ok {
			return v, NewFault(source.MissingKey, p, "key %s is not in the map", Format(k, FormatOpts{})).
				WithAlso(kp, "not in the map").
				WithNote(MapSize(c.Map().Len()))
		}
		return v, nil
	}
	items := c.List().Items
	i, ok := Index(k, len(items))
	if !ok {
		return k, IndexFault(NewFault(source.IndexRange, p, "index %s is out of range for a list of %s", FormatNumber(k.Num), Plural(len(items))),
			kp, k, len(items))
	}
	return items[i], nil
}

// SetItemOf returns the list or map cur with item k set to x. p is the
// position of the statement and kp that of the key.
func SetItemOf(cur, k, x Value, isMap bool, p, kp ir.Pos) (Value, *Fault) {
	if isMap {
		return MapValue(cur.Map().With(k, x)), nil
	}
	items := cur.List().Items
	i, ok := Index(k, len(items))
	if !ok {
		return Value{}, IndexFault(NewFault(source.IndexRange, p, "index %s is out of range for a list of %s", FormatNumber(k.Num), Plural(len(items))),
			kp, k, len(items))
	}
	cp := append([]Value(nil), items...)
	cp[i] = x
	return ListOf(cp), nil
}
