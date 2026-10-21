package render

import (
	"bytes"
	"flag"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/synodlang/compiler/internal/source"
)

var update = flag.Bool("update", false, "rewrite the golden files")

type sp = source.Span

func span(l1, c1, l2, c2 int) sp {
	return sp{Start: source.Pos{Line: l1, Col: c1}, End: source.Pos{Line: l2, Col: c2}}
}

const phraseSrc = "# package p\n\ndefine main\n  when the node starts\n    if n.Role is \"leader\"\n      send hello\n"

// ten builds a file of n numbered lines "line N".
func lines(n int) string {
	var b strings.Builder
	for i := 1; i <= n; i++ {
		b.WriteString("line ")
		b.WriteString(strings.Repeat("x", i%3))
		b.WriteString("\n")
	}
	return b.String()
}

type tcase struct {
	name  string
	files map[string]string
	d     func(f func(string) *source.File) source.Diagnostic
	opts  Options // Source is filled in by the test
	color bool    // also write a color golden
}

func mk(file string, s sp, code source.Code, msg, hint string, more ...source.Extra) func(func(string) *source.File) source.Diagnostic {
	return func(f func(string) *source.File) source.Diagnostic {
		return source.NewSpan(source.NewFile(file, nil), s, code, msg, hint, more...)
	}
}

