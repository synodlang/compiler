package resolve

import (
	"strings"

	"github.com/synodlang/compiler/internal/parse"
	"github.com/synodlang/compiler/internal/source"
)

// printItem prints an item the way the source writes it.
func printItem(it parse.Item) string {
	var b strings.Builder
	writeItem(&b, it)
	return b.String()
}

func writeItem(b *strings.Builder, it parse.Item) {
	switch v := it.(type) {
	case *parse.Word:
		b.WriteString(v.Name)
	case *parse.Member:
		b.WriteString(strings.Join(v.Parts, "."))
	case *parse.Number:
		b.WriteString(v.Text)
	case *parse.Duration:
		b.WriteString(v.Text)
	case *parse.Text:
		b.WriteString(v.Raw)
	case *parse.Color:
		b.WriteString(v.Text)
	case *parse.Path:
		b.WriteString(v.Text)
	case *parse.Paren:
		b.WriteByte('(')
		writeSentence(b, v.S)
		b.WriteByte(')')
	case *parse.List:
		b.WriteByte('{')
		for i, e := range v.Items {
			if i > 0 {
				b.WriteString(", ")
			}
			writeItem(b, e)
		}
		b.WriteByte('}')
	case *parse.Hole:
		b.WriteString(holeText(v))
	}
}

func writeItems(b *strings.Builder, items []parse.Item) {
	for i, it := range items {
		if i > 0 {
			b.WriteByte(' ')
		}
		writeItem(b, it)
	}
}

func writeSentence(b *strings.Builder, s *parse.Sentence) {
	for i, g := range s.Groups {
		if i > 0 {
			b.WriteString(", ")
		}
		writeItems(b, g)
	}
}

// printSentence prints a sentence as written.
func printSentence(s *parse.Sentence) string {
	var b strings.Builder
	writeSentence(&b, s)
	return b.String()
}

// printItems prints a run of items.
func printItems(items []parse.Item) string {
	var b strings.Builder
	writeItems(&b, items)
	return b.String()
}

// partPos is the position of part i of a member.
func partPos(m *parse.Member, i int) source.Pos {
	col := m.P.Col
	for j := 0; j < i; j++ {
		col += len([]rune(m.Parts[j])) + 1
	}
	return source.Pos{Line: m.P.Line, Col: col}
}

// unquote resolves the escapes of a text literal (with its quotes).
func unquote(raw string) string {
	s := raw
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		s = s[1 : len(s)-1]
	}
	if !strings.Contains(s, `\`) {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) {
			i++
			switch s[i] {
			case 'n':
				b.WriteByte('\n')
			default:
				b.WriteByte(s[i])
			}
			continue
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

func lowerFirst(s string) string {
	if s == "" {
		return s
	}
	return strings.ToLower(s[:1]) + s[1:]
}
