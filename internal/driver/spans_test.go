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
	"github.com/synodlang/compiler/internal/load"
	"github.com/synodlang/compiler/internal/resolve"
	"github.com/synodlang/compiler/internal/source"
	"github.com/synodlang/compiler/internal/stdlib"
)

// goldenCase is one case of testdata/check, checked.
type goldenCase struct {
	name string
	dir  string
	res  Result
	// pre are the diagnostics of the stages before the checker: the loader,
	// the resolver and the canonical-form check.
	pre []source.Diagnostic
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
				out = append(out, goldenCase{c.Name() + "/" + cs.Name(), dir, checkDir(t, dir, false), preCheck(t, dir)})
			}
		}
	}
	return out
}

// preCheck runs the stages before the checker on the packages of dir and
// returns their diagnostics, sorted and free of duplicates.
func preCheck(t *testing.T, dir string) []source.Diagnostic {
	t.Helper()
	abs, err := filepath.Abs(dir)
	if err != nil {
		t.Fatal(err)
	}
	abs = slash(abs)
	fsys := load.OSFS{}
	targets, err := load.Discover(fsys, abs)
	if err != nil {
		t.Fatal(err)
	}
	cfg := load.Config{FS: fsys, Std: stdlib.Root(), Display: func(p string) string {
		if len(p) > len(abs) && p[:len(abs)] == abs {
			return p[len(abs)+1:]
		}
		return p
	}}
	g, diags := load.Load(cfg, targets)
	byPath := map[string]*resolve.Package{}
	for _, lp := range g.Packages {
		if lp.Broken {
			continue
		}
		deps := map[string]*resolve.Package{}
		for _, d := range lp.Deps {
			if rp := byPath[d.Path]; rp != nil {
				deps[d.Path] = rp
			}
		}
		rp, ds := resolve.Resolve(lp, deps)
		diags = append(diags, ds...)
		byPath[lp.Path] = rp
		for _, f := range rp.Files {
			if d := format.CheckWith(f.Load.Src, rp.Fixes[f.Load]); d != nil {
				diags = append(diags, *d)
			}
		}
	}
	return tidy(diags)
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

// TestSpans checks the spans, labels and edits of the diagnostics of every
// golden case: the primary spans of the stages before the checker (loader,
// parser, resolver, canonical form) are non-empty, with a short allowlist,
// and every span, label and edit lies within its file; the edits of one help
// do not overlap.
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
		for _, d := range c.pre {
			if c.res.Sources[d.File] == nil {
				continue
			}
			n++
			if d.End == (source.Pos{Line: d.Line, Col: d.Col}) && !emptySpanOK(d) {
				t.Errorf("%s: %s: empty primary span", c.name, d.Header())
			}
		}
	}
	if n < 100 {
		t.Errorf("only %d diagnostics of the stages before the checker were checked", n)
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

// TestFixesApply applies the help edits of the diagnostics of the stages
// before the checker to a copy of their case and checks again: the
// diagnostic is gone (same code and line; same code and message when the
// edit moves lines). The edit of "line is not canonical" gives the line that
// the formatter prints.
func TestFixesApply(t *testing.T) {
	applied := map[string]int{}
	for _, c := range goldenCases(t) {
		for _, d := range c.pre {
			f := c.res.Sources[d.File]
			if f == nil {
				continue
			}
			for hi, h := range d.Help {
				if len(h.Edits) == 0 {
					continue
				}
				fixed := applyEdits(f.Src, h.Edits)
				tmp := t.TempDir()
				copyDir(t, c.dir, tmp)
				if err := os.WriteFile(filepath.Join(tmp, filepath.FromSlash(d.File)), fixed, 0o644); err != nil {
					t.Fatal(err)
				}
				moves := false
				for _, e := range h.Edits {
					if strings.Contains(e.New, "\n") || e.End.Line > e.Line {
						moves = true
					}
				}
				again := checkDir(t, tmp, false)
				for _, d2 := range again.Diags {
					if d2.File != d.File || d2.Code != d.Code {
						continue
					}
					if moves && d2.Message == d.Message || !moves && d2.Line == d.Line {
						t.Errorf("%s: help %d of %s: still reported: %s", c.name, hi, d.Header(), d2.Header())
					}
				}
				applied[string(d.Code)+": "+kindOfFix(d, h)]++
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
	for _, want := range []string{
		"non-canonical: line", "bad-header: ", "unknown-import: ",
		"unknown-phrase: parentheses", "unneeded-alias: ", "unused-import: ", "duplicate-name: import",
		"non-canonical: lower-case", "bad-receiver: ", "unknown-phrase: use line",
	} {
		if applied[want] == 0 {
			t.Errorf("no fix of kind %q was applied; applied: %v", want, applied)
		}
	}
}

// kindOfFix names the kind of fix of a help, for the coverage check of
// TestFixesApply.
func kindOfFix(d source.Diagnostic, h source.Help) string {
	switch {
	case d.Code == source.NonCanonical && strings.Contains(d.Message, "must start lower-case"):
		return "lower-case"
	case d.Code == source.NonCanonical && d.Message == "line is not canonical":
		return "line"
	case d.Code == source.UnknownPhrase && strings.HasPrefix(h.Text, "parenthesize"):
		return "parentheses"
	case d.Code == source.UnknownPhrase && strings.HasPrefix(h.Text, "add \"use"):
		return "use line"
	case d.Code == source.DuplicateName && strings.Contains(d.Message, "already imported"):
		return "import"
	}
	return ""
}
