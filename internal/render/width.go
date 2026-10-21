package render

import (
	"unicode"
	"unicode/utf8"
)

// tabWidth is the number of columns a tab occupies in a snippet.
const tabWidth = 4

// cell is one code point (or one byte of invalid UTF-8) of a source line,
// ready to print: out is its sanitized text and w its display width.
type cell struct {
	out string
	w   int
}

// wide lists the code point ranges drawn two columns wide: East Asian wide
// and full-width characters and emoji. It is a small built-in approximation
// of Unicode's East Asian Width table, not the table itself.
var wide = [][2]rune{
	{0x1100, 0x115F},
	{0x2E80, 0x303E},
	{0x3040, 0xA4CF}, // 2E80-A4CF except U+303F
	{0xAC00, 0xD7A3},
	{0xF900, 0xFAFF},
	{0xFE30, 0xFE4F},
	{0xFF00, 0xFF60},
	{0xFFE0, 0xFFE6},
	{0x1F300, 0x1F64F},
	{0x1F900, 0x1F9FF},
	{0x20000, 0x3FFFD},
}

// runeWidth returns the display width of r: 0 for combining marks and format
// characters, 2 for wide characters, 4 for a tab, else 1.
func runeWidth(r rune) int {
	if r == '\t' {
		return tabWidth
	}
	if unicode.In(r, unicode.Mn, unicode.Me, unicode.Cf) {
		return 0
	}
	for _, rg := range wide {
		if r >= rg[0] && r <= rg[1] {
			return 2
		}
	}
	return 1
}

// bidi reports whether r is a bidirectional control character.
func bidi(r rune) bool {
	return (r >= 0x202A && r <= 0x202E) || (r >= 0x2066 && r <= 0x2069)
}

// unsafe reports whether r must not reach a terminal: C0 controls except
// tab, DEL, C1 controls and bidirectional controls.
func unsafe(r rune) bool {
	return (r < 0x20 && r != '\t') || r == 0x7F || (r >= 0x80 && r <= 0x9F) || bidi(r)
}

// cells splits s into cells, one per code point; each byte of invalid UTF-8
// is one cell of U+FFFD, in step with source.File.PosOf.
func cells(s string) []cell {
	out := make([]cell, 0, len(s))
	for i := 0; i < len(s); {
		r, n := utf8.DecodeRuneInString(s[i:])
		i += n
		switch {
		case r == utf8.RuneError && n == 1, unsafe(r):
			out = append(out, cell{"�", 1})
		case r == '\t':
			out = append(out, cell{"    ", tabWidth})
		default:
			out = append(out, cell{string(r), runeWidth(r)})
		}
	}
	return out
}

// textWidth returns the display width of s after sanitizing.
func textWidth(s string) int {
	w := 0
	for _, c := range cells(s) {
		w += c.w
	}
	return w
}
