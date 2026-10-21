package parse

import (
	"path/filepath"
	"testing"

	"github.com/synodlang/compiler/internal/source"
)

// walkItems calls fn for every item of the tree, nested items included, and
// tf for every type.
func walkItems(f *File, fn func(Item), tf func(Type)) {
	var sent func(s *Sentence)
	var item func(it Item)
	var typ func(t Type)
	typ = func(t Type) {
		tf(t)
		switch t := t.(type) {
		case *ListType:
			typ(t.Elem)
		case *MapType:
			typ(t.Key)
			typ(t.Value)
		}
	}
	item = func(it Item) {
		fn(it)
		switch it := it.(type) {
		case *Paren:
			sent(it.S)
		case *List:
			for _, e := range it.Items {
				item(e)
			}
		case *Hole:
			item(it.Name)
			typ(it.Type)
		}
	}
	sent = func(s *Sentence) {
		for _, g := range s.Groups {
			for _, it := range g {
				item(it)
			}
		}
	}
	var lines func(ls []*Line)
	lines = func(ls []*Line) {
		for _, l := range ls {
			sent(l.Sentence)
			if l.Block != nil {
				lines(l.Block.Lines)
			}
			for _, c := range l.Conts {
				lines(c.Block.Lines)
			}
		}
	}
	for _, n := range f.Body {
		if l, ok := n.(*Line); ok {
			lines([]*Line{l})
		}
	}
}

// checkEnds verifies that every item and type ends after it starts, on its
// line.
func checkEnds(t testing.TB, f *File) {
	t.Helper()
	walkItems(f, func(it Item) {
		if e := it.End(); e.Line != it.Pos().Line || e.Col <= it.Pos().Col {
			t.Errorf("%T at %v ends at %v", it, it.Pos(), e)
		}
	}, func(ty Type) {
		if e := ty.End(); e.Line != ty.Pos().Line || e.Col <= ty.Pos().Col {
			t.Errorf("%T at %v ends at %v", ty, ty.Pos(), e)
		}
	})
}

// TestEndsOfExamples checks End on every file of the corpus.
func TestEndsOfExamples(t *testing.T) {
	n := 0
	for _, dir := range []string{"examples", "parse"} {
		walkSynod(t, filepath.Join(testdata, dir), func(path string, src []byte) {
			f, d := Parse(source.NewFile(path, src))
			if d != nil {
				return
			}
			n++
			checkEnds(t, f)
		})
	}
	if n < 20 {
		t.Errorf("only %d files checked", n)
	}
}

// TestSpanHelpers pins the extents of items, types, sentences, parts and
// comments on a line with multi-byte text.
func TestSpanHelpers(t *testing.T) {
	line := "define put [k: map of text to list of number] (x.y.z {1, 2}) \"h\u00e9\" 5ms # c\u00f6mment"
	src := "# package p\n\n" + line + "\n"
	f, d := Parse(source.NewFile("p.synod", []byte(src)))
	if d != nil {
		t.Fatal(d)
	}
	runes := []rune(line)
	// at returns the span of the text from the first rune of from, to the
	// end of the first occurrence of to at or after it.
	at := func(from, to string) source.Span {
		i := indexRunes(runes, from, 0)
		j := indexRunes(runes, to, i) + len([]rune(to))
		return source.Span{Start: source.Pos{Line: 3, Col: i + 1}, End: source.Pos{Line: 3, Col: j + 1}}
	}
	ln := f.Body[0].(*Line)
	g := ln.Sentence.Groups
	hole := g[0][2].(*Hole)
	if got, want := SpanOf(hole), at("[k", "number]"); got != want {
		t.Errorf("hole %v, want %v", got, want)
	}
	if got, want := TypeSpan(hole.Type), at("map of", "number"); got != want {
		t.Errorf("type %v, want %v", got, want)
	}
	if got, want := TypeSpan(hole.Type.(*MapType).Value), at("list of", "number"); got != want {
		t.Errorf("value type %v, want %v", got, want)
	}
	paren := g[0][3].(*Paren)
	if got, want := SpanOf(paren), at("(x", "2})"); got != want {
		t.Errorf("paren %v, want %v", got, want)
	}
	if got, want := SentenceSpan(paren.S), at("x.y.z", "2}"); got != want {
		t.Errorf("paren sentence %v, want %v", got, want)
	}
	m := paren.S.Groups[0][0].(*Member)
	whole := at("x.y.z", "x.y.z")
	for i := range m.Parts {
		col := whole.Start.Col + 2*i
		want := source.Span{Start: source.Pos{Line: 3, Col: col}, End: source.Pos{Line: 3, Col: col + 1}}
		if got := PartSpan(m, i); got != want {
			t.Errorf("part %d %v, want %v", i, got, want)
		}
	}
	if got, want := SpanOf(paren.S.Groups[0][1]), at("{1", "2}"); got != want {
		t.Errorf("list %v, want %v", got, want)
	}
	if got, want := SpanOfItems(g[0][4:]), at("\"h", "5ms"); got != want {
		t.Errorf("items %v, want %v", got, want)
	}
	if got, want := LineSpan(ln), at("define", "5ms"); got != want {
		t.Errorf("line %v, want %v", got, want)
	}
	if got, want := CommentSpan(ln.Comment), at("# c", "mment"); got != want {
		t.Errorf("comment %v, want %v", got, want)
	}
	if got, want := CommentSpan(f.Header), (source.Span{Start: source.Pos{Line: 1, Col: 1}, End: source.Pos{Line: 1, Col: 12}}); got != want {
		t.Errorf("header %v, want %v", got, want)
	}
	checkEnds(t, f)
}

