package source

import (
	"bytes"
	"testing"
)

func TestPosOf(t *testing.T) {
	tests := []struct {
		name string
		src  string
		off  int
		want Pos
	}{
		{"start", "ab\ncd\n", 0, Pos{1, 1}},
		{"mid line", "ab\ncd\n", 1, Pos{1, 2}},
		{"newline itself", "ab\ncd\n", 2, Pos{1, 3}},
		{"second line", "ab\ncd\n", 3, Pos{2, 1}},
		{"end after final newline", "ab\ncd\n", 6, Pos{3, 1}},
		{"clamped", "ab", 99, Pos{1, 3}},
		{"negative", "ab", -4, Pos{1, 1}},
		{"code points not bytes", "é\"x\"", 3, Pos{1, 3}},
		{"bom skipped", "\xef\xbb\xbfab", 3, Pos{1, 1}},
		{"bom inside", "\xef\xbb\xbfab", 1, Pos{1, 1}},
		{"bom second line unaffected", "\xef\xbb\xbfa\nb", 5, Pos{2, 1}},
		{"empty file", "", 0, Pos{1, 1}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := NewFile("x.synod", []byte(tc.src))
			if got := f.PosOf(tc.off); got != tc.want {
				t.Errorf("PosOf(%d) = %v, want %v", tc.off, got, tc.want)
			}
		})
	}
}

func TestLineCount(t *testing.T) {
	for src, want := range map[string]int{"": 1, "a": 1, "a\n": 2, "a\nb": 2} {
		if got := NewFile("x", []byte(src)).LineCount(); got != want {
			t.Errorf("LineCount(%q) = %d, want %d", src, got, want)
		}
	}
}

func TestDiagnosticString(t *testing.T) {
	f := NewFile("gossip/gossip.synod", nil)
	tests := []struct {
		name string
		d    Diagnostic
		want string
	}{
		{
			"no hint",
			New(f, Pos{14, 3}, Syntax, "unexpected ','", ""),
			"gossip/gossip.synod:14:3: error[syntax]: unexpected ','",
		},
		{
			"hint",
			New(f, Pos{1, 1}, BadHeader, `missing "# package" header`, `the first line is "# package <folder>"`),
			"gossip/gossip.synod:1:1: error[bad-header]: missing \"# package\" header\n  hint: the first line is \"# package <folder>\"",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.d.String(); got != tc.want {
				t.Errorf("String() =\n%q\nwant\n%q", got, tc.want)
			}
		})
	}
}

func TestSort(t *testing.T) {
	ds := []Diagnostic{
		{File: "b", Line: 1, Col: 1, Code: Syntax},
		{File: "a", Line: 2, Col: 1, Code: Syntax},
		{File: "a", Line: 1, Col: 9, Code: Syntax},
		{File: "a", Line: 1, Col: 2, Code: NonCanonical},
		{File: "a", Line: 1, Col: 2, Code: BadHeader},
	}
	Sort(ds)
	want := []struct {
		file      string
		line, col int
		code      Code
	}{
		{"a", 1, 2, BadHeader}, {"a", 1, 2, NonCanonical}, {"a", 1, 9, Syntax},
		{"a", 2, 1, Syntax}, {"b", 1, 1, Syntax},
	}
	for i, w := range want {
		g := ds[i]
		if g.File != w.file || g.Line != w.line || g.Col != w.col || g.Code != w.code {
			t.Errorf("ds[%d] = %+v, want %+v", i, g, w)
		}
	}
}

func TestWriteJSON(t *testing.T) {
	f := NewFile("gossip/gossip.synod", nil)
	tests := []struct {
		name string
		ds   []Diagnostic
		want string
	}{
		{"nil", nil, `{"format":"synod-check/1","ok":true,"diagnostics":[]}` + "\n"},
		{"empty", []Diagnostic{}, `{"format":"synod-check/1","ok":true,"diagnostics":[]}` + "\n"},
		{
			"with hint and no html escaping",
			[]Diagnostic{New(f, Pos{14, 3}, UnknownPhrase, `no phrase matches "a<b>&c"`, `did you mean "x"?`)},
			`{"format":"synod-check/1","ok":false,"diagnostics":[{"file":"gossip/gossip.synod","line":14,"col":3,"end":{"line":14,"col":3},"severity":"error","code":"unknown-phrase","message":"no phrase matches \"a<b>&c\"","hint":"did you mean \"x\"?"}]}` + "\n",
		},
		{
			"without hint omits key",
			[]Diagnostic{New(f, Pos{1, 1}, Indent, "tabs are not allowed", "")},
			`{"format":"synod-check/1","ok":false,"diagnostics":[{"file":"gossip/gossip.synod","line":1,"col":1,"end":{"line":1,"col":1},"severity":"error","code":"indent","message":"tabs are not allowed"}]}` + "\n",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			if err := WriteJSON(&buf, tc.ds); err != nil {
				t.Fatal(err)
			}
			if got := buf.String(); got != tc.want {
				t.Errorf("WriteJSON =\n%s\nwant\n%s", got, tc.want)
			}
		})
	}
}

