package scene

import (
	"fmt"

	"github.com/synodlang/compiler/internal/ir"
	"github.com/synodlang/compiler/internal/source"
	"github.com/synodlang/compiler/internal/types"
	"github.com/synodlang/compiler/internal/value"
)

// ex is the frame of a body that is running.
type ex struct{ fr []value.Value }

func fault(code source.Code, p ir.Pos, format string, a ...any) *value.Fault {
	return value.NewFault(code, p, format, a...)
}

// initVal is the initial value of a field (a constant expression), for a
// record made without it.
func (b *builder) initVal(f *ir.Field) (value.Value, *value.Fault) {
	if v, ok := b.inits[f]; ok {
		return v, nil
	}
	if f.Init == nil {
		return value.Value{}, nil
	}
	v, flt := b.eval(&ex{}, f.Init)
	if flt != nil {
		return value.Value{}, flt
	}
	b.inits[f] = v
	return v, nil
}

// attKeyOf is the attachment key of a node, text or number value.
func attKeyOf(v value.Value) attKey {
	switch v.K {
	case value.KNode:
		return nodeKey(v.NodeID())
	case value.KNumber:
		return numberKey(v.Num)
	}
	return textKey(v.Str)
}

var colorType = types.TColor

func (b *builder) eval(e *ex, x ir.Expr) (value.Value, *value.Fault) {
	switch x := x.(type) {
	case *ir.Const:
		switch v := x.V.(type) {
		case string:
			if types.Equal(x.Type(), colorType) {
				return value.Color(b.colorName(v)), nil
			}
			return value.Text(v), nil
		case float64:
			return value.Number(v), nil
		case bool:
			return value.Bool(v), nil
		case int64:
			return value.Time(v), nil
		}
		panic(fmt.Sprintf("scene: constant of type %T", x.V))
	case *ir.Empty:
		if _, ok := x.Ty.(types.Map); ok {
			return value.MapValue(&value.Map{}), nil
		}
		return value.ListOf(nil), nil
	case *ir.ListLit:
		items := make([]value.Value, len(x.Items))
		for i, it := range x.Items {
			v, f := b.eval(e, it)
			if f != nil {
				return v, f
			}
			items[i] = v
		}
		return value.ListOf(items), nil
	case *ir.Load:
		return e.fr[x.Var.Slot], nil
	case *ir.NodeField:
		nv, f := b.eval(e, x.Node)
		if f != nil {
			return nv, f
		}
		if nv.NodeID() == 0 {
			return value.Value{}, fault(source.Nobody, x.Pos(), "node field %s of sim.Nobody", x.Field.Name).
				WithAlso(x.Node.Pos(), "this is sim.Nobody")
		}
		b.read(readKey{int32(nv.NodeID()), b.store.slotOf(x.Field)})
		if v, ok := b.store.field(nv.NodeID(), x.Field); ok {
			return v, nil
		}
		return b.initVal(x.Field)
	case *ir.RecordField:
		rv, f := b.eval(e, x.Rec)
		if f != nil {
			return rv, f
		}
		return rv.Record().Fields[x.Field.Index], nil
	case *ir.MakeRecord:
		fs := make([]value.Value, len(x.Record.Fields))
		for i := range fs {
			if i < len(x.Values) && x.Values[i] != nil {
				v, f := b.eval(e, x.Values[i])
				if f != nil {
					return v, f
				}
				fs[i] = v
			} else {
				v, f := b.initVal(x.Record.Fields[i])
				if f != nil {
					return v, f
				}
				fs[i] = v
			}
		}
		return value.RecordOf(x.Record, fs), nil
	case *ir.Item:
		c, f := b.eval(e, x.Coll)
		if f != nil {
			return c, f
		}
		k, f := b.eval(e, x.Key)
		if f != nil {
			return k, f
		}
		return value.ItemOf(c, k, x.IsMap, x.Pos(), x.Key.Pos())
	case *ir.Arith:
		a, f := b.eval(e, x.A)
		if f != nil {
			return a, f
		}
		c, f := b.eval(e, x.B)
		if f != nil {
			return c, f
		}
		return value.Arith(x.Op, a, c, x.Pos(), x.B.Pos())
	case *ir.Compare:
		a, f := b.eval(e, x.A)
		if f != nil {
			return a, f
		}
		c, f := b.eval(e, x.B)
		if f != nil {
			return c, f
		}
		switch x.Op {
		case ir.Eq:
			return value.Bool(value.Equal(a, c)), nil
		case ir.Ne:
			return value.Bool(!value.Equal(a, c)), nil
		}
		r := value.Compare(a, c)
		switch x.Op {
		case ir.Lt:
			return value.Bool(r < 0), nil
		case ir.Gt:
			return value.Bool(r > 0), nil
		case ir.Le:
			return value.Bool(r <= 0), nil
		}
		return value.Bool(r >= 0), nil
	case *ir.Contains:
		el, f := b.eval(e, x.Elem)
		if f != nil {
			return el, f
		}
		c, f := b.eval(e, x.Coll)
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
		a, f := b.eval(e, x.A)
		if f != nil {
			return a, f
		}
		if a.Bool() != x.And {
			return a, nil
		}
		return b.eval(e, x.B)
	case *ir.Not:
		a, f := b.eval(e, x.X)
		if f != nil {
			return a, f
		}
		return value.Bool(!a.Bool()), nil
	case *ir.Call:
		return b.call(e, x)
	case *ir.PackageVal:
		return value.PackageOf(x.Pkg), nil
	case *ir.FieldVal:
		return value.FieldOf(x.Field), nil
	case *ir.Attachment:
		kv, f := b.eval(e, x.Key)
		if f != nil {
			return kv, f
		}
		if i, ok := b.world.lookup(x.Name, attKeyOf(kv)); ok {
			return value.Shape(i), nil
		}
		return value.Value{}, fault(source.MissingKey, x.Pos(), "no shape is attached to %s under %q",
			value.Format(kv, value.FormatOpts{}), x.Name).
			WithLabel("nothing is attached here").
			WithAlso(x.Key.Pos(), "this key")
	}
	panic(fmt.Sprintf("scene: expression %T is not available in a look", x))
}
