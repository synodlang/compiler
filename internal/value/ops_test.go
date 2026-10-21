package value

import (
	"math"
	"testing"

	"github.com/synodlang/compiler/internal/ir"
	"github.com/synodlang/compiler/internal/source"
)

func TestArithOps(t *testing.T) {
	var p ir.Pos
	if v, f := Arith(ir.Plus, Number(1), Number(2), p, p); f != nil || v.Num != 3 {
		t.Fatal(v, f)
	}
	if _, f := Arith(ir.Divide, Number(1), Number(0), p, p); f == nil || f.Code != source.DivisionByZero {
		t.Fatal(f)
	}
	if _, f := Arith(ir.Times, Number(math.MaxFloat64), Number(2), p, p); f == nil || f.Code != source.NumberRange {
		t.Fatal(f)
	}
	if _, f := Arith(ir.Plus, Time(math.MaxInt64), Time(1), p, p); f == nil || f.Code != source.NumberRange {
		t.Fatal(f)
	}
	if v, f := Arith(ir.Times, Time(1500), Number(2), p, p); f != nil || v.Int != 3000 {
		t.Fatal(v, f)
	}
}

func TestItemOps(t *testing.T) {
	var p ir.Pos
	l := ListOf([]Value{Text("a"), Text("b")})
	if v, f := ItemOf(l, Number(1), false, p, p); f != nil || v.Str != "b" {
		t.Fatal(v, f)
	}
	if _, f := ItemOf(l, Number(2), false, p, p); f == nil || f.Code != source.IndexRange {
		t.Fatal(f)
	}
	m := MapValue((&Map{}).With(Text("k"), Number(1)))
	if _, f := ItemOf(m, Text("x"), true, p, p); f == nil || f.Code != source.MissingKey {
		t.Fatal(f)
	}
	nl, f := SetItemOf(l, Number(0), Text("z"), false, p, p)
	if f != nil || nl.List().Items[0].Str != "z" || l.List().Items[0].Str != "a" {
		t.Fatal(nl, f)
	}
	if _, f := SetItemOf(l, Number(-1), Text("z"), false, p, p); f == nil {
		t.Fatal("no fault")
	}
}

func TestShapeColorKinds(t *testing.T) {
	if !Equal(Shape(3), Shape(3)) || Equal(Shape(3), Shape(4)) || Compare(Shape(1), Shape(2)) >= 0 {
		t.Error("shape order")
	}
	if !Equal(Color("Accent"), Color("Accent")) || Compare(Color("A"), Color("B")) >= 0 {
		t.Error("color order")
	}
	if Format(Shape(1), FormatOpts{}) != "<shape>" || Format(Color("Accent"), FormatOpts{}) != "Accent" {
		t.Error("format")
	}
}
