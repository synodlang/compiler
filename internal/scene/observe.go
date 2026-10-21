package scene

import (
	"github.com/synodlang/compiler/internal/ir"
	"github.com/synodlang/compiler/internal/trace"
	"github.com/synodlang/compiler/internal/value"
)

// eventID is the native id of the observer event for fact event ev, or ""
// when no observer can match it.
func eventID(ev string) string {
	switch ev {
	case "sim.Started":
		return "sim.started"
	case "sim.Ended":
		return "sim.ended"
	case "sim.Added":
		return "sim.added"
	case "sim.Removed":
		return "sim.removed"
	case "sim.Crashed":
		return "sim.crashed"
	case "sim.Restarted":
		return "sim.restarted"
	case "sim.Changed":
		return "sim.changed"
	case "sim.Injected":
		return "sim.injected"
	case "sim.Fired":
		return "sim.fired"
	case "sim.Broken":
		return "sim.broken"
	case "sim.Emitted":
		return "sim.emitted"
	case "sim.Failed":
		return "sim.failed"
	case "net.Sent":
		return "net.sent"
	case "net.Arrived":
		return "net.arrived"
	case "net.Dropped":
		return "net.dropped"
	case "net.Partitioned":
		return "net.partitioned"
	case "net.Healed":
		return "net.healed"
	case "disk.Written":
		return "disk.written"
	}
	return ""
}

func nodeList(ids []int) value.Value {
	items := make([]value.Value, len(ids))
	for i, id := range ids {
		items[i] = value.Node(id)
	}
	return value.ListOf(items)
}

// holes are the values of the inline holes of the event phrase for fact f,
// in order. A message carries its id in Value.Int.
func (b *builder) holes(f *trace.Fact) []value.Value {
	node := func(n int) value.Value { return value.Node(max(n, 0)) }
	msg := func() value.Value { return value.Value{K: value.KRecord, Ref: f.Msg, Int: int64(f.MsgID)} }
	rec := func(r *value.Record) value.Value { return value.Value{K: value.KRecord, Ref: r} }
	switch f.Ev {
	case "sim.Added", "sim.Removed", "sim.Crashed", "sim.Restarted", "sim.Fired":
		return []value.Value{node(f.Node)}
	case "sim.Changed":
		return []value.Value{value.FieldOf(f.Field), node(f.Node), f.Old, f.New}
	case "sim.Injected", "sim.Emitted":
		return []value.Value{rec(f.Record), node(f.Node)}
	case "sim.Broken":
		return []value.Value{value.Text(trace.RuleText(*f, b.in.Trace))}
	case "sim.Failed":
		msg := ""
		if f.Fault != nil {
			msg = f.Fault.Message
		}
		return []value.Value{value.Text(msg)}
	case "net.Sent", "net.Arrived":
		return []value.Value{msg(), node(f.From), node(f.To)}
	case "net.Dropped":
		return []value.Value{msg(), node(f.From), node(f.To), value.Text(f.Reason)}
	case "net.Partitioned":
		return []value.Value{nodeList(f.A), nodeList(f.B)}
	case "disk.Written":
		return []value.Value{value.Text(f.Key), node(f.Node)}
	}
	return nil
}

// observe runs the observers that match fact f, in order. The store already
// holds the state after f.
func (b *builder) observe(f *trace.Fact) *value.Fault {
	id := eventID(f.Ev)
	if id == "" {
		return nil
	}
	var vals []value.Value
	haveVals := false
	for _, h := range b.obs {
		if h.Event != id {
			continue
		}
		if !haveVals {
			vals, haveVals = b.holes(f), true
		}
		e := &ex{fr: make([]value.Value, h.Slots)}
		ok, flt := b.match(e, h, vals)
		if flt != nil {
			return flt
		}
		if !ok {
			continue
		}
		b.act, b.cur, b.inst, b.track, b.depth = b.begin(h.Pos), nil, nil, nil, 0
		if _, _, flt := b.execBlock(e, h.Body); flt != nil {
			return flt
		}
	}
	return nil
}

// match binds the holes of an event to the variables of handler h; false
// means the handler does not apply to this occurrence.
func (b *builder) match(e *ex, h *ir.Handler, vals []value.Value) (bool, *value.Fault) {
	for _, m := range h.Match {
		v := vals[m.Hole]
		switch m.Kind {
		case ir.MatchBind:
			e.fr[m.Var.Slot] = v
		case ir.MatchRecord:
			if v.K != value.KRecord || v.Record().Type != m.Record {
				return false, nil
			}
			e.fr[m.Var.Slot] = v
		case ir.MatchNodeType:
			if !b.runsPackage(v.NodeID(), m.NodePkg) {
				return false, nil
			}
			e.fr[m.Var.Slot] = v
		case ir.MatchField:
			if v.K != value.KField || v.Ref.(*ir.Field) != m.Field {
				return false, nil
			}
		case ir.MatchEqual:
			b.act, b.cur = &activation{prefix: "match."}, nil
			x, flt := b.eval(e, m.Value)
			if flt != nil {
				return false, flt
			}
			if !value.Equal(x, v) {
				return false, nil
			}
		}
	}
	return true, nil
}

// runsPackage reports whether node n runs package p.
func (b *builder) runsPackage(n int, p *ir.Package) bool {
	for _, q := range b.store.runs(n) {
		if q == p {
			return true
		}
	}
	return false
}
