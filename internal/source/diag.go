package source

import (
	"sort"
	"strconv"
	"strings"
)

// Diagnostic is one compile-time error. The field order is the key order of
// check.json (implementation/04-formats.md).
type Diagnostic struct {
	File     string `json:"file"`
	Line     int    `json:"line"`
	Col      int    `json:"col"`
	End      Pos    `json:"end"`      // end of the primary span, exclusive
	Severity string `json:"severity"` // always "error"
	Code     Code   `json:"code"`
	Message  string `json:"message"`
	Hint     string `json:"hint,omitempty"`

	// Rich diagnostics (spec/11 section 11.1). Hint is kept verbatim for the
	// one-line form; the rest is only used by the rich renderer and JSON.
	Label  string   `json:"label,omitempty"`  // text under the primary span
	Labels []Label  `json:"labels,omitempty"` // secondary labels
	Help   []Help   `json:"help,omitempty"`
	Notes  []string `json:"notes,omitempty"`
}

// New returns an error diagnostic for file f at position p. The primary span
// is empty: it marks the column of p.
func New(f *File, p Pos, code Code, msg, hint string) Diagnostic {
	return Diagnostic{
		File:     f.Path,
		Line:     p.Line,
		Col:      p.Col,
		End:      p,
		Severity: "error",
		Code:     code,
		Message:  msg,
		Hint:     hint,
	}
}

// NewSpan returns an error diagnostic whose primary span is sp, with the
// optional extras (label, secondary labels, help, notes) applied.
func NewSpan(f *File, sp Span, code Code, msg, hint string, more ...Extra) Diagnostic {
	d := Diagnostic{
		File:     f.Path,
		Line:     sp.Start.Line,
		Col:      sp.Start.Col,
		End:      sp.End,
		Severity: "error",
		Code:     code,
		Message:  msg,
		Hint:     hint,
	}
	for _, m := range more {
		m(&d)
	}
	return d
}

// DiagKey identifies a diagnostic for de-duplication.
type DiagKey struct {
	File      string
	Line, Col int
	Code      Code
	Message   string
}

// Key returns the identity of d: two diagnostics with equal keys are
// duplicates.
func (d Diagnostic) Key() DiagKey {
	return DiagKey{d.File, d.Line, d.Col, d.Code, d.Message}
}

// String formats d as "path:L:C: error[code]: message", followed by
// "\n  hint: ..." when there is a hint (spec/11 section 11.1).
func (d Diagnostic) String() string {
	var b strings.Builder
	b.WriteString(d.Header())
	if d.Hint != "" {
		b.WriteString("\n  hint: ")
		b.WriteString(d.Hint)
	}
	return b.String()
}

// Header returns the first line of String: "path:L:C: error[code]: message".
// A diagnostic without a position (Line 0) has no ":L:C": "path: error[code]:
// message".
func (d Diagnostic) Header() string {
	var b strings.Builder
	b.WriteString(d.File)
	if d.Line > 0 {
		b.WriteByte(':')
		b.WriteString(strconv.Itoa(d.Line))
		b.WriteByte(':')
		b.WriteString(strconv.Itoa(d.Col))
	}
	b.WriteString(": ")
	b.WriteString(d.Severity)
	b.WriteByte('[')
	b.WriteString(string(d.Code))
	b.WriteString("]: ")
	b.WriteString(d.Message)
	return b.String()
}

// Sort orders ds by file, line, column and code. The sort is stable.
func Sort(ds []Diagnostic) {
	sort.SliceStable(ds, func(i, j int) bool {
		a, b := ds[i], ds[j]
		if a.File != b.File {
			return a.File < b.File
		}
		if a.Line != b.Line {
			return a.Line < b.Line
		}
		if a.Col != b.Col {
			return a.Col < b.Col
		}
		return a.Code < b.Code
	})
}
