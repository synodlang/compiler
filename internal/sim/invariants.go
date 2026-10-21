package sim

import (
	"github.com/synodlang/compiler/internal/ir"
	"github.com/synodlang/compiler/internal/trace"
	"github.com/synodlang/compiler/internal/value"
)

// invState tracks one always line.
type invState struct {
	inv        *ir.Invariant
	holds      bool
	breaks     int
	firstBreak int64
}

func (s *sim) initInvariants() {
	for _, inv := range s.prog.Invariants {
		s.invs = append(s.invs, &invState{inv: inv, holds: true})
	}
}

// checkInvariants evaluates every always line after a step.
func (s *sim) checkInvariants() *value.Fault {
	for _, st := range s.invs {
		e := &ex{fr: make([]value.Value, st.inv.Slots), kind: kindCheck}
		v, f := s.eval(e, st.inv.Cond)
		if f != nil {
			return f
		}
		if s.hostFault != nil {
			return s.hostFault
		}
		now := v.Bool()
		if st.holds && !now {
			if st.breaks == 0 {
				st.firstBreak = s.now
			}
			st.breaks++
			s.rec(trace.Fact{Ev: "sim.Broken", Node: -1, Rule: st.inv.Rule, RulePos: st.inv.Pos})
		}
		st.holds = now
	}
	return nil
}
