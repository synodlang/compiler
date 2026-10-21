package format

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/synodlang/compiler/internal/lex"
	"github.com/synodlang/compiler/internal/parse"
	"github.com/synodlang/compiler/internal/source"
)

// A textMutator damages the layout of canonical source text.
type textMutator struct {
	name string
	fn   func(src []byte, toks []lex.Token) []byte
}

// A treeMutator damages the content of a parsed canonical tree; the tree is
// then printed with emit alone, which does not repair it.
type treeMutator struct {
	name string
	fn   func(t *parse.File)
}

// structural reports a token kind that is not text on a line.
func structural(k lex.Kind) bool {
	switch k {
	case lex.NEWLINE, lex.INDENT, lex.DEDENT, lex.EOF:
		return true
	}
	return false
}

// rewriteGaps copies src, replacing the gap between two tokens of one line
// with whatever gap returns (it returns its input to keep the gap).
func rewriteGaps(src []byte, toks []lex.Token, gap func(prev, next lex.Token, gap string) string) []byte {
	var out bytes.Buffer
	copied := 0
	var prev lex.Token
	have := false
	for _, t := range toks {
		if structural(t.Kind) {
			continue
		}
		if have && prev.Pos.Line == t.Pos.Line {
			start, end := prev.Off+len(prev.Text), t.Off
			if start <= end {
				g := string(src[start:end])
				if ng := gap(prev, t, g); ng != g {
					out.Write(src[copied:start])
					out.WriteString(ng)
					copied = end
				}
			}
		}
		prev, have = t, true
	}
	out.Write(src[copied:])
	return out.Bytes()
}

func isOpen(k lex.Kind) bool { return k == lex.LPAREN || k == lex.LBRACE || k == lex.LBRACKET }
func isClose(k lex.Kind) bool {
	return k == lex.RPAREN || k == lex.RBRACE || k == lex.RBRACKET
}

// lines splits canonical text into lines without their newlines.
func lines(src []byte) []string {
	return strings.Split(strings.TrimSuffix(string(src), "\n"), "\n")
}

func join(ls []string, sep string) []byte {
	return []byte(strings.Join(ls, sep) + sep)
}

func indented(l string) bool { return strings.HasPrefix(l, " ") }

func isComment(l string) bool { return strings.HasPrefix(strings.TrimLeft(l, " "), "#") }

var textMutators = []textMutator{
	{"double spaces", func(src []byte, toks []lex.Token) []byte {
		return rewriteGaps(src, toks, func(_, _ lex.Token, g string) string {
			if g == " " {
				return "  "
			}
			return g
		})
	}},
	{"spaces inside brackets and before commas", func(src []byte, toks []lex.Token) []byte {
		return rewriteGaps(src, toks, func(p, n lex.Token, g string) string {
			if g == "" && (isOpen(p.Kind) || isClose(n.Kind) || n.Kind == lex.COMMA || n.Kind == lex.COLON) {
				return " "
			}
			return g
		})
	}},
	{"trailing spaces", func(src []byte, _ []lex.Token) []byte {
		ls := lines(src)
		for i, l := range ls {
			if l != "" {
				if isComment(l) {
					ls[i] = l + " \t "
				} else {
					ls[i] = l + "   "
				}
			}
		}
		return join(ls, "\n")
	}},
	{"crlf", func(src []byte, _ []lex.Token) []byte {
		return bytes.ReplaceAll(src, []byte("\n"), []byte("\r\n"))
	}},
	{"no final newline", func(src []byte, _ []lex.Token) []byte {
		return bytes.TrimSuffix(src, []byte("\n"))
	}},
	{"byte order mark", func(src []byte, _ []lex.Token) []byte {
		return append([]byte("\xef\xbb\xbf"), src...)
	}},
	{"blank line after every line in blocks", func(src []byte, _ []lex.Token) []byte {
		ls := lines(src)
		var out []string
		for i, l := range ls {
			out = append(out, l)
			if i+1 < len(ls) && (indented(l) || indented(ls[i+1])) {
				out = append(out, "")
			}
		}
		return join(out, "\n")
	}},
	{"two blank lines between declarations", func(src []byte, _ []lex.Token) []byte {
		var out []string
		for _, l := range lines(src) {
			out = append(out, l)
			if l == "" {
				out = append(out, "")
			}
		}
		return join(out, "\n")
	}},
	{"no blank lines between declarations", func(src []byte, _ []lex.Token) []byte {
		ls := lines(src)
		var out []string
		for i, l := range ls {
			if l == "" && i > 0 && i+1 < len(ls) {
				prev, next := ls[i-1], ls[i+1]
				if prev != "" && !isComment(prev) && next != "" && !indented(next) && !isComment(next) {
					continue
				}
			}
			out = append(out, l)
		}
		return join(out, "\n")
	}},
	{"blank lines after header and between imports", func(src []byte, _ []lex.Token) []byte {
		var out []string
		for i, l := range lines(src) {
			out = append(out, l)
			if (i == 0 && strings.HasPrefix(l, "# package ")) || strings.HasPrefix(l, "use ") {
				out = append(out, "")
			}
		}
		return join(out, "\n")
	}},
}

