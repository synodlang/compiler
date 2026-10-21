package trace

import (
	"testing"

	"github.com/synodlang/compiler/internal/ir"
)

func TestQualifiers(t *testing.T) {
	a := &ir.Package{Path: "a.com/raft", Name: "raft"}
	b := &ir.Package{Path: "b.com/raft", Name: "raft"}
	c := &ir.Package{Path: "c.com/raft", Name: "raft"}
	r2 := &ir.Package{Path: "d.com/raft2", Name: "raft2"}
	n := &ir.Package{Path: "std/net", Name: "net", IsStd: true}
	other := &ir.Package{Path: "z/other", Name: "other"}
	// Program order is dependency order, not path order.
	q := Qualifiers(&ir.Program{Packages: []*ir.Package{c, n, r2, b, a}})
	want := map[*ir.Package]string{a: "raft", b: "raft3", c: "raft4", r2: "raft2", n: "net", other: "other"}
	for p, w := range want {
		if got := q(p); got != w {
			t.Errorf("%s: got %q want %q", p.Path, got, w)
		}
	}
}

func TestQualifiersNoCollision(t *testing.T) {
	a := &ir.Package{Path: "x/a", Name: "a"}
	q := Qualifiers(&ir.Program{Packages: []*ir.Package{a}})
	if q(a) != "a" {
		t.Error(q(a))
	}
}
