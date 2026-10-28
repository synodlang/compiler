package parse

import (
	"strings"

	"github.com/synodlang/compiler/internal/source"
)

// headerPrefix starts the text of a package header comment, themePrefix that
// of a theme header (spec/06 section 6.7).
const (
	headerPrefix = "package "
	themePrefix  = "theme "
)

// isHeaderText reports whether the text of an own-line comment starts a
// header, "package ..." or "theme ...".
func isHeaderText(text string) bool {
	return strings.HasPrefix(text, headerPrefix) || strings.HasPrefix(text, themePrefix)
}

// packageName returns the name of a header text such as "package gossip" or
// "theme terminal", or "" if the name is not a single word
// [A-Za-z][A-Za-z0-9]*.
func packageName(text string) string {
	name := strings.TrimPrefix(strings.TrimPrefix(text, headerPrefix), themePrefix)
	if name == text {
		return ""
	}
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

// HeaderDiagnostic checks the header of f against folder, the base name of
// the folder holding the file (spec/06 sections 6.1 and 6.7). The header must
// be "# package <word>" with the word equal to folder or to "main", or
// "# theme <word>" with the word equal to folder. It returns nil if the
// header is fine.
func HeaderDiagnostic(f *File, folder string) *source.Diagnostic {
	fail := func(sp source.Span, msg, hint string, more ...source.Extra) *source.Diagnostic {
		d := source.NewSpan(f.Source, sp, source.BadHeader, msg, hint, more...)
		return &d
	}
	want := headerPrefixMark + folder
	switch {
	case f.Header == nil:
		top := source.At(source.Pos{Line: 1, Col: 1})
		return fail(top, `missing "# package" header`,
			`the first line is "# package <folder>"`,
			source.HelpEdit(`add the header "`+want+`" as the first line`, source.EditAt(top, want+"\n")))
	case f.Package == "":
		sp := CommentSpan(f.Header)
		return fail(sp, `bad package header "# `+f.Header.Text+`"`,
			`write "# package <name>"`,
			source.HelpEdit(`write "`+want+`"`, source.EditAt(sp, want)))
	case f.Theme && f.Package != folder:
		nameAt := source.Pos{Line: f.Header.Pos.Line, Col: f.Header.Pos.Col + width("# "+themePrefix)}
		name := source.Span{Start: nameAt, End: after(nameAt, width(f.Package))}
		return fail(CommentSpan(f.Header),
			`theme "`+f.Package+`" must be the folder name "`+folder+`"`, "",
			source.HelpEdit(`write the folder name "`+folder+`"`, source.EditAt(name, folder)),
			source.Note(`a main file writes "# package main"`))
	case !f.Theme && f.Package != folder && f.Package != "main":
		// The span is the whole header, so that the position stays at its
		// start; the edit changes the name.
		nameAt := source.Pos{Line: f.Header.Pos.Line, Col: f.Header.Pos.Col + width(headerPrefixMark)}
		name := source.Span{Start: nameAt, End: after(nameAt, width(f.Package))}
		return fail(CommentSpan(f.Header),
			`package "`+f.Package+`" must be the folder name "`+folder+`" or main`, "",
			source.HelpEdit(`write the folder name "`+folder+`"`, source.EditAt(name, folder)),
			source.Note(`a main file writes "main"`))
	}
	return nil
}

// headerPrefixMark is the header as written up to the name.
const headerPrefixMark = "# " + headerPrefix
