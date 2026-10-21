package format

import (
	"bytes"
	"path/filepath"
	"testing"

	"github.com/synodlang/compiler/internal/source"
)

// TestMisformatted: each hand-damaged copy in testdata/misformatted formats
// to the vendored example at the same relative path.
func TestMisformatted(t *testing.T) {
	root := filepath.Join(testdata, "misformatted")
	n := 0
	walkSynod(t, root, func(path string, src []byte) {
		n++
		rel, err := filepath.Rel(root, path)
		if err != nil {
			t.Fatal(err)
		}
		want := mustRead(t, filepath.Join(testdata, "examples", rel))
		if got := must(t, path, src); !bytes.Equal(got, want) {
			t.Errorf("%s formats to\n%s", rel, got)
		}
		if Check(source.NewFile(path, src)) == nil {
			t.Errorf("%s: Check accepts it", rel)
		}
	})
	if n < 4 {
		t.Errorf("%d misformatted files, want at least 4", n)
	}
}
