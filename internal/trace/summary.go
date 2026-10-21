package trace

import (
	"io"
	"strconv"
	"strings"

	"github.com/synodlang/compiler/internal/ir"
	"github.com/synodlang/compiler/internal/value"
)

// Summary limits: more than maxFactLines fact lines are cut to the first
// headFactLines and the last tailFactLines.
const (
	maxFactLines  = 400
	headFactLines = 250
	tailFactLines = 100
)

// WriteSummary writes the text summary of a run for agents (--summary):
// the run, the end, the always lines, the failure, the steps that recorded
// facts and the final state of every node. The "always:" section is left out
// when the program has no always line. Facts print as their event phrase
// with unexported record fields shown. Times use value.FormatTime. When more
// than 400 fact lines exist, the first 250 and the last 100 are kept, joined
// by a line "… N steps omitted (full trace: <TracePath>)" where N counts the
// steps whose facts are all omitted; a step cut by the gap prints its header
// again before its remaining facts.
func WriteSummary(w io.Writer, in SummaryInput, o Options) error {
	s := &summarizer{o: o, qual: o.qualOf()}
	s.opts = value.FormatOpts{Qual: s.qual, ShowUnexported: true}
	var b strings.Builder
	b.WriteString("run " + in.Header.Program + " seed " + strconv.FormatUint(in.Header.Seed, 10) + "\n")

	reason := in.EndReason
	if reason == "" {
		reason = "failed"
	}
	b.WriteString("end: " + reason + " at " + value.FormatTime(in.End) + ", " +
		Count(len(in.Final), "node") + ", " + Count(len(in.Steps), "step") + ", " +
		Count(len(in.Facts), "event") + "\n")

	if len(in.Invariants) > 0 {
		b.WriteString("always:\n")
		for _, inv := range in.Invariants {
			rule := o.rule(inv.Rule, inv.Pos)
			if inv.Holds && inv.Breaks == 0 {
				b.WriteString("  holds          " + rule + "\n")
				continue
			}
			n := inv.Breaks
			if n == 0 {
				n = 1
			}
			b.WriteString("  broken " + strconv.Itoa(n) + "x @" + value.FormatTime(inv.FirstBreak) + "  " + rule + "\n")
		}
	}

	if f := in.Failed; f != nil {
		b.WriteString("failed: at " + value.FormatTime(in.End) + ": ")
		if f.HasPos {
			b.WriteString(o.pos(f.Pos) + ": ")
		}
		b.WriteString("error[" + string(f.Code) + "]: " + f.Message + "\n")
	}

	b.WriteString("steps:\n")
	s.steps(&b, in)

	b.WriteString("final:\n")
	for _, n := range in.Final {
		state := "up"
		switch {
		case n.Removed:
			state = "removed"
		case !n.Up:
			state = "down"
		}
		line := "  " + value.NodeName(n.Node) + " " + state
		if st := s.state(n.Runs, n.State); st != "" {
			line += "  " + st
		}
		b.WriteString(line + "\n")
	}
	_, err := io.WriteString(w, b.String())
	return err
}

type summarizer struct {
	o    Options
	qual func(*ir.Package) string
	opts value.FormatOpts
}

// line is one output line; head lines carry the step they head.
type line struct {
	text string
	head bool
	step int // index into groups
}

func (s *summarizer) steps(b *strings.Builder, in SummaryInput) {
	byIndex := map[int]Step{}
	for _, st := range in.Steps {
		byIndex[st.Index] = st
	}
	var lines []line
	groups, facts := 0, 0
	cur := -1
	for i := range in.Facts {
		f := &in.Facts[i]
		if groups == 0 || f.Step != cur {
			cur = f.Step
			lines = append(lines, line{text: s.header(byIndex, f), head: true, step: groups})
			groups++
		}
		lines = append(lines, line{text: "  " + s.phrase(f), step: groups - 1})
		facts++
	}
	if facts <= maxFactLines {
		for _, l := range lines {
			b.WriteString(l.text + "\n")
		}
		return
	}
	// Keep the first headFactLines fact lines and the last tailFactLines.
	headEnd, seen := 0, 0 // lines[:headEnd] is the head
	for i, l := range lines {
		if !l.head {
			seen++
		}
		if seen == headFactLines {
			headEnd = i + 1
			break
		}
	}
	tailStart, seen := len(lines), 0
	for i := len(lines) - 1; i >= 0; i-- {
		if !lines[i].head {
			seen++
		}
		if seen == tailFactLines {
			tailStart = i
			break
		}
	}
	shown := map[int]bool{}
	for _, l := range lines[:headEnd] {
		shown[l.step] = true
	}
	for _, l := range lines[tailStart:] {
		shown[l.step] = true
	}
	omitted := groups - len(shown)
	for _, l := range lines[:headEnd] {
		b.WriteString(l.text + "\n")
	}
	note := "… " + strconv.Itoa(omitted) + " steps omitted"
	if in.TracePath != "" {
		note += " (full trace: " + in.TracePath + ")"
	}
	b.WriteString(note + "\n")
	if !lines[tailStart].head {
		// The cut falls inside a step: repeat its header.
		for i := tailStart - 1; i >= 0; i-- {
			if lines[i].head {
				b.WriteString(lines[i].text + "\n")
				break
			}
		}
	}
	for _, l := range lines[tailStart:] {
		b.WriteString(l.text + "\n")
	}
}

