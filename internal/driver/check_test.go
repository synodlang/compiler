package driver

import (
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/synodlang/compiler/internal/ir"
	"github.com/synodlang/compiler/internal/stdlib"
	"github.com/synodlang/compiler/internal/types"
)

// TestExamplesClean checks every example with no diagnostics at all.
func TestExamplesClean(t *testing.T) {
	res := checkDir(t, "../../testdata/examples", true)
	for _, d := range res.Diags {
		t.Errorf("%s", d)
	}
	if len(res.Programs) == 0 && len(res.Diags) == 0 {
		t.Errorf("no programs lowered")
	}
}

// TestStdClean checks every std package with no diagnostics: a module that
// imports them all is checked, and the std files are part of the result.
func TestStdClean(t *testing.T) {
	var pkgs []string
	root := stdlib.Root()
	err := fs.WalkDir(root, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(p, ".synod") {
			return err
		}
		dir := path.Dir(p)
		if len(pkgs) == 0 || pkgs[len(pkgs)-1] != dir {
			pkgs = append(pkgs, dir)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(pkgs)
	uses := ""
	for _, p := range pkgs {
		uses += "use std/" + p + "\n"
	}
	res := checkMap(map[string]string{"a/a.synod": "# package a\n" + uses + "\ndefine One\n  result 1\n"}, "a")
	for _, d := range res.Diags {
		if strings.HasPrefix(d.File, "std/") {
			t.Errorf("%s", d)
		}
	}
	n := 0
	for _, p := range res.Packages {
		if p.IsStd {
			n++
		}
	}
	if n != len(pkgs) {
		t.Errorf("%d std packages checked, %d in std", n, len(pkgs))
	}
}

func golden(t *testing.T, path, got string) {
	t.Helper()
	if *update {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(want) != got {
		t.Errorf("%s differs (run with -update to rewrite)\n--- got\n%s--- want\n%s", path, got, want)
	}
}

// TestIRGolden dumps the programs of five examples.
func TestIRGolden(t *testing.T) {
	res := checkDir(t, "../../testdata/examples", true)
	if len(res.Diags) > 0 {
		t.Fatalf("%s", res.Diags[0])
	}
	const base = "github.com/synodlang/examples/"
	for _, c := range []struct{ name, key string }{
		{"tcp", base + "tcp#main"},
		{"paxos", base + "paxos#main"},
		{"cassandra", base + "cassandra#main"},
		{"raftlook", base + "raftlook"},
		{"webapp", base + "webapp#main"},
	} {
		p := res.Programs[c.key]
		if p == nil {
			t.Fatalf("no program %s", c.key)
		}
		golden(t, "../../testdata/ir/"+c.name+".txt", ir.Dump(p))
		// The dump is the same on a second run.
		again := checkDir(t, "../../testdata/examples", true)
		if ir.Dump(again.Programs[c.key]) != ir.Dump(p) {
			t.Errorf("%s: dump is not deterministic", c.name)
		}
	}
}

// TestInferredTypes lists the inferred type of every node field and record
// field of the examples.
func TestInferredTypes(t *testing.T) {
	res := checkDir(t, "../../testdata/examples", true)
	var b strings.Builder
	for _, rp := range res.Packages {
		if rp.IsStd || rp.IsMain {
			continue
		}
		in := res.Infos[rp.Path]
		if rp.Node != nil {
			for _, f := range rp.Node.Fields {
				b.WriteString(rp.Name + "." + f.Name + ": " + types.Format(in.FieldTy[f], rp.Path, nil) + "\n")
			}
		}
		for _, r := range rp.Records {
			for _, f := range r.Fields {
				b.WriteString(rp.Name + "." + r.Name + "." + f.Name + ": " + types.Format(in.FieldTy[f], rp.Path, nil) + "\n")
			}
		}
	}
	golden(t, "../../testdata/ir/types.txt", b.String())
}
