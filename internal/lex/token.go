// Package lex turns Synod source text into tokens (spec/01).
//
// The lexer is line oriented. It produces Python-style INDENT and DEDENT
// tokens, keeps comments as tokens so that the formatter can preserve them,
// and stops at the first error.
package lex

import "github.com/synodlang/compiler/internal/source"

// Kind is the kind of a token.
type Kind uint8

// Token kinds. Reserved words, true and false are ordinary WORD tokens.
const (
	EOF      Kind = iota
	NEWLINE       // end of a code line or of an own-line comment
	INDENT        // a code line is indented one level deeper than the previous one
	DEDENT        // one per level closed
	COMMENT       // "# text" or "#"
	WORD          // name
	MEMBER        // a.b, a.b.c: words joined by dots, one token
	NUMBER        // 12, -3, 0.5
	DURATION      // 5ms, 1.5s
	TEXT          // "..." including the quotes
	COLOR         // #F4A340
	PATH          // import path after "use"
	LPAREN
	RPAREN
	LBRACE
	RBRACE
	LBRACKET
	RBRACKET
	COLON
	COMMA
)

var kindNames = [...]string{
	EOF: "EOF", NEWLINE: "NEWLINE", INDENT: "INDENT", DEDENT: "DEDENT",
	COMMENT: "COMMENT", WORD: "WORD", MEMBER: "MEMBER", NUMBER: "NUMBER",
	DURATION: "DURATION", TEXT: "TEXT", COLOR: "COLOR", PATH: "PATH",
	LPAREN: "LPAREN", RPAREN: "RPAREN", LBRACE: "LBRACE", RBRACE: "RBRACE",
	LBRACKET: "LBRACKET", RBRACKET: "RBRACKET", COLON: "COLON", COMMA: "COMMA",
}

// String returns the upper-case name of k, such as "WORD".
func (k Kind) String() string {
	if int(k) < len(kindNames) {
		return kindNames[k]
	}
	return "KIND?"
}

// isAtom reports whether k is a kind that two of which must not touch.
func (k Kind) isAtom() bool {
	switch k {
	case WORD, MEMBER, NUMBER, DURATION, TEXT, COLOR, PATH:
		return true
	}
	return false
}

// Token is one lexical token.
//
// INDENT and DEDENT sit at column 1 of the line that causes them, NEWLINE
// at the end of its line, and the DEDENTs before EOF at the end of the file.
type Token struct {
	Kind Kind
	// Text is the exact source text. For COMMENT it is the whole "#..."
	// with trailing spaces and tabs removed. It is empty for NEWLINE,
	// INDENT, DEDENT and EOF.
	Text string
	Pos  source.Pos
	Off  int // byte offset of the start of the token
	// Blank is set on the first token of a source line (the COMMENT of a
	// comment line, otherwise the first token after any INDENT or DEDENT)
	// when at least one blank line lies directly above that line.
	Blank bool
	// Own is set on a COMMENT that stands alone on its line.
	Own bool
}
