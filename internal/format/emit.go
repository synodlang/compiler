package format

import (
	"bytes"
	"strings"

	"github.com/synodlang/compiler/internal/parse"
)

// emit lays out t as text. It prints the tree as it is, with the layout of
// spec/10 (indentation, spacing, blank lines, comments), and rewrites
// nothing: canonicalize does that.
//
// The leading use lines of the body are the imports. They follow the header
// without blank lines; one blank line separates every other top-level
// element, including the first one after the imports. Without imports the
// first element follows the header directly.
func emit(t *parse.File) []byte {
	e := &emitter{}
	if t.Header != nil {
		e.comment(0, t.Header)
	}
	i := 0
	for ; i < len(t.Body); i++ {
		l, ok := isUse(t.Body[i])
		if !ok {
			break
		}
		e.line(0, l)
	}
	sep := i > 0
	first := true
	blank := func() {
		if sep || !first {
			e.buf.WriteByte('\n')
		}
		first = false
	}
	for ; i < len(t.Body); i++ {
		blank()
		switch n := t.Body[i].(type) {
		case *parse.Line:
			e.line(0, n)
		case *parse.CommentGroup:
			e.comments(0, n.Comments)
		}
	}
	if len(t.Trailing) > 0 {
		blank()
		e.comments(0, t.Trailing)
	}
	return e.buf.Bytes()
}

type emitter struct{ buf bytes.Buffer }

func (e *emitter) indent(depth int) {
	for ; depth > 0; depth-- {
		e.buf.WriteString("  ")
	}
}

func commentText(c *parse.Comment) string {
	if c.Text == "" {
		return "#"
	}
	return "# " + c.Text
}

func (e *emitter) comment(depth int, c *parse.Comment) {
	e.indent(depth)
	e.buf.WriteString(commentText(c))
	e.buf.WriteByte('\n')
}

func (e *emitter) comments(depth int, cs []*parse.Comment) {
	for _, c := range cs {
		e.comment(depth, c)
	}
}

// head writes a sentence and its trailing comment, then a newline.
func (e *emitter) head(depth int, s *parse.Sentence, c *parse.Comment) {
	e.indent(depth)
	e.sentence(s)
	if c != nil {
		e.buf.WriteByte(' ')
		e.buf.WriteString(commentText(c))
	}
	e.buf.WriteByte('\n')
}

func (e *emitter) line(depth int, l *parse.Line) {
	e.comments(depth, l.Leading)
	e.head(depth, l.Sentence, l.Comment)
	e.block(depth, l.Block)
	for _, c := range l.Conts {
		e.comments(depth, c.Leading)
		e.indent(depth)
		e.buf.WriteString(c.Word.Name)
		if c.Comment != nil {
			e.buf.WriteByte(' ')
			e.buf.WriteString(commentText(c.Comment))
		}
		e.buf.WriteByte('\n')
		e.block(depth, c.Block)
	}
}

func (e *emitter) block(depth int, b *parse.Block) {
	if b == nil {
		return
	}
	for _, l := range b.Lines {
		e.line(depth+1, l)
	}
	e.comments(depth+1, b.Trailing)
}

func (e *emitter) sentence(s *parse.Sentence) {
	for gi, g := range s.Groups {
		if gi > 0 {
			e.buf.WriteString(", ")
		}
		for ii, it := range g {
			if ii > 0 {
				e.buf.WriteByte(' ')
			}
			e.item(it)
		}
	}
}

func (e *emitter) item(it parse.Item) {
	switch v := it.(type) {
	case *parse.Word:
		e.buf.WriteString(v.Name)
	case *parse.Member:
		e.buf.WriteString(strings.Join(v.Parts, "."))
	case *parse.Number:
		e.buf.WriteString(v.Text)
	case *parse.Duration:
		e.buf.WriteString(v.Text)
	case *parse.Text:
		e.buf.WriteString(v.Raw)
	case *parse.Color:
		e.buf.WriteString(v.Text)
	case *parse.Path:
		e.buf.WriteString(v.Text)
	case *parse.Paren:
		e.buf.WriteByte('(')
		e.sentence(v.S)
		e.buf.WriteByte(')')
	case *parse.List:
		e.buf.WriteByte('{')
		for i, x := range v.Items {
			if i > 0 {
				e.buf.WriteString(", ")
			}
			e.item(x)
		}
		e.buf.WriteByte('}')
	case *parse.Hole:
		e.buf.WriteByte('[')
		e.buf.WriteString(v.Name.Name)
		e.buf.WriteString(": ")
		e.typ(v.Type)
		e.buf.WriteByte(']')
	}
}

func (e *emitter) typ(t parse.Type) {
	switch v := t.(type) {
	case *parse.NamedType:
		e.buf.WriteString(v.Name)
	case *parse.ListType:
		e.buf.WriteString("list of ")
		e.typ(v.Elem)
	case *parse.MapType:
		e.buf.WriteString("map of ")
		e.typ(v.Key)
		e.buf.WriteString(" to ")
		e.typ(v.Value)
	}
}
