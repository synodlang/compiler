package format

import (
	"bytes"
	"strings"
	"unicode/utf8"

	"github.com/synodlang/compiler/internal/source"
)

// Check reports the first way in which f differs from its canonical form as
// a non-canonical diagnostic, and returns nil when f is canonical. A file
// that does not parse yields its syntax or indent diagnostic instead.
func Check(f *source.File) *source.Diagnostic { return CheckWith(f, nil) }

// CheckWith is Check for the canonical form that FormatWith(f, fixes) gives.
// The messages are the same as Check's; when the fixes change a line, the
// hint is the canonical line.
func CheckWith(f *source.File, fixes []Fix) *source.Diagnostic {
	out, d := FormatWith(f, fixes)
	if d != nil {
		return d
	}
	if bytes.Equal(out, f.Src) {
		return nil
	}
	mk := func(sp source.Span, msg, hint string, more ...source.Extra) *source.Diagnostic {
		d := source.NewSpan(f, sp, source.NonCanonical, msg, hint, more...)
		return &d
	}
	pos := func(line, col int) source.Pos { return source.Pos{Line: line, Col: col} }
	// within returns the span on one line from column from to column to.
	within := func(line, from, to int) source.Span { return source.Span{Start: pos(line, from), End: pos(line, to)} }
	if bytes.HasPrefix(f.Src, []byte("\xef\xbb\xbf")) {
		return mk(source.At(pos(1, 1)), "byte order mark at start of file", "",
			source.HelpText("remove the byte order mark (synod fmt does this)"))
	}
	s := strings.Split(string(f.Src), "\n")
	c := strings.Split(string(out), "\n")
	write := func(i int) string {
		if i < len(c) && c[i] != "" {
			return `write "` + c[i] + `"`
		}
		return ""
	}
	// deleteLine is the edit that removes the whole line n.
	deleteLine := func(n int) source.Edit {
		return source.EditAt(source.Span{Start: pos(n, 1), End: pos(n+1, 1)}, "")
	}
	for i := 0; ; i++ {
		switch {
		case i >= len(s) && i >= len(c):
			return mk(source.At(pos(1, 1)), "line is not canonical", "") // unreachable: the texts differ
		// Format prints one output line per source line and never invents
		// content, so past the end of one text the other has only the final
		// newline or blank lines left.
		case i >= len(s):
			last := s[len(s)-1]
			end := pos(len(s), utf8.RuneCountInString(last)+1)
			return mk(source.At(end), "missing newline at end of file", "",
				source.HelpEdit("end the file with a newline (synod fmt does this)", source.EditAt(source.At(end), "\n")))
		case i >= len(c):
			// The extra lines are blank. When the one reported is the empty
			// line after the final newline, the blank line before it goes.
			n := i + 1
			if n == len(s) && i > 0 {
				n = i
			}
			sp := source.At(pos(i+1, 1))
			if w := utf8.RuneCountInString(s[i]); w > 0 {
				sp = within(i+1, 1, w+1)
			}
			return mk(sp, "unexpected blank line", "",
				source.HelpEdit("remove the blank line (synod fmt does this)", deleteLine(n)))
		case s[i] == c[i]:
			continue
		}
		si, ci := s[i], c[i]
		col := firstDiff(si, ci)
		line := i + 1
		switch {
		case si == ci+"\r":
			w := utf8.RuneCountInString(ci) + 1
			return mk(within(line, w, w+1), "line ends with CR LF", "use LF line endings",
				source.HelpText("use LF line endings (synod fmt does this)"))
		case strings.TrimRight(si, " \t") == ci:
			from, to := utf8.RuneCountInString(ci)+1, utf8.RuneCountInString(si)+1
			return mk(within(line, from, to), "trailing spaces", "",
				source.HelpEdit("remove the trailing spaces (synod fmt does this)", source.EditAt(within(line, from, to), "")))
		case strings.TrimSpace(si) == "" && i+1 < len(s) && s[i+1] == ci:
			sp := source.At(pos(line, 1))
			if w := utf8.RuneCountInString(si); w > 0 {
				sp = within(line, 1, w+1)
			}
			return mk(sp, "unexpected blank line", "",
				source.HelpEdit("remove the blank line (synod fmt does this)", deleteLine(line)))
		case ci == "" && i+1 < len(c) && si == c[i+1]:
			return mk(source.At(pos(line, 1)), "missing blank line", "",
				source.HelpEdit("add a blank line (synod fmt does this)", source.EditAt(source.At(pos(line, 1)), "\n")))
		case strings.TrimSpace(si) != "" && i+1 < len(s) && equalLines(s[i+1:], c[i:]):
			return mk(within(line, 1, utf8.RuneCountInString(si)+1), "unexpected line", "",
				source.HelpEdit("remove the line (synod fmt does this)", deleteLine(line)))
		}
		// The primary span is what differs: from the first difference to the
		// end of the line, less what the two lines share at the end.
		rs, rc := []rune(si), []rune(ci)
		pre := col - 1
		suf := 0
		for suf < len(rs)-pre && suf < len(rc)-pre && rs[len(rs)-1-suf] == rc[len(rc)-1-suf] {
			suf++
		}
		to := len(rs) - suf + 1
		if to == col && col <= len(rs) {
			to = col + 1 // the canonical line only adds text: mark the next character
		}
		extras := []source.Extra{}
		if ci != "" {
			extras = append(extras, source.HelpEdit("write the canonical line (synod fmt does this)",
				source.EditAt(within(line, 1, len(rs)+1), ci)))
		}
		return mk(within(line, col, to), "line is not canonical", write(i), extras...)
	}
}

func equalLines(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// firstDiff returns the 1-based code point column where a and b first differ.
func firstDiff(a, b string) int {
	col := 1
	for len(a) > 0 && len(b) > 0 {
		ra, na := utf8.DecodeRuneInString(a)
		rb, nb := utf8.DecodeRuneInString(b)
		if ra != rb {
			break
		}
		a, b = a[na:], b[nb:]
		col++
	}
	return col
}
