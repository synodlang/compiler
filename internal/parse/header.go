package parse

import (
	"strings"

	"github.com/synodlang/compiler/internal/source"
)

// headerPrefix starts the text of a package header comment.
const headerPrefix = "package "

// packageName returns the name of a header text such as "package gossip", or
// "" if the name is not a single word [A-Za-z][A-Za-z0-9]*.
func packageName(text string) string {
	name := strings.TrimPrefix(text, headerPrefix)
	if name == "" {
		return ""
	}
	for i := 0; i < len(name); i++ {
		c := name[i]
		letter := c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
		if !letter && (i == 0 || c < '0' || c > '9') {
			return ""
		}
	}
	return name
}

// HeaderDiagnostic checks the package header of f against folder, the base
// name of the folder holding the file (spec/06 section 6.1). The header must
// be "# package <word>" with the word equal to folder or to "main". It
// returns nil if the header is fine.
func HeaderDiagnostic(f *File, folder string) *source.Diagnostic {
	fail := func(p source.Pos, msg, hint string) *source.Diagnostic {
		d := source.New(f.Source, p, source.BadHeader, msg, hint)
		return &d
	}
	switch {
	case f.Header == nil:
		return fail(source.Pos{Line: 1, Col: 1}, `missing "# package" header`,
			`the first line is "# package <folder>"`)
	case f.Package == "":
		return fail(f.Header.Pos, `bad package header "# `+f.Header.Text+`"`,
			`write "# package <name>"`)
	case f.Package != folder && f.Package != "main":
		return fail(f.Header.Pos,
			`package "`+f.Package+`" must be the folder name "`+folder+`" or main`, "")
	}
	return nil
}
