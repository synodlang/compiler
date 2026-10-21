// Package sim is the deterministic simulator: it runs an ir.Program with an
// event queue, simulated nodes and network, and an interpreter, and records
// the facts of the run (spec/07, spec/08).
//
// Run never recovers panics: a panic is a compiler bug.
package sim

import (
	"fmt"
	"sort"
	"strings"

	"github.com/synodlang/compiler/internal/ir"
	"github.com/synodlang/compiler/internal/natives"
	"github.com/synodlang/compiler/internal/rng"
	"github.com/synodlang/compiler/internal/source"
	"github.com/synodlang/compiler/internal/trace"
	"github.com/synodlang/compiler/internal/value"
)

// Options configure a run; zero fields take the defaults shown.
type Options struct {
	// Seed overrides sim.Seed when not nil.
	Seed *uint64
	// TimeLimit is the simulated time limit in microseconds (600_000_000).
	TimeLimit int64
	// MaxStepsPerInstant bounds the steps at one instant (100_000).
	MaxStepsPerInstant int
	// MaxDepth bounds the call depth (10_000).
	MaxDepth int
	// MaxItems bounds list and text sizes (natives.MaxItems).
	MaxItems int
	// MaxSteps bounds the steps of a run; 0 means unlimited.
	MaxSteps int
}

// Result is the outcome of a run.
type Result struct {
	Header     trace.Header
	Facts      []trace.Fact
	Steps      []trace.Step
	Final      []trace.NodeState
	Invariants []trace.InvariantResult
	// EndReason is "stop" or "empty"; empty after a failure.
	EndReason string
	End       int64
	Failed    *value.Fault
}

// nativeTable is the table of natives; tests may replace it.
var nativeTable = natives.Table

type sim struct {
	o      Options
	prog   *ir.Program
	table  map[string]natives.Func
	byPath map[string]*ir.Package

	now       int64
	q         queue
	rng       *rng.PCG32
	seed      uint64
	hdrSeed   uint64
	stopAt    int64
	hasStop   bool
	nodes     []*node
	net       network
	msgID     int
	inits     map[*ir.Field]value.Value
	invs      []*invState
	facts     []trace.Fact
	steps     []trace.Step
	step      int
	curPos    ir.Pos
	depth     int
	failed    *value.Fault
	ended     string
	hostFault *value.Fault

	instantAt int64
	instantN  int
	total     int
}

// Run executes p.
func Run(p *ir.Program, o Options) *Result {
	if o.TimeLimit == 0 {
		o.TimeLimit = 600_000_000
	}
	if o.MaxStepsPerInstant == 0 {
		o.MaxStepsPerInstant = 100_000
	}
	if o.MaxDepth == 0 {
		o.MaxDepth = 10_000
	}
	if o.MaxItems == 0 {
		o.MaxItems = natives.MaxItems
	}
	s := &sim{o: o, prog: p, table: nativeTable, net: newNetwork(),
		inits: map[*ir.Field]value.Value{}, byPath: map[string]*ir.Package{}}
	for _, pk := range p.Packages {
		s.byPath[pk.Path] = pk
	}
	s.seed = 1
	if o.Seed != nil {
		s.seed = *o.Seed
	}
	s.rng = rng.New(s.seed)
	s.initInvariants()
	s.run()
	return s.result()
}

func (s *sim) rec(f trace.Fact) {
	f.Step, f.T = s.step, s.now
	s.facts = append(s.facts, f)
}

func (s *sim) beginStep(kind string, node int, pos ir.Pos) {
	s.step = len(s.steps)
	s.steps = append(s.steps, trace.Step{Index: s.step, T: s.now, Kind: kind, Node: node, Pos: pos})
}

func (s *sim) fail(f *value.Fault) {
	s.failed = f
	s.rec(trace.Fact{Ev: "sim.Failed", Node: -1, Fault: f})
}

func (s *sim) end(reason string) {
	s.ended = reason
	s.rec(trace.Fact{Ev: "sim.Ended", Node: -1, Reason: reason})
}

