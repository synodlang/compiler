package doc

import (
	"io/fs"
	"sort"
	"strings"

	"github.com/synodlang/compiler/internal/stdlib"
)

// StdPackages returns the import paths of the std packages ("std/net",
// "std/look/basic") in byte order.
func StdPackages() []string {
	root := stdlib.Root()
	has := map[string]bool{}
	_ = fs.WalkDir(root, ".", func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() && strings.HasSuffix(p, ".synod") {
			dir := "."
			if i := strings.LastIndexByte(p, '/'); i >= 0 {
				dir = p[:i]
			}
			has["std/"+dir] = true
		}
		return nil
	})
	out := make([]string, 0, len(has))
	for p := range has {
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}
