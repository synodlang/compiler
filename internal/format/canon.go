package format

import (
	"sort"
	"strings"

	"github.com/synodlang/compiler/internal/lex"
	"github.com/synodlang/compiler/internal/parse"
)

// canonicalize rewrites the content of t in place into canonical form:
//
//  1. numbers, durations and colors get their canonical spelling;
//  2. parentheses around a single item are removed, except around the first
//     item of a line (so "(use) x" stays an ordinary line and "(else)" never
//     becomes a continuation line);
//  3. "not (a is b)" inside parentheses becomes "a is not b", and likewise
//     for the other comparison pairs;
//  4. "use" lines move to the front of the file, sorted by import path.
//
// It does not touch layout; see emit.
func canonicalize(t *parse.File) {
	for _, n := range t.Body {
		if l, ok := n.(*parse.Line); ok {
			canonLine(l)
		}
	}
	hoistImports(t)
}

func canonLine(l *parse.Line) {
	canonSentence(l.Sentence, true)
	canonBlock(l.Block)
	for _, c := range l.Conts {
		canonBlock(c.Block)
	}
}

func canonBlock(b *parse.Block) {
	if b == nil {
		return
	}
	for _, l := range b.Lines {
		canonLine(l)
	}
}

// canonSentence canonicalizes the items of s. head reports that s is the
// sentence of a line, whose first item keeps its parentheses.
func canonSentence(s *parse.Sentence, head bool) {
	for gi, g := range s.Groups {
		for ii, it := range g {
			it = canonItem(it)
			if !(head && gi == 0 && ii == 0) {
				it = unwrap(it)
			}
			g[ii] = it
		}
	}
}

// unwrap removes parentheses around a single item, repeatedly.
func unwrap(it parse.Item) parse.Item {
	for {
		p, ok := it.(*parse.Paren)
		if !ok || len(p.S.Groups) != 1 || len(p.S.Groups[0]) != 1 {
			return it
		}
		it = p.S.Groups[0][0]
	}
}

func canonItem(it parse.Item) parse.Item {
	switch v := it.(type) {
	case *parse.Number:
		v.Text = lex.CanonicalNumber(v.Text)
	case *parse.Duration:
		v.Text = lex.CanonicalDuration(v.Text)
	case *parse.Color:
		v.Text = lex.CanonicalColor(v.Text)
	case *parse.Paren:
		canonSentence(v.S, false)
		negate(v)
	case *parse.List:
		for i, e := range v.Items {
			v.Items[i] = unwrap(canonItem(e))
		}
	}
	return it
}

// comparison describes the one comparison operator of a group.
type comparison struct {
	at, n   int      // operator words are g[at : at+n]
	inverse []string // the operator that means the opposite
}

var inverses = map[string]string{
	"is":        "is not",
	"is not":    "is",
	"is in":     "is not in",
	"is not in": "is in",
	"less than": "at least",
	"at least":  "less than",
	"more than": "at most",
	"at most":   "more than",
}

// findComparison locates the comparison operator in g. A group has one when
// it holds exactly one of the words is, less, more and at, that word starts a
// known operator, and there is an operand on each side. Groups with "and" or
// "or" have none, since negating them would change their meaning.
func findComparison(g []parse.Item) (comparison, bool) {
	at := -1
	for i, it := range g {
		w, ok := it.(*parse.Word)
		if !ok {
			continue
		}
		switch w.Name {
		case "and", "or":
			return comparison{}, false
		case "is", "less", "more", "at":
			if at >= 0 {
				return comparison{}, false
			}
			at = i
		}
	}
	if at < 1 {
		return comparison{}, false
	}
	var op []string
	for j := at; j < len(g) && j < at+3; j++ {
		w, ok := g[j].(*parse.Word)
		if !ok {
			break
		}
		op = append(op, w.Name)
	}
	for n := len(op); n >= 1; n-- {
		if inv, ok := inverses[strings.Join(op[:n], " ")]; ok && at+n < len(g) {
			return comparison{at: at, n: n, inverse: strings.Fields(inv)}, true
		}
	}
	return comparison{}, false
}

// negate rewrites a parenthesized "not (A op B)" into "(A op' B)".
func negate(p *parse.Paren) {
	if len(p.S.Groups) != 1 || len(p.S.Groups[0]) != 2 {
		return
	}
	g := p.S.Groups[0]
	inner, ok := g[1].(*parse.Paren)
	if !parse.IsWord(g[0], "not") || !ok || len(inner.S.Groups) != 1 {
		return
	}
	c, ok := findComparison(inner.S.Groups[0])
	if !ok {
		return
	}
	p.S = &parse.Sentence{Groups: [][]parse.Item{replaceOp(inner.S.Groups[0], c)}}
}

// replaceOp returns g with its operator replaced by the inverse.
func replaceOp(g []parse.Item, c comparison) []parse.Item {
	out := make([]parse.Item, 0, len(g)+1)
	out = append(out, g[:c.at]...)
	pos := g[c.at].Pos()
	for _, w := range c.inverse {
		out = append(out, &parse.Word{P: pos, Name: w})
	}
	return append(out, g[c.at+c.n:]...)
}

// isUse reports whether n is an import line.
func isUse(n parse.Node) (*parse.Line, bool) {
	l, ok := n.(*parse.Line)
	if !ok || !parse.IsWord(l.Sentence.Groups[0][0], "use") {
		return nil, false
	}
	return l, true
}

// importPath returns the path of an import line, or "".
func importPath(l *parse.Line) string {
	g := l.Sentence.Groups[0]
	if len(g) > 1 {
		if p, ok := g[1].(*parse.Path); ok {
			return p.Text
		}
	}
	return ""
}

// hoistImports moves the use lines to the front of the body, stably sorted by
// the byte order of their path.
//
// It does nothing when the first import would bring a leading comment that
// reads like a package header into the first line of a file without header:
// the comment would turn into a header on the next run.
func hoistImports(t *parse.File) {
	var uses []*parse.Line
	var rest []parse.Node
	for _, n := range t.Body {
		if l, ok := isUse(n); ok {
			uses = append(uses, l)
		} else {
			rest = append(rest, n)
		}
	}
	if len(uses) == 0 {
		return
	}
	sort.SliceStable(uses, func(i, j int) bool { return importPath(uses[i]) < importPath(uses[j]) })
	if t.Header == nil && len(uses[0].Leading) > 0 && (strings.HasPrefix(uses[0].Leading[0].Text, "package ") || strings.HasPrefix(uses[0].Leading[0].Text, "theme ")) {
		return
	}
	body := make([]parse.Node, 0, len(t.Body))
	for _, l := range uses {
		body = append(body, l)
	}
	t.Body = append(body, rest...)
}
