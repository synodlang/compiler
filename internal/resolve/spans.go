package resolve

import (
	"github.com/synodlang/compiler/internal/parse"
	"github.com/synodlang/compiler/internal/source"
)

// This file holds the small helpers that build spans, labels and edits for
// the diagnostics of the resolver.

// also is a secondary label in the file f.
func (f *File) also(sp source.Span, text string) source.Extra {
	return source.Also(f.Path, sp, text)
}

// sp is the span of an item.
func sp(it parse.Item) source.Span { return parse.SpanOf(it) }

// deleteLine is the edit that removes line n whole.
func deleteLine(n int) source.Edit {
	return source.Edit{Line: n, Col: 1, End: source.Pos{Line: n + 1, Col: 1}}
}

// insertAt is the edit that inserts text at p.
func insertAt(p source.Pos, text string) source.Edit { return source.EditAt(source.At(p), text) }

// parenEdits are the two insertions that wrap s in parentheses.
func parenEdits(s source.Span) []source.Edit {
	return []source.Edit{insertAt(s.Start, "("), insertAt(s.End, ")")}
}

// lowerEdit is the edit that lower-cases the first letter of the name that
// starts at p.
func lowerEdit(p source.Pos, name string) source.Edit {
	return source.EditAt(source.Span{Start: p, End: source.Pos{Line: p.Line, Col: p.Col + 1}}, lowerFirst(name)[:1])
}

// runeSpan is the span of n code points from p.
func runeSpan(p source.Pos, n int) source.Span {
	return source.Span{Start: p, End: source.Pos{Line: p.Line, Col: p.Col + n}}
}

// spanTo is the span from the start of a to the end of b.
func spanTo(a, b source.Span) source.Span { return source.Span{Start: a.Start, End: b.End} }

// notImported is the help of a package of std that a file uses without
// importing it: the text, and the edit that adds the use line where
// canonical order puts it, when there is one.
func (f *File) notImported(q string) source.Extra {
	text := `add "use std/` + q + `"`
	if e, ok := f.useEdit("std/" + q); ok {
		return source.HelpEdit(text, e)
	}
	return source.HelpText(text)
}

// useEdit returns the edit that inserts "use path" among the use lines of f
// in the order that synod fmt keeps (sorted by path, before everything else).
func (f *File) useEdit(path string) (source.Edit, bool) {
	tree := f.Load.Tree
	src := f.Load.Src
	var uses []*parse.Line
	for _, n := range tree.Body {
		if l, ok := n.(*parse.Line); ok && parse.IsWord(l.Sentence.Groups[0][0], "use") {
			uses = append(uses, l)
		}
	}
	line := func(l *parse.Line) int {
		if len(l.Leading) > 0 {
			return l.Leading[0].Pos.Line
		}
		return l.Pos.Line
	}
	at := func(n int, text string) (source.Edit, bool) {
		if n < 1 || n > src.LineCount() {
			return source.Edit{}, false
		}
		return insertAt(source.Pos{Line: n, Col: 1}, text), true
	}
	for _, l := range uses {
		g := l.Sentence.Groups[0]
		if len(g) > 1 {
			if p, ok := g[1].(*parse.Path); ok && p.Text > path {
				return at(line(l), "use "+path+"\n")
			}
		}
	}
	if len(uses) > 0 {
		return at(uses[len(uses)-1].Pos.Line+1, "use "+path+"\n")
	}
	if h := tree.Header; h != nil {
		if src.Line(h.Pos.Line+1) == "" && h.Pos.Line+2 <= src.LineCount() {
			return at(h.Pos.Line+2, "use "+path+"\n\n")
		}
		return at(h.Pos.Line+1, "use "+path+"\n")
	}
	return at(1, "use "+path+"\n\n")
}

// Texts of the notes that more than one site writes.
const builtinTypesNote = "built-in types: text, number, bool, time, node, msg, shape, color, field, package, block, value"
