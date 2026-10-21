package lex

import (
	"testing"

	"github.com/synodlang/compiler/internal/source"
)

// TestErrorSpans checks the primary span, label and edits of the lexer's
// diagnostics, one source per kind of error.
func TestErrorSpans(t *testing.T) {
	type edit struct {
		line, col, endCol int
		new               string
	}
	tests := []struct {
		name  string
		src   string
		line  int
		from  int // column
		to    int // end column, exclusive
		label string
		help  int // number of helps
		edit  *edit
		note  bool
	}{
		{name: "invalid utf8", src: "set a to \xff\n", line: 1, from: 10, to: 11},
		{name: "lone CR", src: "set a\rb\n", line: 1, from: 6, to: 7},
		{name: "tab indent", src: "a\n\t\tb\n", line: 2, from: 1, to: 3, help: 1},
		{name: "odd indent", src: "a\n   b c\n", line: 2, from: 4, to: 5, label: "indented 3 spaces", note: true},
		{name: "unexpected indent", src: "a\n      b c\n", line: 2, from: 7, to: 8, label: "indented 6 spaces, at most 2 allowed", help: 1},
		{name: "first line indented", src: "  a\n", line: 1, from: 3, to: 4, label: "the first line starts at the left edge", help: 1},
		{name: "missing space", src: "set a 1\"x\"\n", line: 1, from: 8, to: 11, label: "add a space before this", help: 1,
			edit: &edit{1, 8, 8, " "}},
		{name: "leading dot", src: "set a to .5\n", line: 1, from: 10, to: 12, help: 1, edit: &edit{1, 10, 10, "0"}},
		{name: "plus", src: "set a to +1\n", line: 1, from: 10, to: 11, help: 1},
		{name: "underscore", src: "set a_b to 1\n", line: 1, from: 6, to: 7},
		{name: "unexpected character", src: "set a to 1 ; 2\n", line: 1, from: 12, to: 13},
		{name: "hash word", src: "set a to 1 #todo\n", line: 1, from: 12, to: 17, help: 1, edit: &edit{1, 13, 13, " "}, note: true},
		{name: "member number", src: "set a to x.1\n", line: 1, from: 10, to: 13, help: 1},
		{name: "bad member", src: "set a to x.\n", line: 1, from: 10, to: 12},
		{name: "minus", src: "set a to - 1\n", line: 1, from: 10, to: 11, help: 1},
		{name: "bad number", src: "set a to 12abc\n", line: 1, from: 10, to: 15},
		{name: "unterminated", src: "set a to \"abc\n", line: 1, from: 10, to: 14, label: "text starts here"},
		{name: "unknown escape", src: "set a to \"a\\qb\"\n", line: 1, from: 12, to: 14, help: 1},
		{name: "control in text", src: "set a to \"a\x01b\"\n", line: 1, from: 12, to: 13},
		{name: "tab", src: "set a\tto 1\n", line: 1, from: 6, to: 7},
		{name: "tab in path", src: "use \tx/y\n", line: 1, from: 5, to: 6},
		{name: "missing path", src: "use \n", line: 1, from: 5, to: 5, label: "path expected here"},
		{name: "bad path", src: "use a//b\n", line: 1, from: 5, to: 9},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := source.NewFile("t.synod", []byte(tc.src))
			_, d := Lex(f)
			if d == nil {
				t.Fatal("no diagnostic")
			}
			if d.Line != tc.line || d.Col != tc.from || d.End.Line != tc.line || d.End.Col != tc.to {
				t.Errorf("span %d:%d to %d:%d, want %d:%d to %d:%d (%s)", d.Line, d.Col, d.End.Line, d.End.Col, tc.line, tc.from, tc.line, tc.to, d.Message)
			}
			if d.Label != tc.label {
				t.Errorf("label %q, want %q", d.Label, tc.label)
			}
			if len(d.Help) != tc.help {
				t.Errorf("%d helps, want %d", len(d.Help), tc.help)
			}
			if (len(d.Notes) > 0) != tc.note {
				t.Errorf("notes %v", d.Notes)
			}
			if tc.edit != nil {
				e := d.Help[0].Edits
				want := source.Edit{Line: tc.edit.line, Col: tc.edit.col, End: source.Pos{Line: tc.edit.line, Col: tc.edit.endCol}, New: tc.edit.new}
				if len(e) != 1 || e[0] != want {
					t.Errorf("edits %+v, want %+v", e, want)
				}
			}
		})
	}
}

// TestTokenEnd checks that Token.End counts code points.
func TestTokenEnd(t *testing.T) {
	toks, d := Lex(source.NewFile("t.synod", []byte("set a to \"h\u00e9\u00e9\" # \u00fc\n")))
	if d != nil {
		t.Fatal(d)
	}
	want := map[Kind]int{WORD: 4, TEXT: 15, COMMENT: 19}
	for _, tk := range toks {
		if w, ok := want[tk.Kind]; ok && tk.Kind != WORD {
			if tk.End().Col != w {
				t.Errorf("%s ends at %d, want %d", tk.Kind, tk.End().Col, w)
			}
		}
	}
	if toks[0].End() != (source.Pos{Line: 1, Col: 4}) {
		t.Errorf("first token ends at %v", toks[0].End())
	}
}
