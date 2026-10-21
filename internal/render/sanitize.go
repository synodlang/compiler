package render

import (
	"strings"
	"unicode/utf8"
)

// sanitize replaces what must not reach a terminal in labels, notes and
// help text: C0 controls except tab, DEL, C1 controls, bidirectional
// controls and each byte of invalid UTF-8 become U+FFFD.
func sanitize(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); {
		r, n := utf8.DecodeRuneInString(s[i:])
		i += n
		if (r == utf8.RuneError && n == 1) || unsafe(r) {
			b.WriteRune('�')
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}
