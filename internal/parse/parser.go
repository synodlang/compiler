package parse

import (
	"bytes"
	"strings"

	"github.com/synodlang/compiler/internal/lex"
	"github.com/synodlang/compiler/internal/source"
)

// maxNest is the deepest nesting of parentheses, lists and types.
const maxNest = 100

// Parse parses f. The first syntax or indent error stops the parse: the
// result is then nil and the diagnostic says what is wrong.
//
// When the lexer fails on some line, a structural error on an earlier line is
// reported in preference, so that the diagnostic is the first one in the file.
func Parse(f *source.File) (*File, *source.Diagnostic) {
	toks, d := lex.Lex(f)
	if d != nil {
		return nil, earlier(f, d)
	}
	return parseTokens(f, toks)
}

// earlier returns the diagnostic of the first error of f, given that lexing
// failed with d: a parse error in the lines before d's line, if there is one,
// else d.
func earlier(f *source.File, d *source.Diagnostic) *source.Diagnostic {
	off, line := 0, 1
	for line < d.Line {
		i := bytes.IndexByte(f.Src[off:], '\n')
		if i < 0 {
			return d
		}
		off += i + 1
		line++
	}
	if off == 0 {
		return d
	}
	head := source.NewFile(f.Path, f.Src[:off])
	toks, d2 := lex.Lex(head)
	if d2 != nil {
		return d
	}
	if _, pd := parseTokens(head, toks); pd != nil {
		return pd
	}
	return d
}

// bailout carries a diagnostic out of the recursive descent.
type bailout struct{ d *source.Diagnostic }

type parser struct {
	f       *source.File
	toks    []lex.Token
	i       int
	pending []*Comment // own-line comments not yet attached
	nest    int        // nesting of parentheses, lists and types
	holesOK bool       // the current line may hold holes
}

func parseTokens(f *source.File, toks []lex.Token) (file *File, d *source.Diagnostic) {
	p := &parser{f: f, toks: toks}
	defer func() {
		if r := recover(); r != nil {
			b, ok := r.(bailout)
			if !ok {
				panic(r)
			}
			file, d = nil, b.d
		}
	}()
	file = &File{Source: f}
	if t := p.tok(); t.Kind == lex.COMMENT && t.Own && isHeaderText(commentText(t)) {
		file.Header = p.comment()
		file.Package = packageName(file.Header.Text)
		file.Theme = strings.HasPrefix(file.Header.Text, themePrefix)
		p.skipNewline()
	}
	body := p.seq(0)
	if p.tok().Kind != lex.EOF {
		p.failTok(p.tok(), "unexpected "+p.tok().Kind.String())
	}
	// Whatever comments remain are in the pending list; the last run of them
	// is the file's trailing comments, earlier runs are detached groups.
	segs := splitRuns(p.pending)
	for i, seg := range segs {
		if i == len(segs)-1 {
			file.Trailing = seg
		} else {
			body = append(body, &CommentGroup{Comments: seg})
		}
	}
	file.Body = body
	return file, nil
}

// tok returns the current token.
func (p *parser) tok() *lex.Token { return &p.toks[p.i] }

// fail stops the parse with a syntax error whose primary span is sp.
func (p *parser) fail(sp source.Span, msg, hint string, more ...source.Extra) {
	d := source.NewSpan(p.f, sp, source.Syntax, msg, hint, more...)
	panic(bailout{&d})
}

// tokSpan returns the span of the token t.
func tokSpan(t *lex.Token) source.Span { return source.Span{Start: t.Pos, End: t.End()} }

// failTok is fail at the span of t.
func (p *parser) failTok(t *lex.Token, msg string) { p.fail(tokSpan(t), msg, "") }

// lineEnd returns the end of the code on the line of the token at index i,
// from the token at i on: the end of its last token before the comment or
// the end of the line.
func (p *parser) lineEnd(i int) source.Pos {
	end := p.toks[i].End()
	for j := i; j < len(p.toks); j++ {
		switch p.toks[j].Kind {
		case lex.NEWLINE, lex.EOF, lex.COMMENT, lex.INDENT, lex.DEDENT:
			return end
		}
		end = p.toks[j].End()
	}
	return end
}

// holeEnd returns the end of the hole whose "[" is the token at index i: the
// matching "]", or else the end of the line.
func (p *parser) holeEnd(i int) source.Pos {
	depth := 0
	for j := i; j < len(p.toks); j++ {
		switch p.toks[j].Kind {
		case lex.LBRACKET:
			depth++
		case lex.RBRACKET:
			if depth--; depth == 0 {
				return p.toks[j].End()
			}
		case lex.NEWLINE, lex.EOF, lex.COMMENT:
			return p.lineEnd(i)
		}
	}
	return p.lineEnd(i)
}

