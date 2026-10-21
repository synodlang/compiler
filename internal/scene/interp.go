package scene

import (
	"fmt"

	"github.com/synodlang/compiler/internal/ir"
	"github.com/synodlang/compiler/internal/natives"
	"github.com/synodlang/compiler/internal/source"
	"github.com/synodlang/compiler/internal/value"
)

// The look interpreter mirrors the simulator's (internal/sim): the same
// statements and expressions over the same values, with the look-only
// statements (draw) and expressions (attachments, colors) added and the
// node-only ones (set of a node field, deliver, emit) absent; the checker
// has already ruled those out in look context.

// execBlock runs statements; returned reports a Result.
func (b *builder) execBlock(e *ex, blk *ir.Block) (bool, value.Value, *value.Fault) {
	for _, st := range blk.Stmts {
		ret, v, f := b.exec(e, st)
		if f != nil || ret {
			return ret, v, f
		}
	}
	return false, value.Value{}, nil
}

// maxDelayMs bounds an "after" so that scaling it cannot overflow.
const maxDelayMs = 1_000_000_000_000_000

// floorDiv is a/b rounded toward minus infinity, b > 0.
func floorDiv(a, b int64) int64 {
	q := a / b
	if a%b != 0 && a < 0 {
		q--
	}
	return q
}

func (b *builder) exec(e *ex, st ir.Stmt) (bool, value.Value, *value.Fault) {
	none := value.Value{}
	switch st := st.(type) {
	case *ir.SetLocal:
		v, f := b.eval(e, st.X)
		if f != nil {
			return false, none, f
		}
		e.fr[st.Var.Slot] = v
	case *ir.SetItem:
		vp, ok := st.Base.(*ir.VarPlace)
		if !ok {
			panic("scene: set of a node field in a look")
		}
		k, f := b.eval(e, st.Key)
		if f != nil {
			return false, none, f
		}
		x, f := b.eval(e, st.X)
		if f != nil {
			return false, none, f
		}
		nv, f := value.SetItemOf(e.fr[vp.Var.Slot], k, x, st.IsMap, st.Pos(), st.Key.Pos())
		if f != nil {
			return false, none, f
		}
		e.fr[vp.Var.Slot] = nv
	case *ir.CallStmt:
		if _, f := b.call(e, st.Call); f != nil {
			return false, none, f
		}
	case *ir.RunBlock:
		cl := e.fr[st.Var.Slot].Closure()
		return false, none, b.runClosure(cl, st.Pos())
	case *ir.If:
		c, f := b.eval(e, st.Cond)
		if f != nil {
			return false, none, f
		}
		if c.Bool() {
			return b.execBlock(e, st.Then)
		} else if st.Else != nil {
			return b.execBlock(e, st.Else)
		}
	case *ir.Each:
		over, f := b.eval(e, st.Over)
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
				if ret, v, f := b.execBlock(e, st.Body); f != nil || ret {
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
				if ret, v, f := b.execBlock(e, st.Body); f != nil || ret {
					return ret, v, f
				}
			}
		}
	case *ir.After:
		d, f := b.eval(e, st.Delay)
		if f != nil {
			return false, none, f
		}
		if d.Int < 0 {
			return false, none, fault(source.BadArgument, st.Pos(), "after needs a delay of 0 or more, not %s", value.FormatTime(d.Int)).
				WithAlso(st.Delay.Pos(), "is "+value.FormatTime(d.Int))
		}
		ms := floorDiv(d.Int, 1000)
		if ms > maxDelayMs {
			return false, none, fault(source.NumberRange, st.Pos(), "the result is too large for a number").WithLabel("too large")
		}
		b.timerSeq++
		b.timers.push(&timer{at: b.now + b.clock.scale(ms), seq: b.timerSeq,
			body: st.Body, frame: append([]value.Value(nil), e.fr...), pos: st.Pos(), cur: b.cur})
	case *ir.Draw:
		return false, none, b.draw(e, st)
	case *ir.Result:
		v, f := b.eval(e, st.X)
		return true, v, f
	default:
		panic(fmt.Sprintf("scene: statement %T is not available in a look", st))
	}
	return false, none, nil
}

// draw runs a draw statement: bind the shape, attach it, start its reactive
// block.
func (b *builder) draw(e *ex, st *ir.Draw) *value.Fault {
	v, f := b.eval(e, st.Shape)
	if f != nil {
		return f
	}
	e.fr[st.Var.Slot] = v
	i := int(v.Int)
	if st.For != nil {
		kv, f := b.eval(e, st.For)
		if f != nil {
			return f
		}
		b.attached[i] = true
		if err := b.world.attach(st.Name, attKeyOf(kv), i, b.now); err != nil {
			return b.worldFault(err, st.Pos())
		}
	}
	if st.Reactive != nil {
		return b.newInstance(st, e, i)
	}
	return nil
}

// runClosure runs a block with a fresh copy of its captured frame.
func (b *builder) runClosure(cl *value.Closure, p ir.Pos) *value.Fault {
	if b.depth >= b.o.MaxDepth {
		return fault(source.RunLimit, p, "more than %d nested calls", b.o.MaxDepth).
			WithLabel(fmt.Sprintf("nested call over %d", b.o.MaxDepth))
	}
	b.depth++
	_, _, f := b.execBlock(&ex{fr: append([]value.Value(nil), cl.Frame...)}, cl.Body)
	b.depth--
	return f
}

// closureFor returns the block argument blk as a closure; a block that only
// runs a block parameter is replaced by the closure that parameter holds.
func closureFor(e *ex, blk *ir.Block) value.Value {
	if len(blk.Stmts) == 1 {
		if rb, ok := blk.Stmts[0].(*ir.RunBlock); ok {
			return e.fr[rb.Var.Slot]
		}
	}
	return value.BlockOf(&value.Closure{Body: blk, Frame: append([]value.Value(nil), e.fr...)})
}

func (b *builder) call(e *ex, c *ir.Call) (value.Value, *value.Fault) {
	p := c.Proc
	var recv value.Value
	if c.Recv != nil {
		v, f := b.eval(e, c.Recv)
		if f != nil {
			return v, f
		}
		recv = v
	}
	args := make([]value.Value, len(c.Args), len(c.Args)+1)
	for i, a := range c.Args {
		v, f := b.eval(e, a)
		if f != nil {
			return v, f
		}
		args[i] = v
	}
	if p.Kind == ir.ProcNative {
		if c.Recv != nil {
			args = append([]value.Value{recv}, args...)
		}
		if fn := Table[p.Native]; fn != nil {
			return fn(b, c, args)
		}
		if fn := natives.Table[p.Native]; fn != nil {
			return fn(&b.host, c, args)
		}
		panic("scene: no native " + p.Native)
	}
	blocks := make([]value.Value, len(c.Blocks))
	for i, blk := range c.Blocks {
		blocks[i] = closureFor(e, blk)
	}
	if b.depth >= b.o.MaxDepth {
		return value.Value{}, fault(source.RunLimit, c.Pos(), "more than %d nested calls", b.o.MaxDepth).
			WithLabel(fmt.Sprintf("nested call over %d", b.o.MaxDepth))
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
	b.depth++
	saved := b.cur
	b.cur = p
	_, v, f := b.execBlock(&ex{fr: fr}, p.Body)
	b.cur = saved
	b.depth--
	return v, f
}
