package sim

import (
	"fmt"

	"github.com/synodlang/compiler/internal/ir"
	"github.com/synodlang/compiler/internal/natives"
	"github.com/synodlang/compiler/internal/source"
	"github.com/synodlang/compiler/internal/trace"
	"github.com/synodlang/compiler/internal/value"
)

// execBlock runs statements; returned reports a Result.
func (s *sim) execBlock(e *ex, b *ir.Block) (bool, value.Value, *value.Fault) {
	for _, st := range b.Stmts {
		ret, v, f := s.exec(e, st)
		if f != nil || ret {
			return ret, v, f
		}
	}
	return false, value.Value{}, nil
}

func (s *sim) exec(e *ex, st ir.Stmt) (bool, value.Value, *value.Fault) {
	none := value.Value{}
	switch st := st.(type) {
	case *ir.SetLocal:
		v, f := s.eval(e, st.X)
		if f != nil {
			return false, none, f
		}
		e.fr[st.Var.Slot] = v
	case *ir.SetField:
		nv, f := s.eval(e, st.Node)
		if f != nil {
			return false, none, f
		}
		if nv.NodeID() == 0 {
			return false, none, fault(source.Nobody, st.Pos(), "set %s of sim.Nobody", st.Field.Name).
				WithAlso(st.Node.Pos(), "this is sim.Nobody")
		}
		v, f := s.eval(e, st.X)
		if f != nil {
			return false, none, f
		}
		s.setField(nv.NodeID(), st.Field, v)
	case *ir.SetItem:
		return false, none, s.setItem(e, st)
	case *ir.CallStmt:
		if _, f := s.call(e, st.Call); f != nil {
			return false, none, f
		}
	case *ir.RunBlock:
		cl := e.fr[st.Var.Slot].Closure()
		return false, none, s.runClosure(e, cl, st.Pos())
	case *ir.If:
		c, f := s.eval(e, st.Cond)
		if f != nil {
			return false, none, f
		}
		if c.Bool() {
			return s.execBlock(e, st.Then)
		} else if st.Else != nil {
			return s.execBlock(e, st.Else)
		}
	case *ir.Each:
		over, f := s.eval(e, st.Over)
		if f != nil {
			return false, none, f
		}
		if st.IsMap {
			m := over.Map()
			for i := range m.Keys {
				if st.Key != nil {
					e.fr[st.Key.Slot] = m.Keys[i]
				}
				if st.Val != nil {
					e.fr[st.Val.Slot] = m.Vals[i]
				}
				if ret, v, f := s.execBlock(e, st.Body); f != nil || ret {
					return ret, v, f
				}
			}
		} else {
			for i, it := range over.List().Items {
				if st.Key != nil {
					e.fr[st.Key.Slot] = value.Number(float64(i))
				}
				if st.Val != nil {
					e.fr[st.Val.Slot] = it
				}
				if ret, v, f := s.execBlock(e, st.Body); f != nil || ret {
					return ret, v, f
				}
			}
		}
	case *ir.After:
		d, f := s.eval(e, st.Delay)
		if f != nil {
			return false, none, f
		}
		if d.Int < 0 {
			return false, none, fault(source.BadArgument, st.Pos(), "after needs a delay of 0 or more, not %s", value.FormatTime(d.Int)).
				WithAlso(st.Delay.Pos(), "is "+value.FormatTime(d.Int))
		}
		at := s.now + d.Int
		if at < s.now {
			return false, none, fault(source.NumberRange, st.Pos(), "the result is too large for a number").WithLabel("too large")
		}
		ev := &event{at: at, kind: evTimer, node: e.node, body: st.Body, frame: append([]value.Value(nil), e.fr...), pos: st.Pos()}
		if n := s.getNode(e.node); n != nil {
			ev.inc = n.inc
		}
		s.q.push(ev)
	case *ir.Deliver:
		rv, f := s.eval(e, st.Rec)
		if f != nil {
			return false, none, f
		}
		to, f := s.eval(e, st.To)
		if f != nil {
			return false, none, f
		}
		if to.NodeID() == 0 {
			return false, none, fault(source.Nobody, st.Pos(), "deliver to sim.Nobody").WithAlso(st.To.Pos(), "this is sim.Nobody")
		}
		s.rec(trace.Fact{Ev: "sim.Injected", Node: to.NodeID(), Record: rv.Record()})
		s.q.push(&event{at: s.now, kind: evDeliver, node: to.NodeID(), rec: rv.Record(), pos: st.Pos()})
	case *ir.Emit:
		rv, f := s.eval(e, st.Rec)
		if f != nil {
			return false, none, f
		}
		by, f := s.eval(e, st.By)
		if f != nil {
			return false, none, f
		}
		s.rec(trace.Fact{Ev: "sim.Emitted", Node: by.NodeID(), Record: rv.Record()})
	case *ir.Result:
		v, f := s.eval(e, st.X)
		return true, v, f
	default:
		panic(fmt.Sprintf("sim: statement %T is not available in a simulation", st))
	}
	return false, none, nil
}