func (s *sim) run() {
	s.rec(trace.Fact{Ev: "sim.Started", Node: -1})
	s.beginStep("main", 0, ir.Pos{})
	s.instantAt, s.instantN, s.total = 0, 1, 1
	if s.prog.Main != nil {
		e := &ex{fr: make([]value.Value, s.prog.Main.Slots), kind: kindMain}
		_, _, f := s.execBlock(e, s.prog.Main.Body)
		if f == nil {
			f = s.hostFault
		}
		if f != nil {
			s.hdrSeed = s.seed
			s.fail(f)
			return
		}
	}
	s.hdrSeed = s.seed
	if f := s.checkInvariants(); f != nil {
		s.fail(f)
		return
	}
	for {
		ev, ok := s.q.peek()
		if !ok {
			s.end("empty")
			return
		}
		if s.discardable(ev) {
			s.q.pop()
			continue
		}
		if s.hasStop && ev.at > s.stopAt {
			if s.now < s.stopAt {
				s.now = s.stopAt
			}
			s.end("stop")
			return
		}
		if ev.at > s.o.TimeLimit {
			s.now = s.o.TimeLimit
			msg := "the run reached 10 minutes of simulated time"
			if s.o.TimeLimit != 600_000_000 {
				msg = "the run reached the time limit of " + value.FormatTime(s.o.TimeLimit)
			}
			limit := "10 minutes"
			if s.o.TimeLimit != 600_000_000 {
				limit = value.FormatTime(s.o.TimeLimit)
			}
			s.fail((&value.Fault{Code: source.TimeLimit, Message: msg}).
				WithNote("runs stop after " + limit + " of simulated time"))
			return
		}
		s.q.pop()
		s.now = ev.at
		if ev.at != s.instantAt {
			s.instantAt, s.instantN = ev.at, 0
		}
		s.instantN++
		s.total++
		if s.instantN > s.o.MaxStepsPerInstant {
			s.fail(s.limit(ev, "more than %d steps at "+value.FormatTime(ev.at), s.o.MaxStepsPerInstant).
				WithNote(sprintf("at most %d steps run at one instant of simulated time", s.o.MaxStepsPerInstant)))
			return
		}
		if s.o.MaxSteps > 0 && s.total > s.o.MaxSteps {
			s.fail(s.limit(ev, "more than %d steps", s.o.MaxSteps).
				WithNote(sprintf("at most %d steps run in one run", s.o.MaxSteps)))
			return
		}
		s.hostFault = nil
		f := s.exec1(ev)
		if f == nil {
			f = s.hostFault
		}
		if f == nil {
			f = s.checkInvariants()
		}
		if f != nil {
			s.fail(f)
			return
		}
	}
}

func (s *sim) limit(ev *event, format string, a ...any) *value.Fault {
	if ev.pos == (ir.Pos{}) {
		f := &value.Fault{Code: source.RunLimit}
		f.Message = sprintf(format, a...)
		return f
	}
	return fault(source.RunLimit, ev.pos, format, a...)
}

// discardable reports whether the event is dropped silently on pop.
func (s *sim) discardable(ev *event) bool {
	switch ev.kind {
	case evStart:
		n := s.getNode(ev.node)
		return !n.up || n.inc != ev.inc
	case evTimer:
		if ev.node == 0 {
			return false
		}
		n := s.getNode(ev.node)
		return !n.up || n.inc != ev.inc
	case evDeliver:
		n := s.getNode(ev.node)
		return !n.up
	}
	return false
}

