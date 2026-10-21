package driver

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/synodlang/compiler/internal/format"
	"github.com/synodlang/compiler/internal/source"
)

// goldenCase is one case of testdata/check, checked.
type goldenCase struct {
	name string
	dir  string
	res  Result
}

// goldenCases checks every case of testdata/check.
func goldenCases(t *testing.T) []goldenCase {
	t.Helper()
	root := "../../testdata/check"
	var out []goldenCase
	codes, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range codes {
		if !c.IsDir() {
			continue
		}
		cases, err := os.ReadDir(filepath.Join(root, c.Name()))
		if err != nil {
			t.Fatal(err)
		}
		for _, cs := range cases {
			if cs.IsDir() {
				dir := filepath.Join(root, c.Name(), cs.Name())
				out = append(out, goldenCase{c.Name() + "/" + cs.Name(), dir, checkDir(t, dir, false)})
			}
		}
	}
	return out
}

// rawLine returns line n of f as written: no "\n", no BOM, a "\r" kept.
func rawLine(f *source.File, n int) (string, bool) {
	src := f.Src
	if bytes.HasPrefix(src, []byte("\xef\xbb\xbf")) {
		src = src[3:]
	}
	lines := strings.Split(string(src), "\n")
	if n < 1 || n > len(lines) {
		return "", false
	}
	return lines[n-1], true
}

// spanProblem returns what is wrong with sp in f: it must lie on one line
// and within it (a caret one past the end is fine).
func spanProblem(f *source.File, sp source.Span) string {
	if f == nil {
		return "no source"
	}
	line, ok := rawLine(f, sp.Start.Line)
	if !ok {
		return "line out of range"
	}
	if sp.End.Line != sp.Start.Line {
		return "span covers several lines"
	}
	if sp.Start.Col < 1 || sp.End.Col < sp.Start.Col {
		return "bad columns"
	}
	if sp.End.Col > utf8.RuneCountInString(line)+1 {
		return "span ends past the line"
	}
	return ""
}

// emptySpanOK lists the messages whose primary span may be empty: the thing
// they report is something that is not there, or a blank line.
func emptySpanOK(d source.Diagnostic) bool {
	for _, m := range []string{
		"byte order mark at start of file",
		"missing newline at end of file",
		"missing import path after use",
		`missing "# package" header`,
		"missing blank line",
		"unexpected blank line",
		`missing "module" directive`,
		`missing "synod" directive`,
	} {
		if d.Message == m {
			return true
		}
	}
	return false
}

// notFrontEnd reports the diagnostics of the codes of this test that other
// stages produce: the resolver and the checker give them spans in later
// changes.
func notFrontEnd(d source.Diagnostic) bool {
	switch d.Code {
	case source.Syntax:
		return strings.HasPrefix(d.Message, "duration ") || strings.HasPrefix(d.Message, "number out of")
	case source.NonCanonical:
		return strings.Contains(d.Message, "must start lower-case")
	case source.ExtraMain:
		return !strings.HasPrefix(d.Message, "a second main file")
	}
	return false
}

// frontEndCodes are the codes that the lexer, the parser, the formatter and
// the loader produce.
var frontEndCodes = map[source.Code]bool{
	source.Syntax: true, source.Indent: true, source.NonCanonical: true,
	source.BadHeader: true, source.UnknownImport: true, source.ImportCycle: true,
	source.ExtraMain: true,
}

// TestSpans checks the spans, labels and edits of the diagnostics of every
// golden case: primary spans of the front end are non-empty (with a short
// allowlist), and every span, label and edit lies within its file; the edits
// of one help do not overlap.
func TestSpans(t *testing.T) {
	n := 0
	for _, c := range goldenCases(t) {
		for _, d := range c.res.Diags {
			f := c.res.Sources[d.File]
			sp := source.Span{Start: source.Pos{Line: d.Line, Col: d.Col}, End: d.End}
			where := c.name + ": " + d.Header()
			if f == nil {
				continue
			}
			if p := spanProblem(f, sp); p != "" {
				t.Errorf("%s: primary span %v: %s", where, sp, p)
			}
			if frontEndCodes[d.Code] && !notFrontEnd(d) {
				n++
				if sp.End == sp.Start && !emptySpanOK(d) {
					t.Errorf("%s: empty primary span", where)
				}
			}
			for _, l := range d.Labels {
				lsp := source.Span{Start: source.Pos{Line: l.Line, Col: l.Col}, End: l.End}
				if p := spanProblem(c.res.Sources[l.File], lsp); p != "" {
					t.Errorf("%s: label %q at %s %v: %s", where, l.Text, l.File, lsp, p)
				}
				if l.End == lsp.Start {
					t.Errorf("%s: empty label %q", where, l.Text)
				}
			}
			for _, h := range d.Help {
				checkEdits(t, where, f, h.Edits)
			}
		}
	}
	if n < 20 {
		t.Errorf("only %d front-end diagnostics checked", n)
	}
}

