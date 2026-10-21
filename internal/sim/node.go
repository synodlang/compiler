package sim

import (
	"github.com/synodlang/compiler/internal/ir"
	"github.com/synodlang/compiler/internal/types"
	"github.com/synodlang/compiler/internal/value"
)

// node is one simulated node.
type node struct {
	id      int
	pkg     *ir.Package
	runs    []*ir.Package
	state   [][]value.Value // per package of runs, per node field
	up      bool
	removed bool
	inc     int
	disk    map[string]diskEntry
}

type diskEntry struct {
	v value.Value
	t types.Type
}

// index returns the position of p in the node's runs.
func (n *node) index(p *ir.Package) int {
	for i, q := range n.runs {
		if q == p {
			return i
		}
	}
	return -1
}

// initState evaluates the initial values of every node field of runs.
func (s *sim) initState(runs []*ir.Package) [][]value.Value {
	st := make([][]value.Value, len(runs))
	for i, p := range runs {
		st[i] = make([]value.Value, len(p.NodeFields))
		for j, f := range p.NodeFields {
			v, flt := s.initVal(f)
			if flt != nil && s.hostFault == nil {
				s.hostFault = flt
			}
			st[i][j] = v
		}
	}
	return st
}

// initVal returns the initial value of a field, evaluated once.
func (s *sim) initVal(f *ir.Field) (value.Value, *value.Fault) {
	if v, ok := s.inits[f]; ok {
		return v, nil
	}
	if f.Init == nil {
		return value.Value{}, nil
	}
	v, flt := s.eval(&ex{kind: kindCheck}, f.Init)
	if flt != nil {
		return value.Value{}, flt
	}
	s.inits[f] = v
	return v, nil
}

// exported lists the exported node fields' values, in runs order and then
// declaration order.
func exported(runs []*ir.Package, state [][]value.Value) []value.Value {
	var out []value.Value
	for i, p := range runs {
		for j, f := range p.NodeFields {
			if f.Exported {
				out = append(out, state[i][j])
			}
		}
	}
	return out
}

func (s *sim) getNode(id int) *node {
	if id < 1 || id > len(s.nodes) {
		return nil
	}
	return s.nodes[id-1]
}

func (s *sim) queueStart(n *node) {
	s.q.push(&event{at: s.now, kind: evStart, node: n.id, inc: n.inc})
}
