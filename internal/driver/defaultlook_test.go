package driver

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/synodlang/compiler/internal/load"
)

// TestDefaultLook checks that DefaultLook adds a program for std/look/basic
// that shares its packages and records with the main programs, and that the
// programs themselves do not change.
func TestDefaultLook(t *testing.T) {
	abs, err := filepath.Abs("../../testdata/examples")
	if err != nil {
		t.Fatal(err)
	}
	abs = slash(abs)
	targets, err := load.Discover(load.OSFS{}, abs)
	if err != nil {
		t.Fatal(err)
	}
	plain := Check(Options{}, targets)
	with := Check(Options{DefaultLook: true}, targets)
	for _, r := range []Result{plain, with} {
		if len(r.Diags) > 0 {
			t.Fatal(r.Diags[0])
		}
	}
	if plain.Programs[DefaultLookPath] != nil {
		t.Fatal("basic lowered without DefaultLook")
	}
	basic := with.Programs[DefaultLookPath]
	if basic == nil {
		t.Fatalf("no program for %s", DefaultLookPath)
	}
	if len(basic.Observers) == 0 {
		t.Error("basic has no observers")
	}
	// Only std/look/basic is added, and every other program has the same
	// packages as without it.
	for k, p := range plain.Programs {
		q := with.Programs[k]
		if q == nil || len(q.Packages) != len(p.Packages) {
			t.Fatalf("program %s differs", k)
		}
		for i := range p.Packages {
			if p.Packages[i].Path != q.Packages[i].Path {
				t.Fatalf("program %s: package %d differs", k, i)
			}
		}
	}
	if len(with.Programs) != len(plain.Programs)+1 {
		t.Errorf("%d programs, want %d", len(with.Programs), len(plain.Programs)+1)
	}
	// Shared pointers: every package of basic is the very package of each
	// main program that runs it, records included.
	shared := 0
	for k, p := range with.Programs {
		if !strings.HasSuffix(k, "#main") {
			continue
		}
		for _, bp := range basic.Packages {
			for _, pp := range p.Packages {
				if pp.Path == bp.Path {
					shared++
					if pp != bp {
						t.Fatalf("%s: package %s is not shared", k, bp.Path)
					}
					for i, r := range bp.Records {
						if pp.Records[i] != r {
							t.Fatalf("%s: record %s is not shared", k, r.Name)
						}
					}
				}
			}
		}
	}
	if shared == 0 {
		t.Error("no package shared")
	}
}