// exec1 runs one popped event as a step.
func (s *sim) exec1(ev *event) *value.Fault {
	n := s.getNode(ev.node)
	switch ev.kind {
	case evStart:
		s.beginStep("start", ev.node, ev.pos)
		for _, p := range n.runs {
			if p.Start == nil {
				continue
			}
			fr := make([]value.Value, p.Start.Slots)
			if r := p.Start.Receiver; r != nil {
				fr[r.Slot] = value.Node(n.id)
			}
			if _, _, f := s.execBlock(&ex{fr: fr, node: n.id, kind: kindNode}, p.Start.Body); f != nil {
				return f
			}
		}
	case evDeliver:
		s.beginStep("deliver", ev.node, ev.pos)
		return s.handlers(n, "sim.delivered", []value.Value{{K: value.KRecord, Ref: ev.rec}})
	case evReceive:
		s.beginStep("receive", ev.node, ev.pos)
		if !n.up {
			s.rec(trace.Fact{Ev: "net.Dropped", Node: ev.node, Msg: ev.rec, MsgID: ev.id, From: ev.from, To: ev.node, Reason: "crashed"})
			return nil
		}
		s.rec(trace.Fact{Ev: "net.Arrived", Node: ev.node, Msg: ev.rec, MsgID: ev.id, From: ev.from, To: ev.node})
		return s.handlers(n, "net.received", []value.Value{{K: value.KRecord, Ref: ev.rec}, value.Node(ev.from)})
	case evDrop:
		s.beginStep("drop", ev.node, ev.pos)
		s.rec(trace.Fact{Ev: "net.Dropped", Node: ev.node, Msg: ev.rec, MsgID: ev.id, From: ev.from, To: ev.node, Reason: ev.reason})
	case evTimer:
		s.beginStep("timer", ev.node, ev.pos)
		kind := kindMain
		if ev.node != 0 {
			kind = kindNode
			s.rec(trace.Fact{Ev: "sim.Fired", Node: ev.node})
		}
		_, _, f := s.execBlock(&ex{fr: ev.frame, node: ev.node, kind: kind}, ev.body)
		return f
	}
	return nil
}

// handlers runs every matching node handler of the event on n.
func (s *sim) handlers(n *node, event string, vals []value.Value) *value.Fault {
	for _, p := range n.runs {
		for _, h := range p.NodeHandlers {
			if h.Event != event {
				continue
			}
			e := &ex{fr: make([]value.Value, h.Slots), node: n.id, kind: kindNode}
			if h.Receiver != nil {
				e.fr[h.Receiver.Slot] = value.Node(n.id)
			}
			ok := true
			for _, m := range h.Match {
				v := vals[m.Hole]
				switch m.Kind {
				case ir.MatchBind:
					e.fr[m.Var.Slot] = v
				case ir.MatchRecord:
					if v.K != value.KRecord || v.Record().Type != m.Record {
						ok = false
					} else {
						e.fr[m.Var.Slot] = v
					}
				case ir.MatchNodeType:
					t := s.getNode(v.NodeID())
					if t == nil || t.index(m.NodePkg) < 0 {
						ok = false
					} else {
						e.fr[m.Var.Slot] = v
					}
				case ir.MatchEqual:
					x, f := s.eval(e, m.Value)
					if f != nil {
						return f
					}
					ok = value.Equal(x, v)
				}
				if !ok {
					break
				}
			}
			if !ok {
				continue
			}
			if _, _, f := s.execBlock(e, h.Body); f != nil {
				return f
			}
			if s.hostFault != nil {
				return s.hostFault
			}
		}
	}
	return nil
}

func (s *sim) result() *Result {
	r := &Result{Facts: s.facts, Steps: s.steps, EndReason: s.ended, End: s.now, Failed: s.failed}
	r.Header.Seed = s.hdrSeed
	r.Header.Program = strings.TrimSuffix(s.prog.Name, "#main")
	for _, p := range s.prog.Packages {
		if !p.IsMain {
			r.Header.Packages = append(r.Header.Packages, p.Path)
		}
	}
	sort.Strings(r.Header.Packages)
	for _, n := range s.nodes {
		r.Final = append(r.Final, trace.NodeState{Node: n.id, Up: n.up, Removed: n.removed, Runs: n.runs, State: exported(n.runs, n.state)})
	}
	for _, st := range s.invs {
		r.Invariants = append(r.Invariants, trace.InvariantResult{Rule: st.inv.Rule, Pos: st.inv.Pos, Holds: st.holds, Breaks: st.breaks, FirstBreak: st.firstBreak})
	}
	return r
}

func sprintf(f string, a ...any) string { return fmt.Sprintf(f, a...) }