// checkEdits checks that the edits lie in f and do not overlap.
func checkEdits(t *testing.T, where string, f *source.File, edits []source.Edit) {
	t.Helper()
	type rng struct{ from, to source.Pos }
	var rs []rng
	for _, e := range edits {
		from, to := source.Pos{Line: e.Line, Col: e.Col}, e.End
		if to == (source.Pos{}) {
			to = from
		}
		if _, ok := rawLine(f, from.Line); !ok {
			t.Errorf("%s: edit %+v: line out of range", where, e)
			continue
		}
		deleteLine := to.Line == from.Line+1 && to.Col == 1 && from.Col == 1 && e.New == ""
		if !deleteLine {
			if p := spanProblem(f, source.Span{Start: from, End: to}); p != "" {
				t.Errorf("%s: edit %+v: %s", where, e, p)
			}
		}
		rs = append(rs, rng{from, to})
	}
	less := func(a, b source.Pos) bool { return a.Line < b.Line || a.Line == b.Line && a.Col < b.Col }
	sort.Slice(rs, func(i, j int) bool { return less(rs[i].from, rs[j].from) })
	for i := 1; i < len(rs); i++ {
		if less(rs[i].from, rs[i-1].to) {
			t.Errorf("%s: edits overlap: %v and %v", where, rs[i-1], rs[i])
		}
	}
}

// applyEdits applies edits (of one help) to src.
func applyEdits(src []byte, edits []source.Edit) []byte {
	starts := []int{0}
	for i, b := range src {
		if b == '\n' {
			starts = append(starts, i+1)
		}
	}
	off := func(p source.Pos) int {
		o := starts[p.Line-1]
		if p.Line == 1 && bytes.HasPrefix(src, []byte("\xef\xbb\xbf")) {
			o += 3
		}
		for c := 1; c < p.Col; c++ {
			_, n := utf8.DecodeRune(src[o:])
			o += n
		}
		return o
	}
	type rep struct {
		from, to int
		new      string
	}
	var rs []rep
	for _, e := range edits {
		from := off(source.Pos{Line: e.Line, Col: e.Col})
		to := from
		if e.End != (source.Pos{}) {
			to = off(e.End)
		}
		rs = append(rs, rep{from, to, e.New})
	}
	sort.SliceStable(rs, func(i, j int) bool { return rs[i].from > rs[j].from })
	out := append([]byte(nil), src...)
	for _, r := range rs {
		out = append(out[:r.from], append([]byte(r.new), out[r.to:]...)...)
	}
	return out
}

// copyDir copies the tree at from to to.
func copyDir(t *testing.T, from, to string) {
	t.Helper()
	err := filepath.WalkDir(from, func(p string, e fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(from, p)
		if e.IsDir() {
			return os.MkdirAll(filepath.Join(to, rel), 0o755)
		}
		if strings.HasPrefix(e.Name(), "expect") {
			return nil
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(to, rel), b, 0o644)
	})
	if err != nil {
		t.Fatal(err)
	}
}

// TestFixesApply applies the help edits of non-canonical, bad-header and
// unknown-import diagnostics to a copy of their case and checks again: the
// diagnostic of that code on that line is gone. The edit of "line is not
// canonical" gives the line that the formatter prints.
func TestFixesApply(t *testing.T) {
	fixable := map[source.Code]bool{source.NonCanonical: true, source.BadHeader: true, source.UnknownImport: true}
	applied := map[source.Code]int{}
	for _, c := range goldenCases(t) {
		for _, d := range c.res.Diags {
			if !fixable[d.Code] || notFrontEnd(d) {
				continue
			}
			for hi, h := range d.Help {
				if len(h.Edits) == 0 {
					continue
				}
				f := c.res.Sources[d.File]
				fixed := applyEdits(f.Src, h.Edits)
				tmp := t.TempDir()
				copyDir(t, c.dir, tmp)
				if err := os.WriteFile(filepath.Join(tmp, filepath.FromSlash(d.File)), fixed, 0o644); err != nil {
					t.Fatal(err)
				}
				again := checkDir(t, tmp, false)
				for _, d2 := range again.Diags {
					if d2.File == d.File && d2.Code == d.Code && d2.Line == d.Line {
						t.Errorf("%s: help %d of %s: still reported: %s", c.name, hi, d.Header(), d2.Header())
					}
				}
				applied[d.Code]++
				if d.Code == source.NonCanonical && d.Message == "line is not canonical" {
					want, fd := format.FormatWith(f, c.res.Fixes[d.File])
					if fd != nil {
						t.Fatal(fd)
					}
					wl := strings.Split(string(want), "\n")[d.Line-1]
					gl := strings.Split(string(fixed), "\n")[d.Line-1]
					if wl != gl {
						t.Errorf("%s: edit gives %q, the formatter prints %q", c.name, gl, wl)
					}
				}
			}
		}
	}
	for code := range fixable {
		if applied[code] == 0 {
			t.Errorf("no fix of %s was applied", code)
		}
	}
}