// stray fails on a token that cannot continue a sentence.
func (p *parser) stray(t *lex.Token) {
	switch t.Kind {
	case lex.COLON:
		p.failTok(t, "':' appears only inside a hole")
	case lex.COMMA:
		p.failTok(t, "unexpected ','")
	case lex.RPAREN:
		p.failTok(t, `unexpected ")"`)
	case lex.RBRACE:
		p.failTok(t, `unexpected "}"`)
	case lex.RBRACKET:
		p.failTok(t, `unexpected "]"`)
	}
	p.failTok(t, "unexpected "+t.Kind.String())
}

// commentText returns the text of a COMMENT token after "#" and one space.
func commentText(t *lex.Token) string {
	s := strings.TrimPrefix(t.Text, "#")
	return strings.TrimPrefix(s, " ")
}

// comment consumes a COMMENT token.
func (p *parser) comment() *Comment {
	t := p.tok()
	p.i++
	return &Comment{Pos: t.Pos, Text: commentText(t), Blank: t.Blank}
}

// skipNewline consumes the NEWLINE that ends a line.
func (p *parser) skipNewline() {
	if t := p.tok(); t.Kind == lex.NEWLINE {
		p.i++
		return
	}
	p.stray(p.tok())
}

// comments moves the own-line comments at the current position to pending.
func (p *parser) comments() {
	for p.tok().Kind == lex.COMMENT && p.tok().Own {
		p.pending = append(p.pending, p.comment())
		p.skipNewline()
	}
}

// takePending returns the pending comments and empties the list.
func (p *parser) takePending() []*Comment {
	c := p.pending
	p.pending = nil
	return c
}

// splitRuns cuts cs before every comment that has a blank line above it. The
// result has no empty run, and no runs at all for no comments.
func splitRuns(cs []*Comment) [][]*Comment {
	var runs [][]*Comment
	for i, c := range cs {
		if i == 0 || c.Blank {
			runs = append(runs, nil)
		}
		runs[len(runs)-1] = append(runs[len(runs)-1], c)
	}
	return runs
}

// seq parses lines at block depth depth until a DEDENT or EOF, which it
// leaves in place. Own-line comments after the last line stay pending.
func (p *parser) seq(depth int) []Node {
	var out []Node
	for {
		p.comments()
		if k := p.tok().Kind; k == lex.DEDENT || k == lex.EOF {
			return out
		}
		out = p.line(out, depth)
	}
}

// line parses one line, a block and all, and adds it to out.
func (p *parser) line(out []Node, depth int) []Node {
	first := p.tok()
	blank := first.Blank
	if first.Kind == lex.WORD {
		if depth > 0 && first.Text == "use" {
			p.fail(source.Span{Start: first.Pos, End: p.lineEnd(p.i)}, "use appears only at the top level", "")
		}
		p.holesOK = first.Text == "define" || first.Text == "when"
	} else {
		p.holesOK = false
	}
	lead := p.takePending()
	sent := p.sentence()
	if sent == nil {
		p.stray(p.tok())
	}
	if first.Kind == lex.WORD && first.Text == "use" {
		// The lexer reads an import path only after whitespace, so
		// "use(x)" lexes without one; its printed form would not lex.
		if g := sent.Groups[0]; len(g) < 2 || !isPath(g[1]) {
			p.fail(source.At(first.End()), "missing import path after use", "", source.WithLabel("path expected here"))
		}
	}
	var trailing *Comment
	if p.tok().Kind == lex.COMMENT {
		trailing = p.comment()
	}
	p.skipNewline()
	p.comments()

	if p.tok().Kind != lex.INDENT {
		ln := &Line{Pos: first.Pos, Sentence: sent, Comment: trailing}
		return p.addLine(out, ln, lead, depth, blank)
	}
	// A line with a block. A single bare word is a continuation line.
	if w := soleWord(sent); w != nil {
		if len(out) > 0 {
			if prev, ok := out[len(out)-1].(*Line); ok && prev.Block != nil {
				prev.Conts = append(prev.Conts, &Continuation{
					Leading: lead, Word: w, Comment: trailing, Block: p.block(depth),
				})
				return out
			}
		}
		var more []source.Extra
		if len(out) > 0 {
			if prev, ok := out[len(out)-1].(*Line); ok {
				more = append(more, source.Also(p.f.Path, LineSpan(prev), "this line has no block"))
			}
		}
		p.fail(SpanOf(w), "a continuation line must follow a block",
			`"else" goes directly after its if block, at the if's indentation`, more...)
	}
	ln := &Line{Pos: first.Pos, Sentence: sent, Comment: trailing}
	ln.Block = p.block(depth)
	return p.addLine(out, ln, lead, depth, blank)
}

