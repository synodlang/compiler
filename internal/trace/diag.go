package trace

import (
	"github.com/synodlang/compiler/internal/ir"
	"github.com/synodlang/compiler/internal/source"
	"github.com/synodlang/compiler/internal/value"
)

// FaultDiagnostic returns the failure f as a diagnostic for the rich
// renderer. File is the trace path of the failing file (see Options.Paths),
// so the renderer needs a source lookup keyed by trace path. A fault without
// a position gives a diagnostic with Line 0 and no file. The span is the
// extent of the failing expression or statement; its start is the position
// that the trace records.
func FaultDiagnostic(f *value.Fault, o Options) source.Diagnostic {
	d := source.Diagnostic{Severity: "error", Code: f.Code, Message: f.Message}
	if f.HasPos {
		d.File = o.file(f.Pos.File)
		d.Line, d.Col = f.Pos.Line, f.Pos.Col
		d.End = faultEnd(f.Pos)
		d.Label = f.Label
	}
	for _, l := range f.Also {
		d.Labels = append(d.Labels, source.Label{
			File: o.file(l.Pos.File), Line: l.Pos.Line, Col: l.Pos.Col, End: faultEnd(l.Pos), Text: l.Text,
		})
	}
	d.Notes = append(d.Notes, f.Notes...)
	if f.HasInner {
		d.Notes = append(d.Notes, "raised inside std, at "+o.pos(f.Inner))
	}
	if f.Code == source.TimeLimit {
		d.Help = []source.Help{{Text: "end the run earlier with sim.StopAt"}}
	}
	return d
}

// faultEnd is the end of the extent of p; a position with no extent ends
// where it starts.
func faultEnd(p ir.Pos) source.Pos {
	if p.EndLine == p.Line && p.EndCol > p.Col {
		return source.Pos{Line: p.EndLine, Col: p.EndCol}
	}
	return source.Pos{Line: p.Line, Col: p.Col}
}
