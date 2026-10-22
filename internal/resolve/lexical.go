package resolve

import (
	"strings"
	"unicode/utf8"

	"github.com/synodlang/compiler/internal/lex"
	"github.com/synodlang/compiler/internal/source"
)

// LexicalTokens returns the tokens of a file that could not be resolved
// (decision 79): all of them, or those before its first lexical error. Only
// the roles the lexer can tell are used: language for reserved words, value
// for numbers, durations, texts, colors, true and false, comment, and name
// for every other word, part of a member and path. They have no Def.
func LexicalTokens(f *source.File) []Token {
	toks, _ := lex.LexPartial(f)
	out := []Token{}
	add := func(pos source.Pos, n int, role Role) {
		out = append(out, Token{Line: pos.Line, Col: pos.Col, Len: n, Role: role})
	}
	for _, t := range toks {
		n := utf8.RuneCountInString(t.Text)
		switch t.Kind {
		case lex.COMMENT:
			add(t.Pos, n, RoleComment)
		case lex.WORD:
			role := RoleName
			switch {
			case t.Text == "true" || t.Text == "false":
				role = RoleValue
			case reserved[t.Text]:
				role = RoleLanguage
			}
			add(t.Pos, n, role)
		case lex.MEMBER:
			col := t.Pos.Col
			for _, p := range strings.Split(t.Text, ".") {
				add(source.Pos{Line: t.Pos.Line, Col: col}, utf8.RuneCountInString(p), RoleName)
				col += utf8.RuneCountInString(p) + 1
			}
		case lex.NUMBER, lex.DURATION, lex.TEXT, lex.COLOR:
			add(t.Pos, n, RoleValue)
		case lex.PATH:
			add(t.Pos, n, RoleName)
		}
	}
	return out
}
