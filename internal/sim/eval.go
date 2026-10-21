package sim

import (
	"fmt"

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
	return value.NewFault(code, p, format, a...)
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
			return value.Value{}, fault(source.Nobody, x.Pos(), "node field %s of sim.Nobody", x.Field.Name).
				WithAlso(x.Node.Pos(), "this is sim.Nobody")
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
		return value.ItemOf(c, k, x.IsMap, x.Pos(), x.Key.Pos())
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

func (s *sim) arith(e *ex, x *ir.Arith) (value.Value, *value.Fault) {
	a, f := s.eval(e, x.A)
	if f != nil {
		return a, f
	}
	b, f := s.eval(e, x.B)
	if f != nil {
		return b, f
	}
	return value.Arith(x.Op, a, b, x.Pos(), x.B.Pos())
}
