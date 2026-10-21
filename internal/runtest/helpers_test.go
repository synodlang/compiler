package runtest

import (
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
	"github.com/synodlang/compiler/internal/scene"
	"github.com/synodlang/compiler/internal/sim"
	"github.com/synodlang/compiler/internal/trace"
)

var update = flag.Bool("update", false, "rewrite the golden files of testdata/run/examples")

// examplesDir is the vendored copy of the examples module.
const examplesDir = "../../testdata/examples"

// program is a checked main program with its graph.
type program struct {
	Name  string
	IPath string
	Prog  *ir.Program
	Look  *ir.Program // the default look, nil when Prog brings its own
	Graph *load.Graph
}

// check checks the module below root (a path relative to the package or
// absolute) with the default display paths and returns its main programs by
// name (the last segment of the import path).
func check(t testing.TB, root string) map[string]*program {
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
	res := driver.Check(driver.Options{DefaultLook: true}, targets)
	if len(res.Diags) > 0 {
		var b strings.Builder
		for _, d := range res.Diags {
			b.WriteString(d.String() + "\n")
		}
		t.Fatalf("diagnostics in %s:\n%s", root, b.String())
	}
	out := map[string]*program{}
	for k, p := range res.Programs {
		if !strings.HasSuffix(k, "#main") {
			continue
		}
		ip := strings.TrimSuffix(k, "#main")
		name := ip[strings.LastIndex(ip, "/")+1:]
		pr := &program{Name: name, IPath: ip, Prog: p, Graph: res.Graph}
		if !scene.UsesLook(p) {
			pr.Look = res.Programs[driver.DefaultLookPath]
		}
		out[name] = pr
	}
	return out
}

var (
	exOnce sync.Once
	exProg map[string]*program
)

// examples returns the nine main programs of testdata/examples, checked once.
func examples(t testing.TB) map[string]*program {
	t.Helper()
	exOnce.Do(func() { exProg = check(t, examplesDir) })
	if exProg == nil {
		t.Fatal("examples did not check")
	}
	return exProg
}

// names returns the sorted keys of m.
func names(m map[string]*program) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// run runs p with a seed override (nil keeps the program's).
func (p *program) run(seed *uint64) *sim.Result {
	return sim.Run(p.Prog, sim.Options{Seed: seed})
}

func u64(n uint64) *uint64 { return &n }

// opts returns the trace options of p.
func (p *program) opts() trace.Options {
	return trace.Options{Paths: trace.FilePaths(p.Graph), Qual: trace.Qualifiers(p.Prog)}
}

// encode renders the result as trace.jsonl.
func (p *program) encode(t testing.TB, r *sim.Result) []byte {
	t.Helper()
	var b strings.Builder
	if err := trace.Encode(&b, r.Header, r.Facts, p.opts()); err != nil {
		t.Fatal(err)
	}
	return []byte(b.String())
}

// scene builds the scene of the facts at the default pace and mode.
func (p *program) scene(t testing.TB, facts []trace.Fact) (*trace.Scene, *scene.Failure) {
	t.Helper()
	return p.sceneWith(t, facts, scene.Options{})
}

func (p *program) sceneWith(t testing.TB, facts []trace.Fact, o scene.Options) (*trace.Scene, *scene.Failure) {
	t.Helper()
	return scene.Build(scene.Input{Prog: p.Prog, Look: p.Look, Facts: facts, Trace: p.opts()}, o)
}

// encodeScene renders a scene as scene.json.
func encodeScene(t testing.TB, sc *trace.Scene) []byte {
	t.Helper()
	var b strings.Builder
	if err := trace.EncodeScene(&b, sc); err != nil {
		t.Fatal(err)
	}
	return []byte(b.String())
}

// sceneBytes runs the program with the seed and returns its scene.json; the
// run and the look must not fail.
func (p *program) sceneBytes(t testing.TB, seed *uint64) []byte {
	t.Helper()
	r := p.run(seed)
	if r.Failed != nil {
		t.Fatalf("failed: %v", r.Failed)
	}
	sc, fl := p.scene(t, r.Facts)
	if fl != nil {
		t.Fatalf("look failed at fact %d: %s", fl.Seq, fl.Fault.Error())
	}
	return encodeScene(t, sc)
}

// summary renders the --summary text of the result.
func (p *program) summary(t testing.TB, r *sim.Result) string {
	t.Helper()
	var b strings.Builder
	in := trace.SummaryInput{
		Header: r.Header, Facts: r.Facts, Steps: r.Steps, Final: r.Final,
		Invariants: r.Invariants, EndReason: r.EndReason, End: r.End,
		Failed: r.Failed, TracePath: "out/trace.jsonl",
	}
	if err := trace.WriteSummary(&b, in, p.opts()); err != nil {
		t.Fatal(err)
	}
	return b.String()
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
