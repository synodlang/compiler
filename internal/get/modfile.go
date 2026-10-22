package get

import (
	"sort"
	"strings"
)

// setRequire returns the text of a synod.mod with the require line for
// module m set to version v: other directives and comments stay in their
// order, and the require lines follow them, sorted by module path.
func setRequire(src []byte, m, v string) []byte {
	text := strings.TrimPrefix(string(src), "\xef\xbb\xbf")
	var kept []string
	reqs := map[string]string{}
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimRight(line, " \t\r")
		f := strings.Fields(line)
		if len(f) == 3 && f[0] == "require" {
			reqs[f[1]] = f[2]
			continue
		}
		kept = append(kept, line)
	}
	for len(kept) > 0 && kept[len(kept)-1] == "" {
		kept = kept[:len(kept)-1]
	}
	reqs[m] = v
	names := make([]string, 0, len(reqs))
	for n := range reqs {
		names = append(names, n)
	}
	sort.Strings(names)
	var b strings.Builder
	for _, l := range kept {
		b.WriteString(l)
		b.WriteByte('\n')
	}
	for _, n := range names {
		b.WriteString("require " + n + " " + reqs[n] + "\n")
	}
	return []byte(b.String())
}
