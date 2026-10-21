package format

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/synodlang/compiler/internal/parse"
	"github.com/synodlang/compiler/internal/source"
)

// TestExamplesRoundTrip: every vendored example is canonical already.
func TestExamplesRoundTrip(t *testing.T) {
	walkSynod(t, filepath.Join(testdata, "examples"), func(path string, src []byte) {
		f := source.NewFile(path, src)
		out, d := Format(f)
		if d != nil {
			t.Fatalf("%s: %s", path, d)
		}
		if !bytes.Equal(out, src) {
			t.Errorf("%s: Format changed the file:\n%s", path, out)
		}
		if d := Check(f); d != nil {
			t.Errorf("%s: Check: %s", path, d)
		}
		// emit alone, without the rewrites, also reproduces the file.
		tree, d := parse.Parse(f)
		if d != nil {
			t.Fatal(d)
		}
		if got := emit(tree); !bytes.Equal(got, src) {
			t.Errorf("%s: emit changed the file", path)
		}
	})
}

// TestTwopcNegationUntouched: not (a and b) is not a comparison.
func TestTwopcNegationUntouched(t *testing.T) {
	src := mustRead(t, filepath.Join(testdata, "examples", "twopc", "twopc.synod"))
	if !strings.Contains(string(src), "not (committed and aborted)") {
		t.Fatal("example no longer has the negation")
	}
	if out := must(t, "twopc.synod", src); !strings.Contains(string(out), "not (committed and aborted)") {
		t.Error("negation of a conjunction was rewritten")
	}
}