// runClosure runs a block with a fresh copy of its captured frame.
func (s *sim) runClosure(e *ex, cl *value.Closure, p ir.Pos) *value.Fault {
	if s.depth >= s.o.MaxDepth {
		return fault(source.RunLimit, p, "more than %d nested calls", s.o.MaxDepth).
			WithLabel(fmt.Sprintf("nested call over %d", s.o.MaxDepth))
	}
	s.depth++
	ne := &ex{fr: append([]value.Value(nil), cl.Frame...), node: e.node, kind: e.kind}
	_, _, f := s.execBlock(ne, cl.Body)
	s.depth--
	return f
}

func (s *sim) setField(id int, f *ir.Field, v value.Value) {
	n := s.getNode(id)
	slot := &n.state[n.index(f.Pkg)][f.Index]
	old := *slot
	*slot = v
	if f.Exported && !value.Equal(old, v) {
		s.rec(trace.Fact{Ev: "sim.Changed", Node: id, Field: f, Old: old, New: v})
	}
}

func (s *sim) setItem(e *ex, st *ir.SetItem) *value.Fault {
	nodeID := 0
	var fp *ir.Field
	switch b := st.Base.(type) {
	case *ir.FieldPlace:
		nv, f := s.eval(e, b.Node)
		if f != nil {
			return f
		}
		if nv.NodeID() == 0 {
			return fault(source.Nobody, st.Pos(), "set %s of sim.Nobody", b.Field.Name).
				WithAlso(b.Node.Pos(), "this is sim.Nobody")
		}
		nodeID, fp = nv.NodeID(), b.Field
	}
	k, f := s.eval(e, st.Key)
	if f != nil {
		return f
	}
	x, f := s.eval(e, st.X)
	if f != nil {
		return f
	}
	var cur value.Value
	if fp != nil {
		n := s.getNode(nodeID)
		cur = n.state[n.index(fp.Pkg)][fp.Index]
	} else {
		cur = e.fr[st.Base.(*ir.VarPlace).Var.Slot]
	}
	var nv value.Value
	if st.IsMap {
		nv = value.MapValue(cur.Map().With(k, x))
	} else {
		items := cur.List().Items
		i, ok := index(k, len(items))
		if !ok {
			return indexFault(fault(source.IndexRange, st.Pos(), "index %s is out of range for a list of %s", value.FormatNumber(k.Num), plural(len(items))),
				st.Key.Pos(), k, len(items))
		}
		cp := append([]value.Value(nil), items...)
		cp[i] = x
		nv = value.ListOf(cp)
	}
	if fp != nil {
		s.setField(nodeID, fp, nv)
	} else {
		e.fr[st.Base.(*ir.VarPlace).Var.Slot] = nv
	}
	return nil
}

// closureFor returns the block argument b as a closure; a block that only
// runs a block parameter is replaced by the closure that parameter holds.
func closureFor(e *ex, b *ir.Block) value.Value {
	if len(b.Stmts) == 1 {
		if rb, ok := b.Stmts[0].(*ir.RunBlock); ok {
			return e.fr[rb.Var.Slot]
		}
	}
	return value.BlockOf(&value.Closure{Body: b, Frame: append([]value.Value(nil), e.fr...)})
}

func (s *sim) call(e *ex, c *ir.Call) (value.Value, *value.Fault) {
	p := c.Proc
	var recv value.Value
	if c.Recv != nil {
		v, f := s.eval(e, c.Recv)
		if f != nil {
			return v, f
		}
		recv = v
	}
	args := make([]value.Value, len(c.Args), len(c.Args)+1)
	for i, a := range c.Args {
		v, f := s.eval(e, a)
		if f != nil {
			return v, f
		}
		args[i] = v
	}
	if p.Kind == ir.ProcNative {
		if c.Recv != nil {
			args = append([]value.Value{recv}, args...)
		}
		fn := s.table[p.Native]
		if fn == nil {
			panic("sim: no native " + p.Native)
		}
		s.curPos = c.Pos()
		v, f := fn(s, c, args)
		if f != nil {
			return v, f
		}
		if s.hostFault != nil {
			return v, s.hostFault
		}
		return v, nil
	}
	blocks := make([]value.Value, len(c.Blocks))
	for i, b := range c.Blocks {
		blocks[i] = closureFor(e, b)
	}
	if s.depth >= s.o.MaxDepth {
		return value.Value{}, fault(source.RunLimit, c.Pos(), "more than %d nested calls", s.o.MaxDepth).
			WithLabel(fmt.Sprintf("nested call over %d", s.o.MaxDepth))
	}
	fr := make([]value.Value, p.Slots)
	if p.Receiver != nil && c.Recv != nil {
		fr[p.Receiver.Slot] = recv
	}
	for i, v := range p.Params {
		fr[v.Slot] = args[i]
	}
	for i, v := range p.BlockParams {
		fr[v.Slot] = blocks[i]
	}
	s.depth++
	_, v, f := s.execBlock(&ex{fr: fr, node: e.node, kind: e.kind}, p.Body)
	s.depth--
	return v, f
}

var _ = natives.Table