// addLine appends ln to out with its leading comments. At the top level a
// blank line cuts the comments above it into detached groups.
func (p *parser) addLine(out []Node, ln *Line, lead []*Comment, depth int, blank bool) []Node {
	if depth > 0 {
		ln.Leading = lead
		return append(out, ln)
	}
	runs := splitRuns(lead)
	for i, run := range runs {
		if i == len(runs)-1 && !blank {
			ln.Leading = run
		} else {
			out = append(out, &CommentGroup{Comments: run})
		}
	}
	return append(out, ln)
}

// soleWord returns the word if s is exactly one bare word.
func soleWord(s *Sentence) *Word {
	if len(s.Groups) == 1 && len(s.Groups[0]) == 1 {
		w, _ := s.Groups[0][0].(*Word)
		return w
	}
	return nil
}

// block parses the block at the current INDENT, whose lines are at block
// depth depth+1, and the DEDENT that closes it.
func (p *parser) block(depth int) *Block {
	p.i++ // INDENT
	col := p.tok().Pos.Col
	nodes := p.seq(depth + 1)
	if p.tok().Kind == lex.DEDENT {
		p.i++
	}
	b := &Block{}
	for _, n := range nodes {
		b.Lines = append(b.Lines, n.(*Line))
	}
	n := 0
	for n < len(p.pending) && p.pending[n].Pos.Col >= col {
		n++
	}
	if n > 0 {
		b.Trailing = p.pending[:n:n]
		p.pending = p.pending[n:]
	}
	return b
}

// itemStart reports whether a token of kind k begins an item.
func itemStart(k lex.Kind) bool {
	switch k {
	case lex.WORD, lex.MEMBER, lex.NUMBER, lex.DURATION, lex.TEXT, lex.COLOR,
		lex.PATH, lex.LPAREN, lex.LBRACE, lex.LBRACKET:
		return true
	}
	return false
}

// sentence parses a sentence of a line or of a pair of parentheses: items and
// commas up to the first token that cannot belong to it, which is left in
// place. It returns nil if there is no item at all.
func (p *parser) sentence() *Sentence {
	var groups [][]Item
	var cur []Item
	var commas []*lex.Token
	for {
		t := p.tok()
		switch {
		case itemStart(t.Kind):
			cur = append(cur, p.item())
			continue
		case t.Kind == lex.COMMA:
			if len(cur) == 0 {
				p.failTok(t, "unexpected ','")
			}
			groups = append(groups, cur)
			cur = nil
			commas = append(commas, t)
			p.i++
			continue
		case t.Kind == lex.COLON:
			p.stray(t)
		}
		break
	}
	if len(cur) == 0 {
		if len(groups) > 0 {
			p.failTok(commas[len(commas)-1], "unexpected ','")
		}
		return nil
	}
	groups = append(groups, cur)
	p.checkCommas(groups, commas)
	return &Sentence{Groups: groups}
}

// checkCommas enforces where commas may stand in a line or parenthesized
// sentence: between the two names of "each", and between the settings of
// "message T with" (spec/02 section 2.1). It reports the first comma
// otherwise.
func (p *parser) checkCommas(groups [][]Item, commas []*lex.Token) {
	if len(groups) < 2 {
		return
	}
	g := groups[0]
	if len(groups) == 2 && len(g) == 2 && IsWord(g[0], "each") {
		if _, ok := g[1].(*Word); ok {
			return
		}
	}
	if len(g) >= 4 && IsWord(g[0], "message") && IsWord(g[2], "with") {
		return
	}
	p.failTok(commas[0], "a comma stands only between list items, message settings and each names")
}

// enter increases the nesting depth, failing at t when it is too deep.
func (p *parser) enter(t *lex.Token) {
	p.nest++
	if p.nest > maxNest {
		p.failTok(t, "nesting too deep")
	}
}

// item parses one item; the current token begins one.
func (p *parser) item() Item {
	t := p.tok()
	switch t.Kind {
	case lex.WORD:
		p.i++
		return &Word{P: t.Pos, Name: t.Text}
	case lex.MEMBER:
		p.i++
		return &Member{P: t.Pos, Parts: strings.Split(t.Text, ".")}
	case lex.NUMBER:
		p.i++
		return &Number{P: t.Pos, Text: t.Text}
	case lex.DURATION:
		p.i++
		return &Duration{P: t.Pos, Text: t.Text}
	case lex.TEXT:
		p.i++
		return &Text{P: t.Pos, Raw: t.Text}
	case lex.COLOR:
		p.i++
		return &Color{P: t.Pos, Text: t.Text}
	case lex.PATH:
		p.i++
		return &Path{P: t.Pos, Text: t.Text}
	case lex.LPAREN:
		return p.paren()
	case lex.LBRACE:
		return p.list()
	}
	return p.hole()
}

