package sim

import (
	"fmt"
	"math"

	"github.com/synodlang/compiler/internal/ir"
	"github.com/synodlang/compiler/internal/source"
	"github.com/synodlang/compiler/internal/types"
	"github.com/synodlang/compiler/internal/value"
)

type ctxKind uint8

const (
	kindNode ctxKind = iota
	kindMain
	kindCheck
)

// ex is the execution context of a body: its frame and the dynamic context
// (kind and current node, 0 for main).
type ex struct {
	fr   []value.Value
	node int
	kind ctxKind
}

func fault(code source.Code, p ir.Pos, format string, a ...any) *value.Fault {
	return &value.Fault{Code: code, Message: fmt.Sprintf(format, a...), Pos: p, HasPos: true}
}

func plural(n int) string {
	if n == 1 {
		return "1 item"
	}
	return fmt.Sprintf("%d items", n)
}

func numResult(f float64, p ir.Pos) (value.Value, *value.Fault) {
	if math.IsInf(f, 0) || math.IsNaN(f) {
		return value.Value{}, fault(source.NumberRange, p, "the result is too large for a number")
	}
	return value.Number(f), nil
}

func timeResult(f float64, p ir.Pos) (value.Value, *value.Fault) {
	f = math.Floor(f)
	if !(f >= -9.223372036854775808e18 && f < 9.223372036854775808e18) {
		return value.Value{}, fault(source.NumberRange, p, "the result is too large for a number")
	}
	return value.Time(int64(f)), nil
}

func (s *sim) eval(e *ex, x ir.Expr) (value.Value, *value.Fault) {
	switch x := x.(type) {
	case *ir.Const:
		switch v := x.V.(type) {
		case string:
			return value.Text(v), nil
		case float64:
			return value.Number(v), nil
		case bool:
			return value.Bool(v), nil
		case int64:
			return value.Time(v), nil
		}
		panic(fmt.Sprintf("sim: constant of type %T", x.V))
	case *ir.Empty:
		if _, ok := x.Ty.(types.Map); ok {
			return value.MapValue(&value.Map{}), nil
		}
		return value.ListOf(nil), nil
	case *ir.ListLit:
		items := make([]value.Value, len(x.Items))
		for i, it := range x.Items {
			v, f := s.eval(e, it)
			if f != nil {
				return v, f
			}
			items[i] = v
		}
		return value.ListOf(items), nil
	case *ir.Load:
		return e.fr[x.Var.Slot], nil
	case *ir.NodeField:
		nv, f := s.eval(e, x.Node)
		if f != nil {
			return nv, f
		}
		if nv.NodeID() == 0 {
			return value.Value{}, fault(source.Nobody, x.Pos(), "node field %s of sim.Nobody", x.Field.Name)
		}
		n := s.getNode(nv.NodeID())
		return n.state[n.index(x.Field.Pkg)][x.Field.Index], nil
	case *ir.RecordField:
		rv, f := s.eval(e, x.Rec)
		if f != nil {
			return rv, f
		}
		return rv.Record().Fields[x.Field.Index], nil
	case *ir.MakeRecord:
		fs := make([]value.Value, len(x.Record.Fields))
		for i := range fs {
			if i < len(x.Values) && x.Values[i] != nil {
				v, f := s.eval(e, x.Values[i])
				if f != nil {
					return v, f
				}
				fs[i] = v
			} else {
				v, f := s.initVal(x.Record.Fields[i])
				if f != nil {
					return v, f
				}
				fs[i] = v
			}
		}
		return value.RecordOf(x.Record, fs), nil
	case *ir.Item:
		c, f := s.eval(e, x.Coll)
		if f != nil {
			return c, f
		}
		k, f := s.eval(e, x.Key)
		if f != nil {
			return k, f
		}
		if x.IsMap {
			v, ok := c.Map().Get(k)
			if !ok {
				return v, fault(source.MissingKey, x.Pos(), "key %s is not in the map", value.Format(k, value.FormatOpts{}))
			}
			return v, nil
		}
		items := c.List().Items
		i, ok := index(k, len(items))
		if !ok {
			return k, fault(source.IndexRange, x.Pos(), "index %s is out of range for a list of %s", value.FormatNumber(k.Num), plural(len(items)))
		}
		return items[i], nil
	case *ir.Arith:
		return s.arith(e, x)
	case *ir.Compare:
		a, f := s.eval(e, x.A)
		if f != nil {
			return a, f
		}
		b, f := s.eval(e, x.B)
		if f != nil {
			return b, f
		}
		switch x.Op {
		case ir.Eq:
			return value.Bool(value.Equal(a, b)), nil
		case ir.Ne:
			return value.Bool(!value.Equal(a, b)), nil
		}
		c := value.Compare(a, b)
		switch x.Op {
		case ir.Lt:
			return value.Bool(c < 0), nil
		case ir.Gt:
			return value.Bool(c > 0), nil
		case ir.Le:
			return value.Bool(c <= 0), nil
		}
		return value.Bool(c >= 0), nil
	case *ir.Contains:
		el, f := s.eval(e, x.Elem)
		if f != nil {
			return el, f
		}
		c, f := s.eval(e, x.Coll)
		if f != nil {
			return c, f
		}
		found := false
		if x.IsMap {
			_, found = c.Map().Get(el)
		} else {
			for _, it := range c.List().Items {
				if value.Equal(it, el) {
					found = true
					break
				}
			}
		}
		return value.Bool(found != x.Not), nil
	case *ir.Logic:
		a, f := s.eval(e, x.A)
		if f != nil {
			return a, f
		}
		if a.Bool() != x.And {
			return a, nil
		}
		return s.eval(e, x.B)
	case *ir.Not:
		a, f := s.eval(e, x.X)
		if f != nil {
			return a, f
		}
		return value.Bool(!a.Bool()), nil
	case *ir.Call:
		return s.call(e, x)
	case *ir.PackageVal:
		return value.PackageOf(x.Pkg), nil
	case *ir.FieldVal:
		return value.FieldOf(x.Field), nil
	}
	panic(fmt.Sprintf("sim: expression %T is not available in a simulation", x))
}

