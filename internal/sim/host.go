package sim

import (
	"github.com/synodlang/compiler/internal/ir"
	"github.com/synodlang/compiler/internal/natives"
	"github.com/synodlang/compiler/internal/rng"
	"github.com/synodlang/compiler/internal/trace"
	"github.com/synodlang/compiler/internal/types"
	"github.com/synodlang/compiler/internal/value"
)

var _ natives.Host = (*sim)(nil)

func (s *sim) Now() int64      { return s.now }
func (s *sim) RNG() *rng.PCG32 { return s.rng }

func (s *sim) Seed(seed uint64) {
	if s.o.Seed != nil {
		return
	}
	s.seed = seed
	s.rng = rng.New(seed)
}

func (s *sim) StopAt(t int64) { s.stopAt, s.hasStop = t, true }

func (s *sim) Spawn(p *ir.Package) int {
	n := &node{id: len(s.nodes) + 1, pkg: p, runs: p.Runs, up: true, disk: map[string]diskEntry{}}
	n.state = s.initState(n.runs)
	s.nodes = append(s.nodes, n)
	s.rec(trace.Fact{Ev: "sim.Added", Node: n.id, Runs: n.runs, State: exported(n.runs, n.state)})
	s.queueStart(n)
	return n.id
}

func (s *sim) Crash(id int) {
	n := s.getNode(id)
	if n == nil || n.removed || !n.up {
		return
	}
	n.up = false
	n.inc++
	s.rec(trace.Fact{Ev: "sim.Crashed", Node: id})
}

func (s *sim) Restart(id int) {
	n := s.getNode(id)
	if n == nil || n.removed {
		return
	}
	n.inc++
	n.up = true
	n.state = s.initState(n.runs)
	s.rec(trace.Fact{Ev: "sim.Restarted", Node: id, Runs: n.runs, State: exported(n.runs, n.state)})
	s.queueStart(n)
}

func (s *sim) Remove(id int) {
	n := s.getNode(id)
	if n == nil || n.removed {
		return
	}
	n.removed = true
	n.up = false
	n.inc++
	s.rec(trace.Fact{Ev: "sim.Removed", Node: id})
}

func (s *sim) Nodes() []int {
	var out []int
	for _, n := range s.nodes {
		if !n.removed {
			out = append(out, n.id)
		}
	}
	return out
}

func (s *sim) NodesRunning(p *ir.Package) []int {
	var out []int
	for _, n := range s.nodes {
		if !n.removed && n.index(p) >= 0 {
			out = append(out, n.id)
		}
	}
	return out
}

func (s *sim) IsUp(id int) bool {
	n := s.getNode(id)
	return n != nil && n.up && !n.removed
}

func (s *sim) Send(m *value.Record, from, to int) {
	s.msgID++
	id := s.msgID
	s.rec(trace.Fact{Ev: "net.Sent", Node: from, Msg: m, MsgID: id, From: from, To: to})
	delay := s.rng.TimeBetween(s.net.latA, s.net.latB)
	reason := ""
	k := dropKey{from, to, m.Type.Name}
	switch {
	case s.net.cut[mkLink(from, to)]:
		reason = "partition"
	case s.net.dropNext[k] > 0:
		s.net.dropNext[k]--
		reason = "targeted"
	case s.net.loss > 0 && s.rng.Chance(s.net.loss):
		reason = "loss"
	}
	kind := evReceive
	if reason != "" {
		kind = evDrop
	}
	s.q.push(&event{at: s.now + delay, kind: kind, node: to, rec: m, from: from, id: id, reason: reason, pos: s.curPos})
}

func (s *sim) SetLatency(a, b int64) { s.net.latA, s.net.latB = a, b }
func (s *sim) SetLoss(p float64)     { s.net.loss = p }

func (s *sim) Partition(a, b []int) {
	for _, x := range a {
		for _, y := range b {
			s.net.cut[mkLink(x, y)] = true
		}
	}
	s.rec(trace.Fact{Ev: "net.Partitioned", Node: -1, A: append([]int(nil), a...), B: append([]int(nil), b...)})
}

func (s *sim) Heal() {
	s.net.cut = map[link]bool{}
	s.rec(trace.Fact{Ev: "net.Healed", Node: -1})
}

func (s *sim) DropNext(typ string, from, to int) { s.net.dropNext[dropKey{from, to, typ}]++ }

func (s *sim) DiskWrite(id int, key string, v value.Value, t types.Type) {
	n := s.getNode(id)
	if n == nil {
		return
	}
	n.disk[key] = diskEntry{v, t}
	s.rec(trace.Fact{Ev: "disk.Written", Node: id, Key: key})
}

func (s *sim) DiskRead(id int, key string) (value.Value, types.Type, bool) {
	n := s.getNode(id)
	if n == nil {
		return value.Value{}, nil, false
	}
	d, ok := n.disk[key]
	return d.v, d.t, ok
}

func (s *sim) Accepts(a, b types.Type) bool {
	return types.Accepts(a, b, func(p, q string) bool {
		pk := s.byPath[p]
		if pk == nil {
			return false
		}
		for _, r := range pk.Runs {
			if r.Path == q {
				return true
			}
		}
		return false
	})
}
