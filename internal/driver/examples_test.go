package driver

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/synodlang/compiler/internal/load"
	"github.com/synodlang/compiler/internal/source"
)

// t4Codes are the codes the checker owns; TestResolveExamples ignores them.
var t4Codes = map[source.Code]bool{
	source.TypeMismatch: true, source.AmbiguousEmpty: true, source.MissingResult: true,
	source.ValueAsStatement: true, source.ActionAsValue: true, source.ForeignState: true,
}

func slash(p string) string { return filepath.ToSlash(p) }

func checkDir(t *testing.T, dir string, withStd bool) Result {
	t.Helper()
	abs, err := filepath.Abs(dir)
	if err != nil {
		t.Fatal(err)
	}
	abs = slash(abs)
	fsys := load.OSFS{}
	targets, err := load.Discover(fsys, abs)
	if err != nil {
		t.Fatal(err)
	}
	_ = os.Stdout
	return Check(Options{Load: load.Config{FS: fsys, Display: func(p string) string {
		if len(p) > len(abs) && p[:len(abs)] == abs {
			return p[len(abs)+1:]
		}
		return p
	}}, Tokens: true}, targets)
}

// TestResolveExamples resolves every example, and through them the std
// packages they use, with no diagnostics from the resolver's codes.
func TestResolveExamples(t *testing.T) {
	res := checkDir(t, "../../testdata/examples", true)
	for _, d := range res.Diags {
		if !t4Codes[d.Code] {
			t.Errorf("%s", d)
		}
	}
	if len(res.Packages) < 20 {
		t.Errorf("only %d packages resolved", len(res.Packages))
	}
}
