package lex

import (
	"bytes"
	"strconv"
	"unicode/utf8"

	"github.com/synodlang/compiler/internal/source"
)

// lexer holds the state of one Lex call.
type lexer struct {
	f     *source.File
	src   []byte
	toks  []Token
	stack []int // indentation of the open blocks, starting with 0
	blank bool  // a blank line was seen since the last non-blank line
	code  bool  // a code line was seen before
}

// Lex splits f into tokens. It stops at the first error, returning a nil
// slice and the diagnostic. On success the slice ends with EOF.
//
// Within one line an invalid UTF-8 byte or a stray CR is reported before any
// other problem on that line.
func Lex(f *source.File) ([]Token, *source.Diagnostic) {
	toks, d := LexPartial(f)
	if d != nil {
		return nil, d
	}
	return toks, nil
}

// LexPartial is Lex that keeps the tokens read before the first error: on
// failure it returns them (without EOF) together with the diagnostic.
func LexPartial(f *source.File) ([]Token, *source.Diagnostic) {
	l := &lexer{f: f, src: f.Src, stack: []int{0}}
	off := 0
	if bytes.HasPrefix(l.src, []byte("\xef\xbb\xbf")) {
		off = 3
	}
	for off < len(l.src) {
		end := len(l.src)
		next := end
		if i := bytes.IndexByte(l.src[off:], '\n'); i >= 0 {
			end = off + i
			next = end + 1
			if end > off && l.src[end-1] == '\r' {
				end-- // CR LF counts as one newline
			}
		}
		if d := l.line(off, end, next-1); d != nil {
			return l.toks, d
		}
		off = next
	}
	for len(l.stack) > 1 {
		l.stack = l.stack[:len(l.stack)-1]
		l.emit(DEDENT, len(l.src), len(l.src))
	}
	l.emit(EOF, len(l.src), len(l.src))
	return l.toks, nil
}

// fail builds a diagnostic whose primary span is empty, at byte offset off.
func (l *lexer) fail(off int, code source.Code, msg, hint string, more ...source.Extra) *source.Diagnostic {
	return l.failRange(off, off, code, msg, hint, more...)
}

// failRange builds a diagnostic whose primary span is the bytes
// src[start:end]. Both offsets are mapped with PosOf.
func (l *lexer) failRange(start, end int, code source.Code, msg, hint string, more ...source.Extra) *source.Diagnostic {
	d := source.NewSpan(l.f, l.span(start, end), code, msg, hint, more...)
	return &d
}

// span returns the span of the bytes src[start:end].
func (l *lexer) span(start, end int) source.Span {
	return source.Span{Start: l.f.PosOf(start), End: l.f.PosOf(end)}
}

// emit appends a token for src[start:end]. Structural kinds carry no text.
func (l *lexer) emit(k Kind, start, end int) *Token {
	t := Token{Kind: k, Pos: l.f.PosOf(start), Off: start}
	switch k {
	case INDENT, DEDENT, NEWLINE, EOF:
	default:
		t.Text = string(l.src[start:end])
		t.Blank, l.blank = l.blank, false
	}
	l.toks = append(l.toks, t)
	return &l.toks[len(l.toks)-1]
}

// line lexes the line src[start:end]; term is the offset of its newline (or
// the end of the file) and is where NEWLINE is placed.
func (l *lexer) line(start, end, term int) *source.Diagnostic {
	// Byte level checks first.
	for i := start; i < end; {
		r, n := utf8.DecodeRune(l.src[i:end])
		switch {
		case r == utf8.RuneError && n == 1:
			return l.failRange(i, i+1, source.Syntax, "invalid UTF-8", "")
		case r == '\r':
			return l.failRange(i, i+1, source.Syntax, "carriage return without line feed", "")
		}
		i += n
	}

	i := start
	for i < end && l.src[i] == ' ' {
		i++
	}
	if i < end && l.src[i] == '\t' {
		j := i
		for j < end && (l.src[j] == ' ' || l.src[j] == '\t') {
			j++
		}
		return l.failRange(i, j, source.Indent, "tabs are not allowed", "indent with 2 spaces per level",
			source.HelpText("indent with 2 spaces per level"))
	}
	if i == end {
		l.blank = true
		return nil
	}

	// An own-line comment takes no part in indentation.
	if l.src[i] == '#' && (i+1 == end || l.src[i+1] == ' ') {
		l.comment(i, end).Own = true
		l.emit(NEWLINE, term, term)
		return nil
	}

	if d := l.indent(start, i-start, end); d != nil {
		return d
	}
	l.code = true
	if d := l.scan(i, end); d != nil {
		return d
	}
	l.emit(NEWLINE, term, term)
	return nil
}

// comment emits the comment starting at i and running to end.
func (l *lexer) comment(i, end int) *Token {
	for end > i && (l.src[end-1] == ' ' || l.src[end-1] == '\t') {
		end--
	}
	return l.emit(COMMENT, i, end)
}

// indent handles the indentation width of a code line that starts at
// lineStart and ends at end. The primary span of its errors is the first
// word of the line, which is where the position of the error lies.
func (l *lexer) indent(lineStart, width, end int) *source.Diagnostic {
	at := lineStart + width
	word := func() int {
		j := at
		for j < end && l.src[j] != ' ' {
			j++
		}
		return j
	}
	if width%2 != 0 {
		return l.failRange(at, word(), source.Indent, "indentation is not a multiple of 2 spaces", "",
			source.WithLabel("indented "+strconv.Itoa(width)+" spaces"),
			source.Note("indentation steps are 2 spaces"))
	}
	top := l.stack[len(l.stack)-1]
	switch {
	case width > top:
		if width != top+2 || !l.code {
			label := "indented " + strconv.Itoa(width) + " spaces, at most " + strconv.Itoa(top+2) + " allowed"
			if !l.code {
				label = "the first line starts at the left edge"
			}
			return l.failRange(at, word(), source.Indent, "unexpected indent",
				"a block is indented 2 spaces more than its statement",
				source.WithLabel(label),
				source.HelpText("indent a block 2 spaces more than its statement"))
		}
		l.stack = append(l.stack, width)
		l.emit(INDENT, lineStart, lineStart)
	case width < top:
		for l.stack[len(l.stack)-1] > width {
			l.stack = l.stack[:len(l.stack)-1]
			l.emit(DEDENT, lineStart, lineStart)
		}
	}
	return nil
}