// indexRunes returns the index of the first occurrence of sub in rs at or
// after from, or -1.
func indexRunes(rs []rune, sub string, from int) int {
	sr := []rune(sub)
	for i := from; i+len(sr) <= len(rs); i++ {
		if string(rs[i:i+len(sr)]) == sub {
			return i
		}
	}
	return -1
}

// TestErrorSpans checks the primary span, label and edits of the parser's
// diagnostics.
func TestErrorSpans(t *testing.T) {
	tests := []struct {
		name  string
		src   string
		line  int
		from  int
		to    int
		label string
		also  string // text of the one secondary label, "" for none
		edits int    // edits of the first help, 0 for no help
	}{
		{name: "stray comma", src: "a b ,\n", line: 1, from: 5, to: 6},
		{name: "leading comma", src: "a\n, b\n", line: 2, from: 1, to: 2},
		{name: "empty parens", src: "a ( ) b\n", line: 1, from: 3, to: 6},
		{name: "unclosed paren", src: "a (b c d\n", line: 1, from: 3, to: 9, label: "unclosed here"},
		{name: "unclosed paren before comment", src: "a (b c # x\n", line: 1, from: 3, to: 7, label: "unclosed here"},
		{name: "unclosed list", src: "a {1, 2\n", line: 1, from: 3, to: 8, label: "unclosed here"},
		{name: "list item run", src: "a {1 b c, 2}\n", line: 1, from: 4, to: 9, edits: 2},
		{name: "missing list item", src: "a {1, , 2}\n", line: 1, from: 7, to: 8},
		{name: "missing list item before brace", src: "a {1,}\n", line: 1, from: 6, to: 7},
		{name: "hole outside define", src: "a [x: number] b\n", line: 1, from: 3, to: 14},
		{name: "bad hole", src: "define a [x number] b\n  c\n", line: 1, from: 10, to: 20},
		{name: "bad type", src: "define a [x: list number]\n  c\n", line: 1, from: 14, to: 18},
		{name: "continuation", src: "a b\nelse\n  c\n", line: 2, from: 1, to: 5, also: "this line has no block"},
		{name: "continuation first", src: "else\n  c\n", line: 1, from: 1, to: 5},
		{name: "nested use", src: "a b\n  use x\n  c\n", line: 2, from: 3, to: 8},
		{name: "use without path", src: "use(x)\n", line: 1, from: 4, to: 4, label: "path expected here"},
		{name: "stray colon", src: "a : b\n", line: 1, from: 3, to: 4},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, d := Parse(source.NewFile("t.synod", []byte(tc.src)))
			if d == nil {
				t.Fatal("no diagnostic")
			}
			if d.Line != tc.line || d.Col != tc.from || d.End.Line != tc.line || d.End.Col != tc.to {
				t.Errorf("span %d:%d to %d:%d, want %d:%d to %d:%d (%s)", d.Line, d.Col, d.End.Line, d.End.Col, tc.line, tc.from, tc.line, tc.to, d.Message)
			}
			if d.Label != tc.label {
				t.Errorf("label %q, want %q", d.Label, tc.label)
			}
			switch {
			case tc.also == "" && len(d.Labels) != 0:
				t.Errorf("labels %+v", d.Labels)
			case tc.also != "" && (len(d.Labels) != 1 || d.Labels[0].Text != tc.also || d.Labels[0].Line != tc.line-1):
				t.Errorf("labels %+v, want %q on line %d", d.Labels, tc.also, tc.line-1)
			}
			n := 0
			if len(d.Help) > 0 {
				n = len(d.Help[0].Edits)
			}
			if n != tc.edits {
				t.Errorf("%d edits, want %d", n, tc.edits)
			}
		})
	}
}

// TestListItemFix checks that the edits of "a list item is one item" give a
// list that parses.
func TestListItemFix(t *testing.T) {
	_, d := Parse(source.NewFile("t.synod", []byte("a {1 b, 2}\n")))
	if d == nil || len(d.Help) != 1 || len(d.Help[0].Edits) != 2 {
		t.Fatalf("%+v", d)
	}
	e := d.Help[0].Edits
	if e[0].New != "(" || e[0].Col != 4 || e[1].New != ")" || e[1].Col != 7 {
		t.Errorf("edits %+v", e)
	}
}

// TestHeaderEdits checks the edits of the bad-header diagnostics.
func TestHeaderEdits(t *testing.T) {
	tests := []struct {
		src, folder string
		from, to    int
		edit        source.Edit
		note        bool
	}{
		{"define a\n  b\n", "gossip", 1, 1,
			source.Edit{Line: 1, Col: 1, New: "# package gossip\n"}, false},
		{"# package a b\n", "a", 1, 14,
			source.Edit{Line: 1, Col: 1, End: source.Pos{Line: 1, Col: 14}, New: "# package a"}, false},
		{"\n# package raft\n", "gossip", 1, 15,
			source.Edit{Line: 2, Col: 11, End: source.Pos{Line: 2, Col: 15}, New: "gossip"}, true},
	}
	for _, tc := range tests {
		f, d := Parse(source.NewFile("t.synod", []byte(tc.src)))
		if d != nil {
			t.Fatal(d)
		}
		hd := HeaderDiagnostic(f, tc.folder)
		if hd == nil || len(hd.Help) != 1 || len(hd.Help[0].Edits) != 1 {
			t.Fatalf("%q: %+v", tc.src, hd)
		}
		want := tc.edit
		if want.End == (source.Pos{}) {
			want.End = source.Pos{Line: want.Line, Col: want.Col}
		}
		if e := hd.Help[0].Edits[0]; e != want {
			t.Errorf("%q: edit %+v, want %+v", tc.src, e, want)
		}
		if (len(hd.Notes) > 0) != tc.note {
			t.Errorf("%q: notes %v", tc.src, hd.Notes)
		}
	}
}