// paren parses "(" sentence ")".
func (p *parser) paren() Item {
	open := p.tok()
	oi := p.i
	p.enter(open)
	p.i++
	s := p.sentence()
	switch t := p.tok(); {
	case t.Kind == lex.RPAREN:
		if s == nil {
			p.fail(source.Span{Start: open.Pos, End: t.End()}, "empty parentheses", "")
		}
	case t.Kind == lex.RBRACE || t.Kind == lex.RBRACKET:
		p.stray(t)
	default:
		p.unclosed(oi, `unclosed "("`)
	}
	end := p.tok().End()
	p.i++
	p.nest--
	return &Paren{P: open.Pos, E: end, S: s}
}

// unclosed fails with msg on the opening token at index oi: its span runs to
// the end of the line.
func (p *parser) unclosed(oi int, msg string) {
	p.fail(source.Span{Start: p.toks[oi].Pos, End: p.lineEnd(oi)}, msg, "",
		source.WithLabel("unclosed here"))
}

// list parses "{" [ item { "," item } ] "}".
func (p *parser) list() Item {
	open := p.tok()
	oi := p.i
	p.enter(open)
	p.i++
	l := &List{P: open.Pos}
	if p.tok().Kind == lex.RBRACE {
		l.E = p.tok().End()
		p.i++
		p.nest--
		return l
	}
	for {
		var els []Item
		for itemStart(p.tok().Kind) {
			els = append(els, p.item())
		}
		switch t := p.tok(); {
		case len(els) == 0 && (t.Kind == lex.COMMA || t.Kind == lex.RBRACE):
			p.fail(tokSpan(t), "missing list item", "write one item between commas, and no comma before }")
		case len(els) > 1:
			run := SpanOfItems(els)
			p.fail(run, "a list item is one item", "parenthesize it",
				source.HelpEdit("parenthesize the item",
					source.EditAt(source.At(run.Start), "("), source.EditAt(source.At(run.End), ")")))
		case len(els) == 1:
			l.Items = append(l.Items, els[0])
		}
		switch t := p.tok(); t.Kind {
		case lex.COMMA:
			p.i++
		case lex.RBRACE:
			l.E = t.End()
			p.i++
			p.nest--
			return l
		case lex.NEWLINE, lex.COMMENT, lex.EOF:
			p.unclosed(oi, `unclosed "{"`)
		default:
			p.stray(t)
		}
	}
}

// hole parses "[" word ":" type "]".
func (p *parser) hole() Item {
	open := p.tok()
	oi := p.i
	whole := func() source.Span { return source.Span{Start: open.Pos, End: p.holeEnd(oi)} }
	if !p.holesOK {
		p.fail(whole(), "holes appear only in define and when lines", "")
	}
	bad := func() { p.fail(whole(), "a hole is [name: type]", "") }
	p.i++
	nt := p.tok()
	if nt.Kind != lex.WORD {
		bad()
	}
	p.i++
	if p.tok().Kind != lex.COLON {
		bad()
	}
	p.i++
	if k := p.tok().Kind; k == lex.RBRACKET || k == lex.NEWLINE || k == lex.COMMENT || k == lex.EOF {
		bad()
	}
	typ := p.typ()
	if p.tok().Kind != lex.RBRACKET {
		bad()
	}
	end := p.tok().End()
	p.i++
	return &Hole{P: open.Pos, E: end, Name: &Word{P: nt.Pos, Name: nt.Text}, Type: typ}
}

// typ parses a hole type.
func (p *parser) typ() Type {
	t := p.tok()
	bad := func() {
		p.fail(tokSpan(t), "bad type", "write list of T / map of K to V")
	}
	expect := func(word string) {
		if n := p.tok(); n.Kind != lex.WORD || n.Text != word {
			bad()
		}
		p.i++
	}
	switch t.Kind {
	case lex.MEMBER:
		if strings.Count(t.Text, ".") != 1 {
			bad()
		}
		p.i++
		return &NamedType{P: t.Pos, Name: t.Text}
	case lex.WORD:
		p.i++
		switch t.Text {
		case "list":
			expect("of")
			p.enter(t)
			defer func() { p.nest-- }()
			elem := p.typ()
			return &ListType{P: t.Pos, E: elem.End(), Elem: elem}
		case "map":
			expect("of")
			p.enter(t)
			defer func() { p.nest-- }()
			k := p.typ()
			expect("to")
			v := p.typ()
			return &MapType{P: t.Pos, E: v.End(), Key: k, Value: v}
		}
		return &NamedType{P: t.Pos, Name: t.Text}
	}
	bad()
	return nil
}

// isPath reports whether it is an import path.
func isPath(it Item) bool {
	_, ok := it.(*Path)
	return ok
}
