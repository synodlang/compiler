package release

import (
	"fmt"
	"strings"
)

// Notes returns the body of the changelog section whose level-2 heading is
// "## <tag>", optionally followed by whitespace and more text (for example
// "## v0.1.0 — 2026-10-21"). The body runs to the next "## " heading and is
// trimmed of leading and trailing blank lines. It is an error if the section
// is missing or has no content.
func Notes(changelog, tag string) (string, error) {
	if tag == "" {
		return "", fmt.Errorf("empty tag")
	}
	lines := strings.Split(strings.ReplaceAll(changelog, "\r\n", "\n"), "\n")
	start := -1
	for i, l := range lines {
		if isHeading(l, tag) {
			start = i + 1
			break
		}
	}
	if start < 0 {
		return "", fmt.Errorf("no section %q in changelog", "## "+tag)
	}
	end := len(lines)
	for i := start; i < len(lines); i++ {
		if strings.HasPrefix(lines[i], "## ") {
			end = i
			break
		}
	}
	body := strings.Trim(strings.Join(lines[start:end], "\n"), "\n \t")
	if body == "" {
		return "", fmt.Errorf("section %q in changelog is empty", "## "+tag)
	}
	return body + "\n", nil
}

func isHeading(line, tag string) bool {
	rest, ok := strings.CutPrefix(strings.TrimRight(line, " \t"), "## "+tag)
	if !ok {
		return false
	}
	return rest == "" || rest[0] == ' ' || rest[0] == '\t'
}