// index reports k as an index into n items.
func index(k value.Value, n int) (int, bool) {
	if k.Num != math.Floor(k.Num) || k.Num < 0 || k.Num >= float64(n) {
		return 0, false
	}
	return int(k.Num), true
}

func (s *sim) arith(e *ex, x *ir.Arith) (value.Value, *value.Fault) {
	a, f := s.eval(e, x.A)
	if f != nil {
		return a, f
	}
	b, f := s.eval(e, x.B)
	if f != nil {
		return b, f
	}
	p := x.Pos()
	switch {
	case a.K == value.KNumber && b.K == value.KNumber:
		switch x.Op {
		case ir.Plus:
			return numResult(a.Num+b.Num, p)
		case ir.Minus:
			return numResult(a.Num-b.Num, p)
		case ir.Times:
			return numResult(a.Num*b.Num, p)
		}
		if b.Num == 0 {
			return value.Value{}, fault(source.DivisionByZero, p, "division by zero")
		}
		return numResult(a.Num/b.Num, p)
	case a.K == value.KTime && b.K == value.KTime:
		switch x.Op {
		case ir.Plus:
			r := a.Int + b.Int
			if (a.Int > 0 && b.Int > 0 && r < 0) || (a.Int < 0 && b.Int < 0 && r >= 0) {
				return value.Value{}, fault(source.NumberRange, p, "the result is too large for a number")
			}
			return value.Time(r), nil
		case ir.Minus:
			r := a.Int - b.Int
			if (a.Int >= 0 && b.Int < 0 && r < 0) || (a.Int < 0 && b.Int > 0 && r >= 0) {
				return value.Value{}, fault(source.NumberRange, p, "the result is too large for a number")
			}
			return value.Time(r), nil
		case ir.Divide:
			if b.Int == 0 {
				return value.Value{}, fault(source.DivisionByZero, p, "division by zero")
			}
			return numResult(float64(a.Int)/float64(b.Int), p)
		}
	case a.K == value.KTime && b.K == value.KNumber:
		switch x.Op {
		case ir.Times:
			return timeResult(float64(float64(a.Int)*b.Num), p)
		case ir.Divide:
			if b.Num == 0 {
				return value.Value{}, fault(source.DivisionByZero, p, "division by zero")
			}
			return timeResult(float64(float64(a.Int)/b.Num), p)
		}
	case a.K == value.KNumber && b.K == value.KTime && x.Op == ir.Times:
		return timeResult(float64(a.Num*float64(b.Int)), p)
	}
	panic("sim: ill-typed arithmetic")
}
