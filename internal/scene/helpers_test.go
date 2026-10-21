package scene

import (
	"bytes"
	"flag"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/synodlang/compiler/internal/driver"
	"github.com/synodlang/compiler/internal/ir"
	"github.com/synodlang/compiler/internal/load"
	"github.com/synodlang/compiler/internal/natives"
	"github.com/synodlang/compiler/internal/sim"
	"github.com/synodlang/compiler/internal/trace"
)

var update = flag.Bool("update", false, "rewrite the golden files of testdata/scene")

// examplesDir is the vendored copy of the examples module.
const examplesDir = "../../testdata/examples"

// prog is a checked main program with its default look and graph.
type prog struct {
	Name  string
	Prog  *ir.Program
	Look  *ir.Program
	Graph *load.Graph
}

func (p *prog) opts() trace.Options {
	return trace.Options{Paths: trace.FilePaths(p.Graph), Qual: trace.Qualifiers(p.Prog)}
}

func (p *prog) input(facts []trace.Fact) Input {
	in := Input{Prog: p.Prog, Facts: facts, Trace: p.opts()}
	if !UsesLook(p.Prog) {
		in.Look = p.Look
	}
	return in
}

// checkDir checks the module below root with the default look and returns its
// main programs by name.
func checkDir(t testing.TB, root string, display func(string) string) map[string]*prog {
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
	cfg := load.Config{}
	if display != nil {
		cfg.FS = load.OSFS{}
		cfg.Display = func(p string) string {
			if len(p) > len(abs) && p[:len(abs)] == abs {
				return p[len(abs)+1:]
			}
			return p
		}
	}
	res := driver.Check(driver.Options{Load: cfg, DefaultLook: true}, targets)
	if len(res.Diags) > 0 {
		var b strings.Builder
		for _, d := range res.Diags {
			b.WriteString(d.String() + "\n")
		}
		t.Fatalf("diagnostics in %s:\n%s", root, b.String())
	}
	out := map[string]*prog{}
	for k, p := range res.Programs {
		if !strings.HasSuffix(k, "#main") {
			continue
		}
		ip := strings.TrimSuffix(k, "#main")
		name := ip[strings.LastIndex(ip, "/")+1:]
		out[name] = &prog{Name: name, Prog: p, Look: res.Programs[driver.DefaultLookPath], Graph: res.Graph}
	}
	return out
}

var (
	exOnce sync.Once
	exProg map[string]*prog
)

// examples returns the nine main programs of testdata/examples, checked once.
func examples(t testing.TB) map[string]*prog {
	t.Helper()
	exOnce.Do(func() { exProg = checkDir(t, examplesDir, nil) })
	if exProg == nil {
		t.Fatal("examples did not check")
	}
	return exProg
}

func names(m map[string]*prog) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// encodeScene renders a scene as scene.json.
func encodeScene(t testing.TB, s *trace.Scene) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := trace.EncodeScene(&b, s); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

// golden compares got with the file, or rewrites it under -update.
func golden(t *testing.T, path string, got []byte) {
	t.Helper()
	if *update {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v (run with -update)", err)
	}
	if string(want) != string(got) {
		t.Errorf("%s differs (run with -update to rewrite)", path)
	}
}

// run simulates the program with its own seed.
func (p *prog) run() *sim.Result { return sim.Run(p.Prog, sim.Options{}) }

// nativesTable is the table of the shared natives.
var nativesTable = natives.Table
