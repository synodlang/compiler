package sim

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/synodlang/compiler/internal/driver"
	"github.com/synodlang/compiler/internal/ir"
	"github.com/synodlang/compiler/internal/load"
	"github.com/synodlang/compiler/internal/trace"
	"github.com/synodlang/compiler/internal/value"
)

var update = flag.Bool("update", false, "rewrite the golden files of testdata/run")

func slash(p string) string { return filepath.ToSlash(p) }

// compileDir checks the module in dir (relative to the package) and returns
// its main program.
func compileDir(t testing.TB, dir string) *ir.Program {
	p, _ := compileGraph(t, dir)
	return p
}

func compileGraph(t testing.TB, dir string) (*ir.Program, *load.Graph) {
	progs, g := compileAll(t, dir)
	if len(progs) != 1 {
		t.Fatalf("%d main programs in %s", len(progs), dir)
	}
	for _, p := range progs {
		return p, g
	}
	return nil, nil
}

// compileAll checks the module in dir and returns its main programs by
// import path.
func compileAll(t testing.TB, dir string) (map[string]*ir.Program, *load.Graph) {
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
	res := driver.Check(driver.Options{Load: load.Config{FS: fsys, Display: func(p string) string {
		if len(p) > len(abs) && p[:len(abs)] == abs {
			return p[len(abs)+1:]
		}
		return p
	}}}, targets)
	if len(res.Diags) > 0 {
		var b strings.Builder
		for _, d := range res.Diags {
			b.WriteString(d.String() + "\n")
		}
		t.Fatalf("diagnostics in %s:\n%s", dir, b.String())
	}
	progs := map[string]*ir.Program{}
	for k, p := range res.Programs {
		if strings.HasSuffix(k, "#main") {
			progs[k] = p
		}
	}
	return progs, res.Graph
}

// compileSrc builds a module in a temp dir from a main file and a library
// file (package p) and compiles it.
func compileSrc(t testing.TB, mainSrc, libSrc string) *ir.Program {
	t.Helper()
	dir := t.TempDir()
	write := func(name, s string) {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("synod.mod", "module example.com/t\nsynod 0.2\n")
	write("p/main.synod", "# package main\n"+mainSrc+"\n")
	write("p/p.synod", "# package p\n"+libSrc+"\n")
	return compileDir(t, dir)
}

// dump renders the facts of a result as text, one line per fact. It is the
// test-only reference form used for the goldens of testdata/run.
func dump(r *Result) string {
	var b strings.Builder
	f := value.FormatOpts{ShowUnexported: true}
	node := func(id int) string { return value.NodeName(id) }
	names := func(ids []int) string {
		var s []string
		for _, i := range ids {
			s = append(s, node(i))
		}
		return "[" + strings.Join(s, ",") + "]"
	}
	rec := func(x *value.Record) string { return value.Format(value.RecordOf(x.Type, x.Fields), f) }
	for i, x := range r.Facts {
		fmt.Fprintf(&b, "%d s%d @%s %s", i+1, x.Step, value.FormatTime(x.T), x.Ev)
		if x.Node >= 0 {
			fmt.Fprintf(&b, " %s", node(x.Node))
		}
		switch x.Ev {
		case "net.Sent", "net.Arrived", "net.Dropped":
			fmt.Fprintf(&b, " #%d %s %s->%s", x.MsgID, rec(x.Msg), node(x.From), node(x.To))
			if x.Reason != "" {
				fmt.Fprintf(&b, " %s", x.Reason)
			}
		case "sim.Added", "sim.Restarted":
			var ps []string
			for _, p := range x.Runs {
				ps = append(ps, p.Path)
			}
			var st []string
			for _, v := range x.State {
				st = append(st, value.Format(v, f))
			}
			fmt.Fprintf(&b, " runs %s state %s", strings.Join(ps, ","), strings.Join(st, ";"))
		case "sim.Changed":
			fmt.Fprintf(&b, " %s.%s %s -> %s", x.Field.Pkg.Name, x.Field.Name, value.Format(x.Old, f), value.Format(x.New, f))
		case "sim.Injected", "sim.Emitted":
			fmt.Fprintf(&b, " %s", rec(x.Record))
		case "sim.Broken":
			fmt.Fprintf(&b, " %s", x.Rule)
		case "sim.Failed":
			fmt.Fprintf(&b, " %s: %s", x.Fault.Code, x.Fault.Message)
			if x.Fault.HasPos {
				fmt.Fprintf(&b, " at %s:%d:%d", x.Fault.Pos.File, x.Fault.Pos.Line, x.Fault.Pos.Col)
			}
		case "sim.Ended":
			fmt.Fprintf(&b, " %s", x.Reason)
		case "net.Partitioned":
			fmt.Fprintf(&b, " %s %s", names(x.A), names(x.B))
		case "disk.Written":
			fmt.Fprintf(&b, " %q", x.Key)
		}
		b.WriteByte('\n')
	}
	return b.String()
}

// goldenDump is the dump of a run with a header line.
func goldenDump(r *Result) string {
	return fmt.Sprintf("seed %d program %s packages %s\n%s", r.Header.Seed, r.Header.Program, strings.Join(r.Header.Packages, ","), dump(r))
}

// encode renders the result as trace.jsonl.
func encode(t testing.TB, prog *ir.Program, g *load.Graph, r *Result) string {
	t.Helper()
	var b strings.Builder
	o := trace.Options{Paths: trace.FilePaths(g), Qual: trace.Qualifiers(prog)}
	if err := trace.Encode(&b, r.Header, r.Facts, o); err != nil {
		t.Fatal(err)
	}
	return b.String()
}

// checkGolden compares got with the file, or rewrites it under -update.
func checkGolden(t *testing.T, path, got string) {
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
		t.Errorf("%s differs\n--- got\n%s--- want\n%s", path, got, want)
	}
}

// has reports whether the dump contains the line fragment.
func lines(r *Result) []string { return strings.Split(strings.TrimSpace(dump(r)), "\n") }
