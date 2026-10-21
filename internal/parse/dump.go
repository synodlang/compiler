package parse

import (
	"fmt"
	"strconv"
	"strings"
)

// Dump renders the tree of f as an S-expression, one node per line with
// sentences on a single line. It is the format of the parser's golden files.
//
// Items print as (w name), (m a b), (n 1), (d 5ms), (t "x"), (c #F4A340),
// (p path), (paren sentence), (list item...) and (hole name type); a
// sentence is (s item... , item...) with a bare "," between groups; types
// print as their name, (list-of T) and (map-of K V). A comment prints as
// (comment L:C "text"), followed by "blank" when a blank line lies above it.
func Dump(f *File) string {
	var kids []string
	if f.Header != nil {
		kids = append(kids, "(header "+commentArgs(f.Header)+" "+strconv.Quote(f.Package)+")")
	}
	for _, n := range f.Body {
		switch n := n.(type) {
		case *Line:
			kids = append(kids, dumpLine(n)...)
		case *CommentGroup:
			kids = append(kids, wrap("group", commentLines(n.Comments))...)
		}
	}
	if len(f.Trailing) > 0 {
		kids = append(kids, wrap("trailing", commentLines(f.Trailing))...)
	}
	return strings.Join(wrap("file", kids), "\n") + "\n"
}

// wrap puts kids, which are lines, inside a node "(head ... )". A node
// without kids is the single line "(head)".
func wrap(head string, kids []string) []string {
	if len(kids) == 0 {
		return []string{"(" + head + ")"}
	}
	out := []string{"(" + head}
	for _, k := range kids {
		out = append(out, "  "+k)
	}
	out[len(out)-1] += ")"
	return out
}

func commentArgs(c *Comment) string {
	s := fmt.Sprintf("%d:%d %s", c.Pos.Line, c.Pos.Col, strconv.Quote(c.Text))
	if c.Blank {
		s += " blank"
	}
	return s
}

func commentLines(cs []*Comment) []string {
	var out []string
	for _, c := range cs {
		out = append(out, "(comment "+commentArgs(c)+")")
	}
	return out
}

func dumpLine(l *Line) []string {
	kids := leadingLines(l.Leading)
	kids = append(kids, dumpSentence(l.Sentence))
	if l.Comment != nil {
		kids = append(kids, commentLines([]*Comment{l.Comment})...)
	}
	if l.Block != nil {
		kids = append(kids, dumpBlock(l.Block)...)
	}
	for _, c := range l.Conts {
		var ck []string
		ck = append(ck, leadingLines(c.Leading)...)
		ck = append(ck, "(w "+c.Word.Name+")")
		if c.Comment != nil {
			ck = append(ck, commentLines([]*Comment{c.Comment})...)
		}
		ck = append(ck, dumpBlock(c.Block)...)
		kids = append(kids, wrap("cont", ck)...)
	}
	return wrap(fmt.Sprintf("line %d:%d", l.Pos.Line, l.Pos.Col), kids)
}

func leadingLines(cs []*Comment) []string {
	if len(cs) == 0 {
		return nil
	}
	return wrap("leading", commentLines(cs))
}

func dumpBlock(b *Block) []string {
	var kids []string
	for _, l := range b.Lines {
		kids = append(kids, dumpLine(l)...)
	}
	if len(b.Trailing) > 0 {
		kids = append(kids, wrap("trailing", commentLines(b.Trailing))...)
	}
	return wrap("block", kids)
}

func dumpSentence(s *Sentence) string {
	var b strings.Builder
	b.WriteString("(s")
	for i, g := range s.Groups {
		if i > 0 {
			b.WriteString(" ,")
		}
		for _, it := range g {
			b.WriteByte(' ')
			b.WriteString(dumpItem(it))
		}
	}
	b.WriteByte(')')
	return b.String()
}

func dumpItem(it Item) string {
	switch it := it.(type) {
	case *Word:
		return "(w " + it.Name + ")"
	case *Member:
		return "(m " + strings.Join(it.Parts, " ") + ")"
	case *Number:
		return "(n " + it.Text + ")"
	case *Duration:
		return "(d " + it.Text + ")"
	case *Text:
		return "(t " + it.Raw + ")"
	case *Color:
		return "(c " + it.Text + ")"
	case *Path:
		return "(p " + it.Text + ")"
	case *Paren:
		return "(paren " + dumpSentence(it.S) + ")"
	case *List:
		s := "(list"
		for _, e := range it.Items {
			s += " " + dumpItem(e)
		}
		return s + ")"
	case *Hole:
		return "(hole " + it.Name.Name + " " + dumpType(it.Type) + ")"
	}
	return "(?)"
}

func dumpType(t Type) string {
	switch t := t.(type) {
	case *NamedType:
		return t.Name
	case *ListType:
		return "(list-of " + dumpType(t.Elem) + ")"
	case *MapType:
		return "(map-of " + dumpType(t.Key) + " " + dumpType(t.Value) + ")"
	}
	return "?"
}
