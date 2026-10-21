package scene

import (
	"github.com/synodlang/compiler/internal/ir"
	"github.com/synodlang/compiler/internal/trace"
	"github.com/synodlang/compiler/internal/value"
)

// Special slots of a readKey besides exported-field slots (which are >= 0).
const (
	slotUp         int32 = -1 // whether the node is up
	slotMembership int32 = -2 // the set of nodes (Node is 0)
)

// readKey is one thing a look can read from the store and that a fact can
// change: an exported node field (Slot from store.slotOf), a node's up flag,
// or node membership. Reactive blocks record the keys they read and re-run
// when store.changes(f) meets them.
type readKey struct {
	Node int32
	Slot int32
}

type nodeRec struct {
	exists  bool
	up      bool
	removed bool
	runs    []*ir.Package
	// fields are the exported node fields of runs, in Runs order and then
	// declaration order; vals[i] is the value of fields[i].
	fields []*ir.Field
	vals   []value.Value
}

// stateRow is one exported field of a node and its value.
type stateRow struct {
	Field *ir.Field
	Value value.Value
}

// store replays the node state of a trace as the facts are presented
// (plan 2.3). Observers of a fact see the store after apply(f).
type store struct {
	recs  []nodeRec // indexed by node id (index 0 unused)
	order []int     // node ids in creation order
	slots map[*ir.Field]int32
	// memberVer increases on every Added and Removed.
	memberVer int
}

func newStore() *store { return &store{slots: map[*ir.Field]int32{}} }

func (s *store) rec(n int) *nodeRec {
	if n <= 0 {
		return nil
	}
	if n >= len(s.recs) {
		return nil
	}
	return &s.recs[n]
}

// slotOf is the readKey slot of an exported node field; slots are handed out
// in order of first use, so they are deterministic.
func (s *store) slotOf(f *ir.Field) int32 {
	if v, ok := s.slots[f]; ok {
		return v
	}
	v := int32(len(s.slots))
	s.slots[f] = v
	return v
}

func exportedFields(runs []*ir.Package) []*ir.Field {
	var out []*ir.Field
	for _, p := range runs {
		for _, f := range p.NodeFields {
			if f.Exported {
				out = append(out, f)
			}
		}
	}
	return out
}

// changes returns what fact f changes, computed BEFORE apply(f): {node, field
// slot} for sim.Changed; for sim.Restarted every exported field whose value
// differs from the snapshot plus {node, up}; {node, up} for sim.Crashed and
// sim.Removed; {0, membership} for sim.Added and sim.Removed. Other facts
// change nothing.
func (s *store) changes(f trace.Fact) []readKey {
	switch f.Ev {
	case "sim.Changed":
		if f.Field == nil {
			return nil
		}
		return []readKey{{int32(f.Node), s.slotOf(f.Field)}}
	case "sim.Restarted":
		var out []readKey
		r := s.rec(f.Node)
		fields := exportedFields(f.Runs)
		for i, fl := range fields {
			if i >= len(f.State) {
				break
			}
			if r != nil && r.exists {
				if old, ok := r.get(fl); ok && value.Equal(old, f.State[i]) {
					continue
				}
			}
			out = append(out, readKey{int32(f.Node), s.slotOf(fl)})
		}
		return append(out, readKey{int32(f.Node), slotUp})
	case "sim.Crashed":
		return []readKey{{int32(f.Node), slotUp}}
	case "sim.Removed":
		return []readKey{{int32(f.Node), slotUp}, {0, slotMembership}}
	case "sim.Added":
		return []readKey{{0, slotMembership}}
	}
	return nil
}

func (r *nodeRec) get(f *ir.Field) (value.Value, bool) {
	for i, g := range r.fields {
		if g == f {
			return r.vals[i], true
		}
	}
	return value.Value{}, false
}

// apply updates the store with fact f.
func (s *store) apply(f trace.Fact) {
	switch f.Ev {
	case "sim.Added":
		n := f.Node
		if n <= 0 {
			return
		}
		for len(s.recs) <= n {
			s.recs = append(s.recs, nodeRec{})
		}
		r := &s.recs[n]
		if !r.exists {
			s.order = append(s.order, n)
		}
		r.exists, r.up, r.removed = true, true, false
		s.load(r, f)
		s.memberVer++
	case "sim.Restarted":
		if r := s.rec(f.Node); r != nil && r.exists {
			r.up = true
			s.load(r, f)
		}
	case "sim.Changed":
		if r := s.rec(f.Node); r != nil && r.exists && f.Field != nil {
			for i, g := range r.fields {
				if g == f.Field {
					r.vals[i] = f.New
					break
				}
			}
		}
	case "sim.Crashed":
		if r := s.rec(f.Node); r != nil && r.exists {
			r.up = false
		}
	case "sim.Removed":
		if r := s.rec(f.Node); r != nil && r.exists {
			r.removed, r.up = true, false
			s.memberVer++
		}
	}
}

func (s *store) load(r *nodeRec, f trace.Fact) {
	r.runs = f.Runs
	r.fields = exportedFields(f.Runs)
	r.vals = make([]value.Value, len(r.fields))
	copy(r.vals, f.State)
}

// nodes lists the nodes that are not removed, in creation (id) order.
func (s *store) nodes() []int {
	out := make([]int, 0, len(s.order))
	for _, n := range s.order {
		if !s.recs[n].removed {
			out = append(out, n)
		}
	}
	return out
}

// nodesRunning lists the nodes that are not removed and run package p.
func (s *store) nodesRunning(p *ir.Package) []int {
	var out []int
	for _, n := range s.order {
		r := &s.recs[n]
		if r.removed {
			continue
		}
		for _, q := range r.runs {
			if q == p {
				out = append(out, n)
				break
			}
		}
	}
	return out
}

// isUp reports whether n is up and not removed.
func (s *store) isUp(n int) bool {
	r := s.rec(n)
	return r != nil && r.exists && r.up && !r.removed
}

// exists reports whether n was ever added; removed whether it was removed.
func (s *store) exists(n int) bool  { r := s.rec(n); return r != nil && r.exists }
func (s *store) removed(n int) bool { r := s.rec(n); return r != nil && r.removed }

// runs are the packages node n runs (nil when unknown).
func (s *store) runs(n int) []*ir.Package {
	if r := s.rec(n); r != nil {
		return r.runs
	}
	return nil
}

// field is the current value of exported node field f of node n.
func (s *store) field(n int, f *ir.Field) (value.Value, bool) {
	r := s.rec(n)
	if r == nil || !r.exists {
		return value.Value{}, false
	}
	return r.get(f)
}

// rows lists every exported field of n with its value, in trace state order
// (for sim.StateRows, which reads every exported field of n).
func (s *store) rows(n int) []stateRow {
	r := s.rec(n)
	if r == nil || !r.exists {
		return nil
	}
	out := make([]stateRow, len(r.fields))
	for i, f := range r.fields {
		out[i] = stateRow{f, r.vals[i]}
	}
	return out
}
