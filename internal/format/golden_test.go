package format

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/synodlang/compiler/internal/source"
)

// TestGoldens formats testdata/format/<name>.in.synod and compares with
// <name>.out.synod. Every output is canonical, and formats to itself.
func TestGoldens(t *testing.T) {
	dir := filepath.Join(testdata, "format")
	n := 0
	walkSynod(t, dir, func(path string, src []byte) {
		if !strings.HasSuffix(path, ".in.synod") {
			return
		}
		n++
		name := strings.TrimSuffix(filepath.Base(path), ".in.synod")
		t.Run(name, func(t *testing.T) {
			out := must(t, path, src)
			golden(t, filepath.Join(dir, name+".out.synod"), out)
			if d := Check(source.NewFile(name, out)); d != nil {
				t.Errorf("output is not canonical: %s", d)
			}
			if again := must(t, name, out); string(again) != string(out) {
				t.Errorf("not idempotent:\n%s", again)
			}
		})
	})
	if n < 15 {
		t.Errorf("only %d format goldens", n)
	}
}