// --- tree mutators -------------------------------------------------------

// eachSentence calls fn for every sentence of the tree, inner sentences
// before the sentences that hold them.
func eachSentence(t *parse.File, fn func(s *parse.Sentence, head bool)) {
	var line func(l *parse.Line)
	var block func(b *parse.Block)
	var sent func(s *parse.Sentence, head bool)
	var item func(it parse.Item)
	item = func(it parse.Item) {
		switch v := it.(type) {
		case *parse.Paren:
			sent(v.S, false)
		case *parse.List:
			for _, e := range v.Items {
				item(e)
			}
		}
	}
	sent = func(s *parse.Sentence, head bool) {
		for _, g := range s.Groups {
			for _, it := range g {
				item(it)
			}
		}
		fn(s, head)
	}
	block = func(b *parse.Block) {
		if b != nil {
			for _, l := range b.Lines {
				line(l)
			}
		}
	}
	line = func(l *parse.Line) {
		sent(l.Sentence, true)
		block(l.Block)
		for _, c := range l.Conts {
			block(c.Block)
		}
	}
	for _, n := range t.Body {
		if l, ok := n.(*parse.Line); ok {
			line(l)
		}
	}
}

func wrappable(it parse.Item) bool {
	switch it.(type) {
	case *parse.Word, *parse.Member, *parse.Number, *parse.Text, *parse.Duration, *parse.Color:
		return true
	}
	return false
}

func wrap(it parse.Item, n int) parse.Item {
	for ; n > 0; n-- {
		it = &parse.Paren{S: &parse.Sentence{Groups: [][]parse.Item{{it}}}}
	}
	return it
}

// wrapAll wraps every simple item except the first of a line in n pairs of
// parentheses, in sentences and in lists.
func wrapAll(t *parse.File, n int) {
	eachSentence(t, func(s *parse.Sentence, head bool) {
		for gi, g := range s.Groups {
			for ii, it := range g {
				if l, ok := it.(*parse.List); ok {
					for k, e := range l.Items {
						if wrappable(e) {
							l.Items[k] = wrap(e, n)
						}
					}
				}
				// The comma rule recognizes "each k" and "message T with"
				// by their bare words.
				keyword := len(s.Groups) > 1 && gi == 0 && ii < 3
				if (head && gi == 0 && ii == 0) || keyword || !wrappable(it) {
					continue
				}
				g[ii] = wrap(it, n)
			}
		}
	})
}

// withZero appends a trailing zero to a number: 2 becomes 2.0, 0.9 0.90.
func withZero(s string) string {
	if strings.Contains(s, ".") {
		return s + "0"
	}
	return s + ".0"
}

// msToSeconds writes a whole number of milliseconds as decimal seconds.
func msToSeconds(ms string) string {
	neg := strings.HasPrefix(ms, "-")
	ms = strings.TrimPrefix(ms, "-")
	for len(ms) < 4 {
		ms = "0" + ms
	}
	i, f := ms[:len(ms)-3], strings.TrimRight(ms[len(ms)-3:], "0")
	s := i
	if f != "" {
		s += "." + f
	}
	if neg {
		s = "-" + s
	}
	return s + "s"
}

func respell(t *parse.File, fn func(it parse.Item) parse.Item) {
	eachSentence(t, func(s *parse.Sentence, _ bool) {
		for _, g := range s.Groups {
			for ii, it := range g {
				if l, ok := it.(*parse.List); ok {
					for k, e := range l.Items {
						l.Items[k] = fn(e)
					}
				}
				g[ii] = fn(it)
			}
		}
	})
}

func respellNumbers(it parse.Item) parse.Item {
	if n, ok := it.(*parse.Number); ok {
		n.Text = withZero(n.Text)
	}
	return it
}