// header is "@<time> <kind> [node]" for the step of fact f.
func (s *summarizer) header(steps map[int]Step, f *Fact) string {
	st, ok := steps[f.Step]
	if !ok {
		return "@" + value.FormatTime(f.T) + " step"
	}
	h := "@" + value.FormatTime(st.T) + " " + st.Kind
	if st.Kind == "timer" && st.Node == 0 {
		h += " main"
	} else if st.Kind != "main" {
		h += " " + value.NodeName(st.Node)
	}
	return h
}

// state is `q.Field value, …` for the exported node fields of runs.
func (s *summarizer) state(runs []*ir.Package, vals []value.Value) string {
	var parts []string
	i := 0
	for _, p := range runs {
		for _, fld := range p.NodeFields {
			if !fld.Exported {
				continue
			}
			if i >= len(vals) {
				break
			}
			parts = append(parts, s.qual(fld.Pkg)+"."+fld.Name+" "+value.Format(vals[i], s.opts))
			i++
		}
	}
	return strings.Join(parts, ", ")
}

func (s *summarizer) rec(r *value.Record) string {
	if r == nil {
		return "<none>"
	}
	return value.Format(value.RecordOf(r.Type, r.Fields), s.opts)
}

func (s *summarizer) nodeList(ids []int) string {
	names := make([]string, len(ids))
	for i, id := range ids {
		names[i] = value.NodeName(id)
	}
	return "{" + strings.Join(names, ", ") + "}"
}

// phrase is a fact as its event phrase.
func (s *summarizer) phrase(f *Fact) string {
	node := value.NodeName(f.Node)
	msg := "#" + strconv.Itoa(f.MsgID) + " " + s.rec(f.Msg) + " from " + value.NodeName(f.From) + " to " + value.NodeName(f.To)
	switch f.Ev {
	case "sim.Ended":
		return "sim.Ended " + f.Reason
	case "sim.Added", "sim.Restarted":
		var names []string
		for _, p := range f.Runs {
			if !p.IsStd {
				names = append(names, s.qual(p))
			}
		}
		if len(names) == 0 {
			for _, p := range f.Runs {
				names = append(names, s.qual(p))
			}
		}
		out := f.Ev + " " + node + " runs " + strings.Join(names, ", ")
		if st := s.state(f.Runs, f.State); st != "" {
			out += ": " + st
		}
		return out
	case "sim.Changed":
		name := "<none>"
		if f.Field != nil {
			name = s.qual(f.Field.Pkg) + "." + f.Field.Name
		}
		return "sim.Changed " + name + " of " + node + " from " + value.Format(f.Old, s.opts) + " to " + value.Format(f.New, s.opts)
	case "sim.Injected":
		return "sim.Injected " + s.rec(f.Record) + " into " + node
	case "sim.Emitted":
		return "sim.Emitted " + s.rec(f.Record) + " by " + node
	case "sim.Broken":
		return "sim.Broken " + s.o.rule(f.Rule, f.RulePos)
	case "sim.Failed":
		if f.Fault == nil {
			return "sim.Failed"
		}
		return "sim.Failed " + string(f.Fault.Code) + ": " + f.Fault.Message
	case "net.Sent":
		return "net.Sent " + msg
	case "net.Arrived":
		return "net.Arrived " + msg
	case "net.Dropped":
		return "net.Dropped " + msg + " (" + f.Reason + ")"
	case "net.Partitioned":
		return "net.Partitioned " + s.nodeList(f.A) + " from " + s.nodeList(f.B)
	case "disk.Written":
		return "disk.Written " + value.Format(value.Text(f.Key), s.opts) + " on " + node
	case "sim.Started", "sim.Removed", "sim.Crashed", "sim.Fired", "net.Healed":
		if f.Node > 0 {
			return f.Ev + " " + node
		}
		return f.Ev
	}
	return f.Ev
}

// FailedLine is the line `synod run` prints for a failed run:
// "failed at 2s: raft/demo.synod:12:5: error[index-range]: message", without
// the position part when the fault has none. end is the time of the failure
// in microseconds.
func FailedLine(f *value.Fault, end int64, o Options) string {
	pos := ""
	if f.HasPos {
		pos = o.pos(f.Pos) + ": "
	}
	return "failed at " + value.FormatTime(end) + ": " + pos + "error[" + string(f.Code) + "]: " + f.Message
}

// Count is n followed by the word, with an "s" unless n is 1: "1 node",
// "2 nodes".
func Count(n int, word string) string {
	if n == 1 {
		return "1 " + word
	}
	return strconv.Itoa(n) + " " + word + "s"
}
