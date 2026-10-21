package trace

import (
	"github.com/synodlang/compiler/internal/source"
	"github.com/synodlang/compiler/internal/value"
)

// FaultDiagnostic returns the failure f as a diagnostic for the rich
// renderer. File is the trace path of the failing file (see Options.Paths),
// so the renderer needs a source lookup keyed by trace path. A fault without
// a position gives a diagnostic with Line 0 and no file.
func FaultDiagnostic(f *value.Fault, o Options) source.Diagnostic {
	d := source.Diagnostic{Severity: "error", Code: f.Code, Message: f.Message}
	if f.HasPos {
		d.File = o.file(f.Pos.File)
		d.Line, d.Col = f.Pos.Line, f.Pos.Col
		d.End = source.Pos{Line: f.Pos.Line, Col: f.Pos.Col}
	}
	return d
}