func cases() []tcase {
	pp := "p/p.synod"
	mapOne := map[string]string{pp: phraseSrc}
	return []tcase{
		{"point", mapOne, mk(pp, source.At(source.Pos{Line: 5, Col: 8}), source.Syntax, "unexpected character", ""), Options{}, false},
		{"span_primary_label", mapOne, mk(pp, span(5, 5, 5, 26), source.UnknownPhrase, `no phrase matches "if n.Role is "leader""`, "",
			source.WithLabel("no phrase takes these items")), Options{}, true},
		{"line_end_and_eof", map[string]string{pp: "abc\ndef"}, mk(pp, source.At(source.Pos{Line: 2, Col: 4}), source.Syntax, "missing newline at end of file", "", source.WithLabel("line ends here")), Options{}, false},
		{"empty_file", map[string]string{pp: ""}, mk(pp, source.At(source.Pos{Line: 1, Col: 1}), source.BadHeader, "missing header", ""), Options{}, false},
		{"two_labels_one_line", mapOne, mk(pp, span(5, 5, 5, 7), source.UnknownPhrase, "no phrase matches", "",
			source.WithLabel("the head"),
			source.Also(pp, span(5, 8, 5, 14), "left operand"),
			source.Also(pp, span(5, 18, 5, 26), "right operand")), Options{}, true},
		{"overlapping_labels", mapOne, mk(pp, span(5, 5, 5, 26), source.UnknownPhrase, `no phrase matches "if n.Role is "leader""`, "",
			source.WithLabel("no phrase takes these items"),
			source.Also(pp, span(5, 8, 5, 26), `3 items here; "if" takes one condition`)), Options{}, false},
		{"labels_one_apart", map[string]string{pp: lines(9)}, mk(pp, span(2, 1, 2, 5), source.Syntax, "boom", "",
			source.WithLabel("here"), source.Also(pp, span(4, 1, 4, 3), "and here")), Options{}, false},
		{"labels_five_apart", map[string]string{pp: lines(9)}, mk(pp, span(2, 1, 2, 5), source.Syntax, "boom", "",
			source.WithLabel("here"), source.Also(pp, span(7, 1, 7, 3), "far here")), Options{}, false},
		{"gutter_two_digits", map[string]string{pp: lines(12)}, mk(pp, span(9, 1, 9, 5), source.Syntax, "boom", "",
			source.WithLabel("here"), source.Also(pp, span(11, 1, 11, 3), "and here")), Options{}, false},
		{"gutter_three_digits", map[string]string{pp: lines(120)}, mk(pp, span(99, 1, 99, 5), source.Syntax, "boom", "",
			source.WithLabel("here"), source.Also(pp, span(100, 1, 100, 3), "and here")), Options{}, false},
		{"cross_file", map[string]string{pp: phraseSrc, "q/q.synod": "# package q\ndefine Foo\n  noop\n"},
			mk(pp, span(5, 8, 5, 14), source.DuplicateName, "already defined", "",
				source.WithLabel("second"),
				source.Also("q/q.synod", span(2, 8, 2, 11), "first defined here"),
				source.Note("names are per package")), Options{}, true},
		{"help_text_and_notes", mapOne, mk(pp, span(5, 8, 5, 14), source.UnknownPhrase, "no phrase", "the hint",
			source.Note("one"), source.Note("two"), source.HelpText("fix it")), Options{}, false},
		{"hint_fallback", mapOne, mk(pp, span(5, 8, 5, 14), source.UnknownPhrase, "no phrase", `did you mean "x"?`), Options{}, false},
		{"edit_insertions", mapOne, mk(pp, span(5, 5, 5, 26), source.UnknownPhrase, `no phrase matches "if n.Role is "leader""`, "",
			source.WithLabel("no phrase takes these items"),
			source.Also(pp, span(5, 8, 5, 26), `3 items here; "if" takes one condition`),
			source.HelpEdit("parenthesize the condition",
				source.EditAt(source.At(source.Pos{Line: 5, Col: 8}), "("),
				source.EditAt(source.At(source.Pos{Line: 5, Col: 26}), ")"))), Options{}, true},
		{"edit_replacement", mapOne, mk(pp, span(5, 8, 5, 14), source.UnknownPhrase, "unknown name", "",
			source.HelpEdit("did you mean \"Rule\"?", source.EditAt(span(5, 8, 5, 14), "Rule"))), Options{}, false},
		{"edit_deletion", mapOne, mk(pp, span(6, 7, 6, 11), source.Syntax, "oops", "",
			source.HelpEdit("remove the word", source.EditAt(span(6, 11, 6, 17), ""))), Options{}, false},
		{"edit_whole_line", mapOne, mk(pp, span(5, 5, 5, 26), source.NonCanonical, "line is not canonical", "",
			source.HelpEdit("write the canonical line", source.EditAt(span(5, 1, 5, 26), "    if (n.Role is \"leader\")"))), Options{}, false},
		{"edit_inserted_line", mapOne, mk(pp, source.At(source.Pos{Line: 3, Col: 1}), source.UnknownImport, "not imported", "",
			source.HelpEdit("add \"use std/lists\"", source.EditAt(source.At(source.Pos{Line: 2, Col: 1}), "use std/lists\n"))), Options{}, false},
		{"edit_delete_line", mapOne, mk(pp, source.At(source.Pos{Line: 2, Col: 1}), source.NonCanonical, "unexpected blank line", "",
			source.HelpEdit("remove it", source.EditAt(span(2, 1, 3, 1), ""))), Options{}, false},
		{"edit_eof_newline", map[string]string{pp: "abc\ndef"}, mk(pp, source.At(source.Pos{Line: 2, Col: 4}), source.NonCanonical, "missing newline at end of file", "",
			source.HelpEdit("end the file with a newline", source.EditAt(source.At(source.Pos{Line: 2, Col: 4}), "\n"))), Options{}, false},
		{"edit_invalid_dropped", mapOne, mk(pp, span(5, 8, 5, 14), source.UnknownPhrase, "unknown name", "",
			source.HelpEdit("past the end", source.EditAt(span(5, 99, 5, 100), "x")),
			source.HelpEdit("overlapping", source.EditAt(span(5, 8, 5, 14), "a"), source.EditAt(span(5, 10, 5, 12), "b")),
			source.HelpEdit("spans lines", source.EditAt(span(5, 8, 6, 2), "a")),
			source.HelpEdit("ok", source.EditAt(span(5, 8, 5, 14), "Rule"))), Options{}, false},
		{"multiline_clamp", mapOne, mk(pp, span(3, 8, 5, 4), source.Syntax, "unclosed", "", source.WithLabel("opens here")), Options{}, false},
		{"tab_line", map[string]string{pp: "a\n\tif x y\n"}, mk(pp, span(2, 5, 2, 8), source.Syntax, "tab", "", source.WithLabel("these")), Options{}, false},
		{"crlf_source", map[string]string{pp: "# package p\r\nfoo bar\r\nbaz\r\n"}, mk(pp, span(2, 5, 2, 8), source.Syntax, "crlf", "", source.WithLabel("bar")), Options{}, false},
		{"bom_line1", map[string]string{pp: "\xef\xbb\xbf# package p\nfoo\n"}, mk(pp, span(1, 3, 1, 10), source.BadHeader, "bad header", "", source.WithLabel("this word")), Options{}, false},
		{"wide_cjk_emoji", map[string]string{pp: "say \"日本語\" 🙂 done\n"}, mk(pp, span(1, 13, 1, 17), source.Syntax, "wide", "", source.WithLabel("done"),
			source.Also(pp, span(1, 6, 1, 9), "cjk")), Options{}, false},
		{"combining_mark", map[string]string{pp: "café au lait\n"}, mk(pp, span(1, 7, 1, 9), source.Syntax, "mark", "", source.WithLabel("au")), Options{}, false},
		{"controls_bidi_sanitized", map[string]string{pp: "a\x01b\x7fc‮d⁦e\u0085f\n"}, mk(pp, span(1, 1, 1, 14), source.Syntax, "ctl", "", source.WithLabel("bad\x1b[31m label‮"),
			source.Note("note \x07 here")), Options{}, false},
		{"invalid_utf8", map[string]string{pp: "ab\xffcd\xc3\n"}, mk(pp, span(1, 4, 1, 6), source.Syntax, "invalid", "", source.WithLabel("after")), Options{}, false},
		{"no_source", nil, mk(pp, source.At(source.Pos{Line: 3, Col: 1}), source.UnknownImport, "cannot read file", "", source.Note("a note"), source.HelpText("do this")), Options{}, false},
		{"line_zero", mapOne, mk(pp, source.At(source.Pos{}), source.MissingMain, "no main", "", source.HelpText("add main")), Options{}, false},
		{"line_past_end", mapOne, mk(pp, source.At(source.Pos{Line: 99, Col: 1}), source.Syntax, "far", "", source.Note("n")), Options{}, false},
		{"header_override", mapOne, mk(pp, span(5, 8, 5, 14), source.DivisionByZero, "division by zero", "", source.WithLabel("here")),
			Options{Header: "failed at 0s: p/p.synod:5:8: error[division-by-zero]: division by zero"}, true},
	}
}

