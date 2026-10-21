package trace

import (
	"testing"

	"github.com/synodlang/compiler/internal/ir"
	"github.com/synodlang/compiler/internal/source"
	"github.com/synodlang/compiler/internal/value"
)

func TestFaultDiagnostic(t *testing.T) {
	o := Options{Paths: map[string]string{"../x/p.synod": "p/p.synod"}}
	f := (&value.Fault{
		Code: source.DivisionByZero, Message: "division by zero", HasPos: true,
		Pos: ir.Pos{File: "../x/p.synod", Line: 6, Col: 29, EndLine: 6, EndCol: 53},
	}).WithAlso(ir.Pos{File: "../x/p.synod", Line: 6, Col: 43, EndLine: 6, EndCol: 52}, "this is 0").
		WithAlso(ir.Pos{File: "../x/p.synod", Line: 6, Col: 1}, "no extent, left out").
		WithNote("a note")
	d := FaultDiagnostic(f, o)
	if d.File != "p/p.synod" || d.Line != 6 || d.Col != 29 || d.End != (source.Pos{Line: 6, Col: 53}) {
		t.Errorf("primary: %+v", d)
	}
	if len(d.Labels) != 1 || d.Labels[0].File != "p/p.synod" || d.Labels[0].End.Col != 52 || d.Labels[0].Text != "this is 0" {
		t.Errorf("labels: %+v", d.Labels)
	}
	if len(d.Notes) != 1 {
		t.Errorf("notes: %v", d.Notes)
	}
	// The trace keeps the start only.
	if got := o.pos(f.Pos); got != "p/p.synod:6:29" {
		t.Errorf("trace pos %q", got)
	}

	tl := FaultDiagnostic(&value.Fault{Code: source.TimeLimit, Message: "m"}, o)
	if tl.Line != 0 || tl.File != "" || len(tl.Help) != 1 {
		t.Errorf("time limit: %+v", tl)
	}
}
