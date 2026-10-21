// Package natives implements the native phrases of the standard library
// (std/sim, net, time, random, lists, maps, math, text, disk) for the
// simulator. A native is a Func that decodes its arguments, checks their
// domains, and acts on the run through the Host interface; it never touches
// simulator internals. Natives are deterministic: all randomness comes from
// Host.RNG, all time from Host.Now.
//
// Table has exactly one entry per stdlib native id of kind phrase, except
// look.* (looks run in a later milestone) and event ids (which are handled by
// the simulator, not called).
package natives

import (
	"github.com/synodlang/compiler/internal/ir"
	"github.com/synodlang/compiler/internal/rng"
	"github.com/synodlang/compiler/internal/types"
	"github.com/synodlang/compiler/internal/value"
)

// Host is what natives may ask of the running simulation. Node ids are the
// ids of value.Node (0 is sim.Nobody, which the natives check first).
// Times are microseconds.
type Host interface {
	// Now is the current simulated time.
	Now() int64
	// RNG is the run's generator; every use consumes the draws documented in
	// package rng.
	RNG() *rng.PCG32

	// Seed reseeds the generator (sim.Seed); ignored when the run was given
	// an explicit seed.
	Seed(seed uint64)
	// StopAt sets the time at which the run stops (sim.StopAt).
	StopAt(t int64)

	// Spawn creates a node running p and returns its id.
	Spawn(p *ir.Package) int
	// Crash, Restart and Remove act on node n as spec/07 describes; they are
	// no-ops where the spec says so.
	Crash(n int)
	Restart(n int)
	Remove(n int)
	// Nodes lists the nodes that are not removed, in creation order.
	Nodes() []int
	// NodesRunning lists the nodes that are not removed (crashed included)
	// and run package p, in creation order.
	NodesRunning(p *ir.Package) []int
	// IsUp reports whether n is up and not removed.
	IsUp(n int) bool

	// Send sends m from one node to another over the simulated network.
	Send(m *value.Record, from, to int)
	// SetLatency sets the delay range in microseconds; SetLoss the drop
	// probability.
	SetLatency(a, b int64)
	SetLoss(p float64)
	// Partition cuts every link between a node of a and a node of b; Heal
	// restores all links; DropNext drops the next message of record type
	// typ (unqualified name) sent from one node to another.
	Partition(a, b []int)
	Heal()
	DropNext(typ string, from, to int)

	// DiskWrite stores v, of static type t, under key on the disk of n.
	DiskWrite(n int, key string, v value.Value, t types.Type)
	// DiskRead returns the stored value and its static type, if any.
	DiskRead(n int, key string) (value.Value, types.Type, bool)
	// Accepts reports whether a value of type s may be used as type t
	// (types.Accepts with the program's "runs" relation).
	Accepts(s, t types.Type) bool
}

// Func is one native phrase. c is the call (for argument types and the
// position of faults, c.Pos()); args are the evaluated receiver, if any,
// then the inline arguments in order. A non-nil fault stops the run.
type Func func(h Host, c *ir.Call, args []value.Value) (value.Value, *value.Fault)

// Table maps a stdlib native id ("net.send") to its implementation. The
// files of this package fill it in init functions.
var Table = map[string]Func{}