func srcFn(files map[string]string) func(string) *source.File {
	return func(p string) *source.File {
		s, ok := files[p]
		if !ok {
			return nil
		}
		return source.NewFile(p, []byte(s))
	}
}

func golden(t *testing.T, name, got string) {
	t.Helper()
	path := filepath.Join("testdata", name)
	if *update {
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(want) != got {
		t.Errorf("%s differs:\n--- got\n%s--- want\n%s", name, got, want)
	}
}

var ansi = regexp.MustCompile("\x1b\\[[0-9;]*m")

func TestGoldens(t *testing.T) {
	for _, tc := range cases() {
		t.Run(tc.name, func(t *testing.T) {
			d := tc.d(nil)
			o := tc.opts
			if tc.files != nil {
				o.Source = srcFn(tc.files)
			}
			var plain bytes.Buffer
			if err := Write(&plain, d, o); err != nil {
				t.Fatal(err)
			}
			got := plain.String()
			golden(t, tc.name+".txt", got)

			// Invariants of every output.
			if strings.ContainsAny(got, "\r\x1b") || strings.Contains(got, "\xef\xbb\xbf") {
				t.Errorf("output holds CR, ESC or BOM:\n%q", got)
			}
			for i, ln := range strings.Split(strings.TrimSuffix(got, "\n"), "\n") {
				if ln != strings.TrimRight(ln, " ") {
					t.Errorf("line %d has trailing spaces: %q", i+1, ln)
				}
				if ln == "" {
					t.Errorf("line %d is empty", i+1)
				}
			}
			wantHead := tc.opts.Header
			if wantHead == "" {
				wantHead = d.Header()
			}
			if first, _, _ := strings.Cut(got, "\n"); first != wantHead {
				t.Errorf("first line %q, want %q", first, wantHead)
			}

			// Color: stripping the escapes gives the plain output.
			o.Color = true
			var col bytes.Buffer
			if err := Write(&col, d, o); err != nil {
				t.Fatal(err)
			}
			if s := ansi.ReplaceAllString(col.String(), ""); s != got {
				t.Errorf("stripped color output differs:\n%s\nvs\n%s", s, got)
			}
			if strings.Contains(col.String(), "\x1b[") == false {
				t.Errorf("color output has no escapes")
			}
			if tc.color {
				golden(t, tc.name+".color.txt", col.String())
			}
		})
	}
}

func TestAll(t *testing.T) {
	files := map[string]string{"p/p.synod": phraseSrc}
	f := source.NewFile("p/p.synod", nil)
	ds := []source.Diagnostic{
		source.NewSpan(f, span(5, 5, 5, 7), source.Syntax, "one", "", source.WithLabel("a")),
		source.NewSpan(f, span(3, 1, 3, 7), source.Syntax, "two", ""),
	}
	var b bytes.Buffer
	if err := All(&b, ds, Options{Source: srcFn(files)}); err != nil {
		t.Fatal(err)
	}
	golden(t, "all.txt", b.String())
	if blocks := strings.Split(strings.TrimSuffix(b.String(), "\n"), "\n\n"); len(blocks) != 2 {
		t.Errorf("%d blocks, want 2", len(blocks))
	}
}

func TestShortFormUnchanged(t *testing.T) {
	// Nothing in the rendering path may change String().
	d := source.NewSpan(source.NewFile("a", nil), span(1, 1, 1, 3), source.Syntax, "m", "h", source.WithLabel("l"), source.HelpText("x"))
	if got, want := d.String(), "a:1:1: error[syntax]: m\n  hint: h"; got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
}
