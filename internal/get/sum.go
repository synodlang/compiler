package get

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// sumLine is one line of synod.sum.
type sumLine struct {
	Module, Version, Commit, Hash string
}

var (
	commitRe = regexp.MustCompile(`^[0-9a-f]{40}$`)
	hashRe   = regexp.MustCompile(`^h1:[A-Za-z0-9+/]{43}=$`)
)

func (l sumLine) String() string {
	return l.Module + " " + l.Version + " " + l.Commit + " " + l.Hash
}

// parseSum reads synod.sum: one "<module> <version> <commit> h1:<base64>"
// per line.
func parseSum(data []byte) (map[string]sumLine, error) {
	out := map[string]sumLine{}
	for i, line := range strings.Split(string(data), "\n") {
		line = strings.TrimRight(line, "\r")
		if strings.TrimSpace(line) == "" {
			continue
		}
		f := strings.Fields(line)
		if len(f) != 4 || !commitRe.MatchString(f[2]) || !hashRe.MatchString(f[3]) {
			return nil, fmt.Errorf("synod.sum line %d: want \"<module> <version> <commit> h1:<hash>\"", i+1)
		}
		if _, dup := out[f[0]]; dup {
			return nil, fmt.Errorf("synod.sum line %d: %s appears twice", i+1, f[0])
		}
		out[f[0]] = sumLine{f[0], f[1], f[2], f[3]}
	}
	return out, nil
}

// formatSum renders lines sorted by module path.
func formatSum(lines []sumLine) []byte {
	sort.Slice(lines, func(i, j int) bool { return lines[i].Module < lines[j].Module })
	var b strings.Builder
	for _, l := range lines {
		b.WriteString(l.String())
		b.WriteByte('\n')
	}
	return []byte(b.String())
}
