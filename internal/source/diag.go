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
	Severity string `json:"severity"` // always "error"
	Code     Code   `json:"code"`
	Message  string `json:"message"`
	Hint     string `json:"hint,omitempty"`
}

// New returns an error diagnostic for file f at position p.
func New(f *File, p Pos, code Code, msg, hint string) Diagnostic {
	return Diagnostic{
		File:     f.Path,
		Line:     p.Line,
		Col:      p.Col,
		Severity: "error",
		Code:     code,
		Message:  msg,
		Hint:     hint,
	}
}

// String formats d as "path:L:C: error[code]: message", followed by
// "\n  hint: ..." when there is a hint (spec/11 section 11.1).
func (d Diagnostic) String() string {
	var b strings.Builder
	b.WriteString(d.File)
	b.WriteByte(':')
	b.WriteString(strconv.Itoa(d.Line))
	b.WriteByte(':')
	b.WriteString(strconv.Itoa(d.Col))
	b.WriteString(": ")
	b.WriteString(d.Severity)
	b.WriteByte('[')
	b.WriteString(string(d.Code))
	b.WriteString("]: ")
	b.WriteString(d.Message)
	if d.Hint != "" {
		b.WriteString("\n  hint: ")
		b.WriteString(d.Hint)
	}
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