func TestFileLine(t *testing.T) {
	f := NewFile("x", []byte("\xef\xbb\xbfab\r\ncd\n\nlast"))
	for n, want := range map[int]string{1: "ab", 2: "cd", 3: "", 4: "last", 0: "", 5: "", -1: ""} {
		if got := f.Line(n); got != want {
			t.Errorf("Line(%d) = %q, want %q", n, got, want)
		}
	}
	if got := NewFile("x", []byte("a\n")).Line(2); got != "" {
		t.Errorf("Line after final newline = %q", got)
	}
	if got := NewFile("x", nil).Line(1); got != "" {
		t.Errorf("Line of empty file = %q", got)
	}
}

func TestNewSpanAndExtras(t *testing.T) {
	f := NewFile("p/p.synod", nil)
	sp := Span{Pos{5, 5}, Pos{5, 26}}
	d := NewSpan(f, sp, UnknownPhrase, "msg", "the hint",
		WithLabel("primary"),
		Also("p/p.synod", Span{Pos{5, 8}, Pos{5, 26}}, "secondary"),
		HelpText("advice"),
		HelpEdit("fix", EditAt(At(Pos{5, 8}), "(")),
		Note("fact"))
	if d.Line != 5 || d.Col != 5 || d.End != (Pos{5, 26}) || d.Label != "primary" ||
		len(d.Labels) != 1 || len(d.Help) != 2 || len(d.Notes) != 1 {
		t.Errorf("NewSpan = %+v", d)
	}
	if got, want := d.String(), "p/p.synod:5:5: error[unknown-phrase]: msg\n  hint: the hint"; got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
	if got, want := d.Header(), "p/p.synod:5:5: error[unknown-phrase]: msg"; got != want {
		t.Errorf("Header() = %q, want %q", got, want)
	}
	var buf bytes.Buffer
	if err := WriteJSON(&buf, []Diagnostic{d}); err != nil {
		t.Fatal(err)
	}
	want := `{"format":"synod-check/1","ok":false,"diagnostics":[{"file":"p/p.synod","line":5,"col":5,"end":{"line":5,"col":26},"severity":"error","code":"unknown-phrase","message":"msg","hint":"the hint","label":"primary","labels":[{"file":"p/p.synod","line":5,"col":8,"end":{"line":5,"col":26},"text":"secondary"}],"help":[{"text":"advice"},{"text":"fix","edits":[{"line":5,"col":8,"end":{"line":5,"col":8},"new":"("}]}],"notes":["fact"]}]}` + "\n"
	if buf.String() != want {
		t.Errorf("WriteJSON =\n%s\nwant\n%s", buf.String(), want)
	}
}

func TestKey(t *testing.T) {
	f := NewFile("a", nil)
	a := New(f, Pos{1, 2}, Syntax, "m", "h1")
	b := NewSpan(f, Span{Pos{1, 2}, Pos{1, 4}}, Syntax, "m", "h2", Note("n"))
	if a.Key() != b.Key() {
		t.Errorf("keys differ for the same position, code and message")
	}
	if c := New(f, Pos{1, 3}, Syntax, "m", ""); a.Key() == c.Key() {
		t.Errorf("keys equal for different columns")
	}
}

func TestHeaderWithoutPosition(t *testing.T) {
	d := Diagnostic{File: "p", Severity: "error", Code: MissingMain, Message: "no main file in p"}
	if got, want := d.Header(), "p: error[missing-main]: no main file in p"; got != want {
		t.Errorf("Header() = %q, want %q", got, want)
	}
}
