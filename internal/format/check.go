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
func Check(f *source.File) *source.Diagnostic {
	out, d := Format(f)
	if d != nil {
		return d
	}
	if bytes.Equal(out, f.Src) {
		return nil
	}
	mk := func(line, col int, msg, hint string) *source.Diagnostic {
		d := source.New(f, source.Pos{Line: line, Col: col}, source.NonCanonical, msg, hint)
		return &d
	}
	if bytes.HasPrefix(f.Src, []byte("\xef\xbb\xbf")) {
		return mk(1, 1, "byte order mark at start of file", "")
	}
	s := strings.Split(string(f.Src), "\n")
	c := strings.Split(string(out), "\n")
	write := func(i int) string {
		if i < len(c) && c[i] != "" {
			return `write "` + c[i] + `"`
		}
		return ""
	}
	for i := 0; ; i++ {
		switch {
		case i >= len(s) && i >= len(c):
			return mk(1, 1, "line is not canonical", "") // unreachable: the texts differ
		// Format prints one output line per source line and never invents
		// content, so past the end of one text the other has only the final
		// newline or blank lines left.
		case i >= len(s):
			last := s[len(s)-1]
			return mk(len(s), utf8.RuneCountInString(last)+1, "missing newline at end of file", "")
		case i >= len(c):
			return mk(i+1, 1, "unexpected blank line", "")
		case s[i] == c[i]:
			continue
		}
		si, ci := s[i], c[i]
		col := firstDiff(si, ci)
		switch {
		case si == ci+"\r":
			return mk(i+1, utf8.RuneCountInString(ci)+1, "line ends with CR LF", "use LF line endings")
		case strings.TrimRight(si, " \t") == ci:
			return mk(i+1, utf8.RuneCountInString(ci)+1, "trailing spaces", "")
		case strings.TrimSpace(si) == "" && i+1 < len(s) && s[i+1] == ci:
			return mk(i+1, 1, "unexpected blank line", "")
		case ci == "" && i+1 < len(c) && si == c[i+1]:
			return mk(i+1, 1, "missing blank line", "")
		case strings.TrimSpace(si) != "" && i+1 < len(s) && equalLines(s[i+1:], c[i:]):
			return mk(i+1, 1, "unexpected line", "")
		}
		return mk(i+1, col, "line is not canonical", write(i))
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
