package trace

import (
	"bytes"
	"strconv"
	"strings"
	"testing"

	"github.com/synodlang/compiler/internal/ir"
	"github.com/synodlang/compiler/internal/source"
	"github.com/synodlang/compiler/internal/value"
)

func sumOpts() Options {
	return Options{Paths: map[string]string{"../x/paxos.synod": "paxos/paxos.synod"}}
}

func summarize(t *testing.T, in SummaryInput) string {
	t.Helper()
	var b bytes.Buffer
	if err := WriteSummary(&b, in, sumOpts()); err != nil {
		t.Fatal(err)
	}
	return b.String()
}

func smallInput() SummaryInput {
	rule := "../x/paxos.synod:109: always ((lists.Size ChosenValues) at most 1)"
	pos := ir.Pos{File: "../x/paxos.synod", Line: 109, Col: 1}
	state := []value.Value{value.Number(0), value.ListOf([]value.Value{value.Text("a\"b"), value.Node(2)})}
	return SummaryInput{
		Header: Header{Seed: 7, Program: "examples/paxos"},
		Steps: []Step{
			{Index: 0, T: 0, Kind: "main"},
			{Index: 1, T: 10270, Kind: "receive", Node: 3},
			{Index: 2, T: 20000, Kind: "timer", Node: 1}, // no facts: not shown
			{Index: 3, T: 90000, Kind: "deliver", Node: 2},
		},
		Facts: []Fact{
			{Step: 0, Ev: "sim.Started", Node: -1},
			{Step: 0, Ev: "sim.Added", Node: 1, Runs: pxPaxos.Runs, State: state},
			{Step: 0, Ev: "sim.Injected", Node: 1, Record: prep(1, "s")},
			{Step: 1, T: 10270, Ev: "net.Arrived", Node: 3, Msg: prep(101, "hid"), MsgID: 5, From: 1, To: 3},
			{Step: 1, T: 10270, Ev: "sim.Changed", Node: 3, Field: fPromis, Old: value.Number(0), New: value.Number(101)},
			{Step: 1, T: 10270, Ev: "net.Dropped", Node: 3, Msg: prep(1, "x"), MsgID: 6, From: 1, To: 3, Reason: "crashed"},
			{Step: 1, T: 10270, Ev: "net.Partitioned", Node: -1, A: []int{1}, B: []int{2, 3}},
			{Step: 1, T: 10270, Ev: "disk.Written", Node: 3, Key: "pages"},
			{Step: 3, T: 90000, Ev: "sim.Broken", Node: -1, Rule: rule, RulePos: pos},
			{Step: 3, T: 90000, Ev: "sim.Emitted", Node: 0, Record: prep(2, "")},
			{Step: 3, T: 90000, Ev: "sim.Crashed", Node: 2},
			{Step: 3, T: 90000, Ev: "net.Healed", Node: -1},
			{Step: 3, T: 90000, Ev: "sim.Failed", Node: -1, Fault: &value.Fault{Code: source.IndexRange, Message: "m"}},
		},
		Final: []NodeState{
			{Node: 1, Up: true, Runs: pxPaxos.Runs, State: state},
			{Node: 2, Up: false, Runs: pxPaxos.Runs, State: state},
			{Node: 3, Removed: true, Runs: []*ir.Package{pxNet}},
		},
		Invariants: []InvariantResult{
			{Rule: "../x/paxos.synod:80: always Atomic", Pos: ir.Pos{File: "../x/paxos.synod", Line: 80}, Holds: true},
			{Rule: rule, Pos: pos, Holds: true, Breaks: 2, FirstBreak: 1200000},
		},
		End: 2000000,
		Failed: &value.Fault{Code: source.IndexRange, Message: "item 0 of a list of 0 items",
			Pos: ir.Pos{File: "../x/paxos.synod", Line: 12, Col: 5}, HasPos: true},
		TracePath: "out/trace.jsonl",
	}
}

func TestSummaryGolden(t *testing.T) {
	golden(t, "summary-small.txt", summarize(t, smallInput()))
	in := smallInput()
	in.Failed, in.EndReason = nil, "stop"
	golden(t, "summary-stop.txt", summarize(t, in))
}

func bigInput(nFacts, perStep int) SummaryInput {
	in := SummaryInput{Header: Header{Seed: 1, Program: "big"}, EndReason: "empty", TracePath: "out/trace.jsonl"}
	for i := 0; i < nFacts; i++ {
		step := i / perStep
		if i%perStep == 0 {
			in.Steps = append(in.Steps, Step{Index: step, T: int64(step) * 1000, Kind: "timer", Node: 1})
		}
		in.Facts = append(in.Facts, Fact{Step: step, Ev: "sim.Fired", Node: 1 + i%3, T: int64(step) * 1000})
	}
	in.End = int64(len(in.Steps)) * 1000
	return in
}

func TestSummaryCap(t *testing.T) {
	// Exactly 400 fact lines: everything printed.
	out := summarize(t, bigInput(400, 1))
	if strings.Contains(out, "omitted") {
		t.Error("400 facts must not be cut")
	}
	// 401: cut.
	out = summarize(t, bigInput(401, 1))
	if !strings.Contains(out, "… 51 steps omitted (full trace: out/trace.jsonl)\n") {
		t.Errorf("note missing:\n%s", out)
	}
	if n := strings.Count(out, "sim.Fired"); n != 350 {
		t.Errorf("%d fact lines, want 350", n)
	}
	golden(t, "summary-cap-1.txt", out)
	// Steps of 7 facts: the cut falls inside steps and headers repeat.
	out = summarize(t, bigInput(1000, 7))
	if n := strings.Count(out, "sim.Fired"); n != 350 {
		t.Errorf("%d fact lines, want 350", n)
	}
	golden(t, "summary-cap-7.txt", out)
	for _, l := range strings.Split(out, "\n") {
		if strings.HasPrefix(l, "… ") && !strings.HasPrefix(l, "… "+strconv.Itoa(0)) {
			return
		}
	}
	t.Error("no omitted note")
}

func TestSummaryAlwaysComplete(t *testing.T) {
	in := bigInput(1000, 7)
	in.Invariants = []InvariantResult{{Rule: "r", Holds: true}}
	in.Final = []NodeState{{Node: 1, Up: true}}
	out := summarize(t, in)
	if !strings.Contains(out, "always:\n  holds          r\n") || !strings.HasSuffix(out, "final:\n  n1 up\n") {
		t.Error(out[len(out)-200:])
	}
}
