package scene

import (
	"reflect"
	"testing"

	"github.com/synodlang/compiler/internal/ir"
	"github.com/synodlang/compiler/internal/trace"
	"github.com/synodlang/compiler/internal/value"
)

func testPkgs() (*ir.Package, *ir.Package, *ir.Field, *ir.Field, *ir.Field) {
	a := &ir.Package{Path: "a", Name: "a"}
	b := &ir.Package{Path: "b", Name: "b"}
	fx := &ir.Field{Pkg: a, Name: "X", Exported: true}
	fh := &ir.Field{Pkg: a, Name: "hidden"}
	fy := &ir.Field{Pkg: b, Name: "Y", Exported: true}
	a.NodeFields = []*ir.Field{fx, fh}
	b.NodeFields = []*ir.Field{fy}
	return a, b, fx, fy, fh
}

func num(f float64) value.Value { return value.Value{K: value.KNumber, Num: f} }

func TestStoreReplay(t *testing.T) {
	a, b, fx, fy, _ := testPkgs()
	st := newStore()
	add := func(n int, runs []*ir.Package, vals ...value.Value) trace.Fact {
		return trace.Fact{Ev: "sim.Added", Node: n, Runs: runs, State: vals}
	}
	f1 := add(1, []*ir.Package{a, b}, num(1), num(2))
	if got := st.changes(f1); !reflect.DeepEqual(got, []readKey{{0, slotMembership}}) {
		t.Fatalf("added changes %v", got)
	}
	st.apply(f1)
	st.apply(add(2, []*ir.Package{a}, num(10)))
	if !reflect.DeepEqual(st.nodes(), []int{1, 2}) {
		t.Fatalf("nodes %v", st.nodes())
	}
	if !reflect.DeepEqual(st.nodesRunning(b), []int{1}) || !reflect.DeepEqual(st.nodesRunning(a), []int{1, 2}) {
		t.Fatalf("nodesRunning b=%v a=%v", st.nodesRunning(b), st.nodesRunning(a))
	}
	if v, ok := st.field(1, fy); !ok || v.Num != 2 {
		t.Fatalf("field Y = %v %v", v, ok)
	}
	if _, ok := st.field(2, fy); ok {
		t.Fatal("node 2 has no Y")
	}
	rows := st.rows(1)
	if len(rows) != 2 || rows[0].Field != fx || rows[1].Field != fy || rows[1].Value.Num != 2 {
		t.Fatalf("rows %v", rows)
	}

	ch := trace.Fact{Ev: "sim.Changed", Node: 1, Field: fx, Old: num(1), New: num(5)}
	if got := st.changes(ch); !reflect.DeepEqual(got, []readKey{{1, st.slotOf(fx)}}) {
		t.Fatalf("changed changes %v", got)
	}
	st.apply(ch)
	if v, _ := st.field(1, fx); v.Num != 5 {
		t.Fatalf("after changed %v", v)
	}

	cr := trace.Fact{Ev: "sim.Crashed", Node: 1}
	if got := st.changes(cr); !reflect.DeepEqual(got, []readKey{{1, slotUp}}) {
		t.Fatalf("crashed changes %v", got)
	}
	st.apply(cr)
	if st.isUp(1) || !st.isUp(2) {
		t.Fatal("isUp after crash")
	}
	if len(st.nodes()) != 2 {
		t.Fatal("crashed node still listed")
	}

	// restart: X differs (5 -> 1), Y does not (2 -> 2); up changes
	rs := trace.Fact{Ev: "sim.Restarted", Node: 1, Runs: []*ir.Package{a, b}, State: []value.Value{num(1), num(2)}}
	want := []readKey{{1, st.slotOf(fx)}, {1, slotUp}}
	if got := st.changes(rs); !reflect.DeepEqual(got, want) {
		t.Fatalf("restarted changes %v want %v", got, want)
	}
	st.apply(rs)
	if !st.isUp(1) {
		t.Fatal("restart did not bring the node up")
	}
	if v, _ := st.field(1, fx); v.Num != 1 {
		t.Fatalf("snapshot not loaded: %v", v)
	}

	ver := st.memberVer
	rm := trace.Fact{Ev: "sim.Removed", Node: 2}
	if got := st.changes(rm); !reflect.DeepEqual(got, []readKey{{2, slotUp}, {0, slotMembership}}) {
		t.Fatalf("removed changes %v", got)
	}
	st.apply(rm)
	if st.isUp(2) || !st.removed(2) || !reflect.DeepEqual(st.nodes(), []int{1}) || st.memberVer == ver {
		t.Fatal("removed node state")
	}
	if got := st.changes(trace.Fact{Ev: "net.Sent"}); got != nil {
		t.Fatalf("net.Sent changes %v", got)
	}
}
