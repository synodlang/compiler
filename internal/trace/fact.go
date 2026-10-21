// Package trace holds the simulator's record of a run (Fact, Step) and turns
// it into trace.jsonl and the agent summary. This file has only the data
// types, which the simulator fills and the encoder reads.
package trace

import (
	"github.com/synodlang/compiler/internal/ir"
	"github.com/synodlang/compiler/internal/value"
)

// Fact is one trace event. Ev is the event name ("net.Sent", "sim.Changed",
// …); only the fields that the event uses (spec implementation/04-formats)
// are set.
type Fact struct {
	// Step is the index of the step that recorded the fact; T its time in
	// microseconds. Seq is implied by position (first fact is seq 1).
	Step int
	T    int64
	Ev   string
	// Node is the "node" key: a node id, 0 for null (Nobody), -1 for no key.
	Node int
	// Msg, MsgID, From, To, Reason: net.* events. Msg is the sent record.
	Msg    *value.Record
	MsgID  int
	From   int
	To     int
	Reason string
	// Record is the record of sim.Injected and sim.Emitted; Field, Old and
	// New belong to sim.Changed.
	Record *value.Record
	Field  *ir.Field
	Old    value.Value
	New    value.Value
	// Runs and State belong to sim.Added and sim.Restarted. State holds one
	// value per exported node field, in Runs order and then declaration
	// order, so State[i] belongs to the i-th exported field.
	Runs  []*ir.Package
	State []value.Value
	// A and B are the node ids of net.Partitioned, in the order given; Key
	// is the key of disk.Written.
	A, B []int
	Key  string
	// Rule and RulePos belong to sim.Broken (Rule uses display paths).
	Rule    string
	RulePos ir.Pos
	// Fault belongs to sim.Failed.
	Fault *value.Fault
}

// Step is one simulation step: the unit that the summary groups facts by.
type Step struct {
	Index int
	T     int64
	// Kind is main, start, deliver, receive, drop or timer.
	Kind string
	// Node is the node the step ran on (0 for main).
	Node int
	// Pos is the position of the code that scheduled or started the step,
	// when known (zero otherwise).
	Pos ir.Pos
}

// Header is the first line of trace.jsonl.
type Header struct {
	// Seed is the seed in force when main returned.
	Seed uint64
	// Program is the import path of the main package, without "#main".
	Program string
	// Packages are the import paths of all other packages of the program,
	// sorted by byte order.
	Packages []string
}

// NodeState is the state of a node at the end of the run.
type NodeState struct {
	Node    int
	Up      bool
	Removed bool
	Runs    []*ir.Package
	// State is laid out like Fact.State.
	State []value.Value
}

// InvariantResult is the summary of one always line over the whole run.
type InvariantResult struct {
	Rule string
	Pos  ir.Pos
	// Holds is the value at the end of the run; Breaks counts true-to-false
	// changes and FirstBreak is the time of the first (microseconds).
	Holds      bool
	Breaks     int
	FirstBreak int64
}

// SummaryInput is what WriteSummary needs: the whole result of a run.
type SummaryInput struct {
	Header     Header
	Facts      []Fact
	Steps      []Step
	Final      []NodeState
	Invariants []InvariantResult
	// EndReason is "stop" or "empty"; empty on failure.
	EndReason string
	// End is the time of the end of the run in microseconds.
	End    int64
	Failed *value.Fault
	// TracePath is the trace file named in the "steps omitted" note.
	TracePath string
}

// Options configure Encode and WriteSummary.
type Options struct {
	// Paths maps a display path (ir.Pos.File) to its trace path.
	Paths map[string]string
	// Qual names a package in qualified names (see Qualifiers).
	Qual func(*ir.Package) string
}
