package driver

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestNoPanicOnTestdata resolves every .synod file under testdata as a
// library and as a main file; none may panic and the result is stable.
func TestNoPanicOnTestdata(t *testing.T) {
	filepath.Walk("../../testdata", func(p string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(p, ".synod") {
			return nil
		}
		b, _ := os.ReadFile(p)
		src := string(b)
		for _, name := range []string{"a/a.synod", "b/main.synod"} {
			r1 := checkMap(map[string]string{name: src}, filepath.Dir(name))
			r2 := checkMap(map[string]string{name: src}, filepath.Dir(name))
			if render(r1.Diags) != render(r2.Diags) {
				t.Errorf("%s: not deterministic", p)
			}
		}
		return nil
	})
}
