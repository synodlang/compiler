package trace_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/synodlang/compiler/internal/driver"
	"github.com/synodlang/compiler/internal/ir"
	"github.com/synodlang/compiler/internal/load"
)

type programRef struct {
	prog  *ir.Program
	extra []*ir.Package
}

func emptyProg() *ir.Program { return &ir.Program{} }

// compile checks the module below root and returns its main programs by
// import path (without "#main").
func compile(t *testing.T, root string) map[string]*programRef {
	t.Helper()
	abs, err := filepath.Abs(root)
	if err != nil {
		t.Fatal(err)
	}
	abs = filepath.ToSlash(abs)
	targets, err := load.Discover(load.OSFS{}, abs)
	if err != nil {
		t.Fatal(err)
	}
	res := driver.Check(driver.Options{}, targets)
	if len(res.Diags) > 0 {
		t.Fatalf("diagnostics in %s: %v", root, res.Diags[0])
	}
	out := map[string]*programRef{}
	for k, p := range res.Programs {
		if strings.HasSuffix(k, "#main") {
			out[strings.TrimSuffix(k, "#main")] = &programRef{prog: p}
		}
	}
	return out
}
