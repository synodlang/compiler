package trace

import (
	"io"

	"github.com/synodlang/compiler/internal/ir"
	"github.com/synodlang/compiler/internal/load"
)

// encoder holds the output buffer of Encode.
type encoder struct {
	w    jw
	o    Options
	qual func(*ir.Package) string
}

// Encode writes trace.jsonl: the header line and one line per fact, each
// ending in LF, with the keys in the order of the format spec. Fact i has
// seq i+1. It fails on a fact with an unknown event or a missing record,
// and on a non-finite number.
func Encode(w io.Writer, h Header, facts []Fact, o Options) error {
	e := &encoder{o: o, qual: o.qualOf()}
	e.header(h)
	for i := range facts {
		if err := e.fact(i+1, &facts[i]); err != nil {
			return err
		}
	}
	_, err := w.Write(e.w.b)
	return err
}

func (e *encoder) header(h Header) {
	w := &e.w
	w.raw(`{"format":"synod-trace/1","synod":`)
	w.str(load.LanguageVersion)
	w.raw(`,"seed":`)
	w.uint(h.Seed)
	w.raw(`,"program":`)
	w.str(h.Program)
	w.raw(`,"packages":`)
	w.strs(h.Packages)
	w.raw("}\n")
}

// extra keys of each event, in the order of the format spec.
const (
	kNode = 1 << iota
	kMsg
	kFromTo
	kReason
	kRecord
)

var layouts = map[string]int{
	"sim.Started":     0,
	"sim.Ended":       0,
	"sim.Added":       kNode,
	"sim.Restarted":   kNode,
	"sim.Removed":     kNode,
	"sim.Crashed":     kNode,
	"sim.Fired":       kNode,
	"sim.Changed":     kNode,
	"sim.Injected":    kNode | kRecord,
	"sim.Emitted":     kNode | kRecord,
	"sim.Broken":      0,
	"sim.Failed":      0,
	"net.Sent":        kNode | kMsg | kFromTo,
	"net.Arrived":     kNode | kMsg | kFromTo,
	"net.Dropped":     kNode | kMsg | kFromTo | kReason,
	"net.Partitioned": 0,
	"net.Healed":      0,
	"disk.Written":    kNode,
}

func (e *encoder) fieldName(f *ir.Field) string { return e.qual(f.Pkg) + "." + f.Name }

func (e *encoder) fact(seq int, f *Fact) error {
	lay, ok := layouts[f.Ev]
	if !ok {
		return encodeError("unknown event " + f.Ev)
	}
	w := &e.w
	w.raw(`{"seq":`)
	w.int(int64(seq))
	w.raw(`,"t":`)
	w.int(f.T)
	w.raw(`,"ev":`)
	w.str(f.Ev)
	if lay&kNode != 0 && f.Node >= 0 {
		w.key("node", false)
		w.nodeKey(f.Node)
	}
	if lay&kMsg != 0 {
		if f.Msg == nil {
			return encodeError(f.Ev + " without msg")
		}
		w.raw(`,"msg":{"id":`)
		w.int(int64(f.MsgID))
		w.raw(`,"type":`)
		w.str(e.qual(f.Msg.Type.Pkg) + "." + f.Msg.Type.Name)
		w.raw(`,"fields":`)
		if err := e.fields(f.Msg); err != nil {
			return err
		}
		w.byte('}')
	}
	if lay&kFromTo != 0 {
		w.key("from", false)
		w.nodeKey(f.From)
		w.key("to", false)
		w.nodeKey(f.To)
	}
	if lay&kReason != 0 {
		w.key("reason", false)
		w.str(f.Reason)
	}
	if lay&kRecord != 0 {
		if f.Record == nil {
			return encodeError(f.Ev + " without record")
		}
		w.key("record", false)
		if err := e.record(f.Record); err != nil {
			return err
		}
	}
	var err error
	switch f.Ev {
	case "sim.Ended":
		w.key("reason", false)
		w.str(f.Reason)
	case "sim.Added", "sim.Restarted":
		err = e.runsState(f)
	case "sim.Changed":
		if f.Field == nil {
			return encodeError("sim.Changed without field")
		}
		w.key("field", false)
		w.str(e.fieldName(f.Field))
		w.key("old", false)
		if err = e.value(f.Old); err == nil {
			w.key("new", false)
			err = e.value(f.New)
		}
	case "sim.Broken":
		w.key("rule", false)
		w.str(e.o.rule(f.Rule, f.RulePos))
	case "sim.Failed":
		if f.Fault == nil {
			return encodeError("sim.Failed without fault")
		}
		w.key("code", false)
		w.str(string(f.Fault.Code))
		w.key("message", false)
		w.str(f.Fault.Message)
		w.key("pos", false)
		if f.Fault.HasPos {
			w.str(e.o.pos(f.Fault.Pos))
		} else {
			w.raw("null")
		}
	case "net.Partitioned":
		w.key("a", false)
		w.nodes(f.A)
		w.key("b", false)
		w.nodes(f.B)
	case "disk.Written":
		w.key("key", false)
		w.str(f.Key)
	}
	if err != nil {
		return err
	}
	w.raw("}\n")
	return nil
}

// runsState writes "runs" and "state" of sim.Added and sim.Restarted.
func (e *encoder) runsState(f *Fact) error {
	w := &e.w
	paths := make([]string, len(f.Runs))
	for i, p := range f.Runs {
		paths[i] = p.Path
	}
	w.key("runs", false)
	w.strs(paths)
	w.key("state", false)
	w.byte('{')
	i, n := 0, 0
	for _, p := range f.Runs {
		for _, fld := range p.NodeFields {
			if !fld.Exported {
				continue
			}
			if i >= len(f.State) {
				return encodeError("state has fewer values than exported fields")
			}
			w.key(e.fieldName(fld), n == 0)
			n++
			if err := e.value(f.State[i]); err != nil {
				return err
			}
			i++
		}
	}
	w.byte('}')
	return nil
}
