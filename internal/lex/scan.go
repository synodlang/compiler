package lex

import (
	"strconv"
	"unicode/utf8"

	"github.com/synodlang/compiler/internal/source"
)

func isLetter(c byte) bool { return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' }
func isDigit(c byte) bool  { return c >= '0' && c <= '9' }
func isAlnum(c byte) bool  { return isLetter(c) || isDigit(c) }
func isHex(c byte) bool {
	return isDigit(c) || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F'
}

var punct = map[byte]Kind{
	'(': LPAREN, ')': RPAREN, '{': LBRACE, '}': RBRACE,
	'[': LBRACKET, ']': RBRACKET, ':': COLON, ',': COMMA,
}

// syntax builds a syntax diagnostic whose primary span is src[start:end].
func (l *lexer) syntax(start, end int, msg, hint string, more ...source.Extra) *source.Diagnostic {
	return l.failRange(start, end, source.Syntax, msg, hint, more...)
}

// atom emits an atom token for src[start:end], refusing it when it touches
// the atom before it. Atoms must be separated by whitespace or punctuation.
func (l *lexer) atom(k Kind, start, end int) *source.Diagnostic {
	if n := len(l.toks); n > 0 {
		prev := l.toks[n-1]
		if prev.Kind.isAtom() && prev.Off+len(prev.Text) == start {
			return l.syntax(start, end, "missing space between "+strconv.Quote(prev.Text)+
				" and "+strconv.Quote(string(l.src[start:end])), "",
				source.WithLabel("add a space before this"),
				source.HelpEdit("put a space between them", source.EditAt(source.At(l.f.PosOf(start)), " ")))
		}
	}
	l.emit(k, start, end)
	return nil
}

// scan lexes the tokens of the code line src[i:end].
func (l *lexer) scan(i, end int) *source.Diagnostic {
	first := len(l.toks) // index of the line's first token
	for i < end {
		c := l.src[i]
		var d *source.Diagnostic
		switch {
		case c == ' ':
			i++
		case c == '\t':
			return l.syntax(i, i+1, "tabs are not allowed", "")
		case c == '#':
			i, d = l.hash(i, end)
		case isLetter(c):
			i, d = l.word(i, end)
			if d == nil && len(l.toks) == first+1 && l.toks[first].Kind == WORD && l.toks[first].Text == "use" {
				i, d = l.path(i, end)
			}
		case isDigit(c) || c == '-':
			i, d = l.number(i, end)
		case c == '.':
			j := i + 1
			if j < end && isDigit(l.src[j]) {
				for j < end && isDigit(l.src[j]) {
					j++
				}
				txt := string(l.src[i:j])
				return l.syntax(i, j, strconv.Quote(txt)+" is not a number", "write 0"+txt,
					source.HelpEdit("write 0"+txt, source.EditAt(source.At(l.f.PosOf(i)), "0")))
			}
			return l.unexpected(i, end)
		case c == '+':
			return l.syntax(i, i+1, `"+" is not allowed`, "add with plus; numbers have no leading +",
				source.HelpText(`write numbers without a leading "+"; add with "plus"`))
		case c == '"':
			i, d = l.text(i, end)
		case c == '_':
			return l.syntax(i, i+1, "names have no underscores", "")
		default:
			k, ok := punct[c]
			if !ok {
				return l.unexpected(i, end)
			}
			l.emit(k, i, i+1)
			i++
		}
		if d != nil {
			return d
		}
	}
	return nil
}

// unterminated reports the text literal that starts at the quote at start
// and runs to end, the end of the line.
func (l *lexer) unterminated(start, end int) *source.Diagnostic {
	return l.syntax(start, end, "unterminated text", "", source.WithLabel("text starts here"))
}

// unexpected reports the character at i.
func (l *lexer) unexpected(i, end int) *source.Diagnostic {
	r, n := utf8.DecodeRune(l.src[i:end])
	return l.syntax(i, i+n, "unexpected character "+strconv.Quote(string(r)), "")
}

// hash lexes a trailing comment or a color literal starting at i.
func (l *lexer) hash(i, end int) (int, *source.Diagnostic) {
	if i+1 == end || l.src[i+1] == ' ' {
		l.comment(i, end)
		return end, nil
	}
	if i+7 <= end && (i+7 == end || !isAlnum(l.src[i+7])) {
		ok := true
		for _, h := range l.src[i+1 : i+7] {
			ok = ok && isHex(h)
		}
		if ok {
			return i + 7, l.atom(COLOR, i, i+7)
		}
	}
	j := i + 1
	for j < end && l.src[j] != ' ' {
		j++
	}
	return i, l.syntax(i, j, "# starts a comment only when followed by a space",
		`write "# text"; a color is # and six hex digits`,
		source.HelpEdit(`put a space after "#" to write a comment`, source.EditAt(source.At(l.f.PosOf(i+1)), " ")),
		source.Note("a color is # and six hex digits"))
}

// word lexes a word or a member starting at i.
func (l *lexer) word(i, end int) (int, *source.Diagnostic) {
	start := i
	kind := WORD
	for {
		for i < end && isAlnum(l.src[i]) {
			i++
		}
		if i >= end || l.src[i] != '.' {
			break
		}
		if i+1 < end && isLetter(l.src[i+1]) {
			kind = MEMBER
			i++
			continue
		}
		member := string(l.src[start:i])
		if i+1 < end && isDigit(l.src[i+1]) {
			j := i + 1
			for j < end && isDigit(l.src[j]) {
				j++
			}
			num := string(l.src[i+1 : j])
			return i, l.syntax(start, j, strconv.Quote(member+"."+num)+" is not a member",
				"write item "+num+" of "+member,
				source.HelpText("write item "+num+" of "+member))
		}
		return i, l.syntax(start, i+1, "bad member", "")
	}
	if i < end && l.src[i] == '_' {
		return i, l.syntax(i, i+1, "names have no underscores", "")
	}
	return i, l.atom(kind, start, i)
}

// number lexes a number or duration starting at i (a digit or "-").
func (l *lexer) number(i, end int) (int, *source.Diagnostic) {
	start := i
	if l.src[i] == '-' {
		i++
		if i >= end || !isDigit(l.src[i]) {
			return start, l.syntax(start, start+1, `"-" must be followed by a digit`, "subtract with minus",
				source.HelpText(`write "minus" to subtract`))
		}
	}
	for i < end && isDigit(l.src[i]) {
		i++
	}
	if i+1 < end && l.src[i] == '.' && isDigit(l.src[i+1]) {
		i++
		for i < end && isDigit(l.src[i]) {
			i++
		}
	}
	kind := NUMBER
	switch {
	case i+1 < end && l.src[i] == 'm' && l.src[i+1] == 's':
		kind, i = DURATION, i+2
	case i < end && l.src[i] == 's':
		kind, i = DURATION, i+1
	}
	if i < end && (isAlnum(l.src[i]) || l.src[i] == '.' || l.src[i] == '_') {
		j := i
		for j < end && (isAlnum(l.src[j]) || l.src[j] == '.' || l.src[j] == '_') {
			j++
		}
		return start, l.syntax(start, j, "bad number", "")
	}
	return i, l.atom(kind, start, i)
}

// text lexes a text literal starting at the quote at i.
func (l *lexer) text(i, end int) (int, *source.Diagnostic) {
	start := i
	for j := i + 1; ; {
		if j >= end {
			return start, l.unterminated(start, end)
		}
		switch c := l.src[j]; {
		case c == '"':
			return j + 1, l.atom(TEXT, start, j+1)
		case c == '\\':
			if j+1 >= end {
				return start, l.unterminated(start, end)
			}
			if e := l.src[j+1]; e != '"' && e != '\\' && e != 'n' {
				_, n := utf8.DecodeRune(l.src[j+1 : end])
				return j, l.syntax(j, j+1+n, "unknown escape", `escapes are \" \\ \n`,
					source.HelpText(`write one of \" \\ \n`))
			}
			j += 2
		case c < 0x20 || c == 0x7f:
			return j, l.syntax(j, j+1, "control character in text", "")
		default:
			j++
		}
	}
}

// path lexes the import path after the word "use", whose token has just been
// emitted and ends at i.
func (l *lexer) path(i, end int) (int, *source.Diagnostic) {
	j := i
	for j < end && l.src[j] == ' ' {
		j++
	}
	if j < end && l.src[j] == '\t' {
		return j, l.syntax(j, j+1, "tabs are not allowed", "")
	}
	if j == i && i < end {
		return i, nil // not separated from "use"; the main loop reports it
	}
	k := j
	for k < end && (isAlnum(l.src[k]) || l.src[k] == '.' || l.src[k] == '/' || l.src[k] == '@' || l.src[k] == '-') {
		k++
	}
	if k == j {
		return j, l.syntax(j, j, "missing import path after use", "", source.WithLabel("path expected here"))
	}
	if !validPath(string(l.src[j:k])) {
		return j, l.syntax(j, k, "bad import path", "")
	}
	return k, l.atom(PATH, j, k)
}

// validPath checks an import path against spec/01 section 1.9:
//
//	path    = segment { "/" segment } [ "@" version ]
//	segment = ( letter | digit ) { letter | digit | "." | "-" }
//	version = "v" digits "." digits "." digits
func validPath(p string) bool {
	at := len(p)
	for i := 0; i < len(p); i++ {
		if p[i] == '@' {
			at = i
			break
		}
	}
	if at < len(p) && !validVersion(p[at+1:]) {
		return false
	}
	seg := 0 // length of the current segment
	for i := 0; i < at; i++ {
		switch c := p[i]; {
		case c == '/':
			if seg == 0 {
				return false
			}
			seg = 0
		case isAlnum(c):
			seg++
		case (c == '.' || c == '-') && seg > 0:
			seg++
		default:
			return false
		}
	}
	return seg > 0
}

func validVersion(v string) bool {
	if len(v) == 0 || v[0] != 'v' {
		return false
	}
	parts := 0
	n := 0 // digits in the current part
	for i := 1; i < len(v); i++ {
		switch {
		case isDigit(v[i]):
			n++
		case v[i] == '.' && n > 0:
			parts++
			n = 0
		default:
			return false
		}
	}
	return parts == 2 && n > 0
}