func respellDurations(it parse.Item) parse.Item {
	d, ok := it.(*parse.Duration)
	if !ok {
		return it
	}
	switch {
	case strings.HasSuffix(d.Text, "ms") && !strings.Contains(d.Text, "."):
		d.Text = msToSeconds(strings.TrimSuffix(d.Text, "ms"))
	case strings.HasSuffix(d.Text, "s") && !strings.HasSuffix(d.Text, "ms") && !strings.Contains(d.Text, "."):
		d.Text = strings.TrimSuffix(d.Text, "s") + "000ms"
	}
	return it
}

func respellColors(it parse.Item) parse.Item {
	if c, ok := it.(*parse.Color); ok {
		c.Text = strings.ToLower(c.Text)
	}
	return it
}

// negateComparisons rewrites every "(A is not B)" style parenthesis into
// "(not (A is B))".
func negateComparisons(t *parse.File) {
	eachSentence(t, func(s *parse.Sentence, _ bool) {
		for _, g := range s.Groups {
			for ii, it := range g {
				p, ok := it.(*parse.Paren)
				if !ok || len(p.S.Groups) != 1 {
					continue
				}
				c, ok := findComparison(p.S.Groups[0])
				if !ok {
					continue
				}
				op := make([]string, c.n)
				for k := range op {
					op[k] = p.S.Groups[0][c.at+k].(*parse.Word).Name
				}
				switch strings.Join(op, " ") {
				case "is not", "is not in", "at least", "at most":
				default:
					continue
				}
				inner := &parse.Paren{S: &parse.Sentence{Groups: [][]parse.Item{replaceOp(p.S.Groups[0], c)}}}
				g[ii] = &parse.Paren{S: &parse.Sentence{Groups: [][]parse.Item{{&parse.Word{Name: "not"}, inner}}}}
			}
		}
	})
}

func reverseImports(t *parse.File) {
	n := 0
	for n < len(t.Body) {
		if _, ok := isUse(t.Body[n]); !ok {
			break
		}
		n++
	}
	for i, j := 0, n-1; i < j; i, j = i+1, j-1 {
		t.Body[i], t.Body[j] = t.Body[j], t.Body[i]
	}
}

var treeMutators = []treeMutator{
	{"parentheses around simple items", func(t *parse.File) { wrapAll(t, 1) }},
	{"double parentheses around simple items", func(t *parse.File) { wrapAll(t, 2) }},
	{"numbers with trailing zeros", func(t *parse.File) { respell(t, respellNumbers) }},
	{"durations in another unit", func(t *parse.File) { respell(t, respellDurations) }},
	{"negated comparisons", negateComparisons},
	{"reversed imports", reverseImports},
	{"all of them", func(t *parse.File) {
		wrapAll(t, 2)
		respell(t, func(it parse.Item) parse.Item {
			return respellColors(respellDurations(respellNumbers(it)))
		})
		negateComparisons(t)
		reverseImports(t)
	}},
}

// checkMutant requires that mut formats back to orig and is not canonical.
func checkMutant(t *testing.T, path string, orig, mut []byte) {
	t.Helper()
	f := source.NewFile(path, mut)
	out, d := Format(f)
	if d != nil {
		t.Errorf("%s: mutant does not parse: %s\n%s", path, d, mut)
		return
	}
	if !bytes.Equal(out, orig) {
		t.Errorf("%s: mutant formats to\n%s\nwant\n%s\nmutant\n%s", path, out, orig, mut)
	}
	if Check(f) == nil {
		t.Errorf("%s: Check accepts the mutant\n%s", path, mut)
	}
}

func TestTextMutations(t *testing.T) {
	for _, m := range textMutators {
		t.Run(m.name, func(t *testing.T) {
			changed := 0
			walkSynod(t, filepath.Join(testdata, "examples"), func(path string, src []byte) {
				toks, d := lex.Lex(source.NewFile(path, src))
				if d != nil {
					t.Fatal(d)
				}
				mut := m.fn(src, toks)
				if bytes.Equal(mut, src) {
					return
				}
				changed++
				checkMutant(t, path, src, mut)
			})
			if changed == 0 {
				t.Error("the mutation changed no example")
			}
		})
	}
}

func TestTreeMutations(t *testing.T) {
	for _, m := range treeMutators {
		t.Run(m.name, func(t *testing.T) {
			changed := 0
			walkSynod(t, filepath.Join(testdata, "examples"), func(path string, src []byte) {
				tree, d := parse.Parse(source.NewFile(path, src))
				if d != nil {
					t.Fatal(d)
				}
				m.fn(tree)
				mut := emit(tree)
				if bytes.Equal(mut, src) {
					return
				}
				changed++
				checkMutant(t, path, src, mut)
			})
			if changed == 0 {
				t.Error("the mutation changed no example")
			}
		})
	}
}
