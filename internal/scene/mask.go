package scene

import (
	"github.com/synodlang/compiler/internal/trace"
	"github.com/synodlang/compiler/internal/value"
)

// maskText is what the trace shows for an unexported record field, and so
// what trace.Decode gives back for it.
const maskText = "…"

// mask returns v with the value of every unexported record field replaced by
// the text "…", at any depth. It is the value a look sees: the same whether
// the facts come straight from the simulator or from trace.Decode, so that
// both give the same scene. A map is rebuilt key by key the way Decode builds
// it.
func mask(v value.Value) value.Value {
	switch v.K {
	case value.KList:
		in := v.List().Items
		out := make([]value.Value, len(in))
		for i, x := range in {
			out[i] = mask(x)
		}
		return value.ListOf(out)
	case value.KMap:
		m := v.Map()
		out := &value.Map{}
		for i := range m.Keys {
			out = out.With(mask(m.Keys[i]), mask(m.Vals[i]))
		}
		return value.MapValue(out)
	case value.KRecord:
		return value.Value{K: value.KRecord, Ref: maskRecord(v.Record()), Int: v.Int}
	}
	return v
}

func maskRecord(r *value.Record) *value.Record {
	if r == nil {
		return nil
	}
	fs := make([]value.Value, len(r.Fields))
	for i, x := range r.Fields {
		if i < len(r.Type.Fields) && !r.Type.Fields[i].Exported {
			fs[i] = value.Text(maskText)
		} else {
			fs[i] = mask(x)
		}
	}
	return &value.Record{Type: r.Type, Fields: fs}
}

// maskFacts returns a copy of facts in which every value is masked.
func maskFacts(facts []trace.Fact) []trace.Fact {
	out := make([]trace.Fact, len(facts))
	for i := range facts {
		f := facts[i]
		f.Msg = maskRecord(f.Msg)
		f.Record = maskRecord(f.Record)
		if f.Field != nil {
			f.Old, f.New = mask(f.Old), mask(f.New)
		}
		if f.State != nil {
			st := make([]value.Value, len(f.State))
			for j, x := range f.State {
				st[j] = mask(x)
			}
			f.State = st
		}
		out[i] = f
	}
	return out
}
