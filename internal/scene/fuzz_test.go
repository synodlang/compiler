package scene

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/synodlang/compiler/internal/driver"
	"github.com/synodlang/compiler/internal/load"
	"github.com/synodlang/compiler/internal/sim"
	"github.com/synodlang/compiler/internal/trace"
)

// lookGen writes a random look library ("fuzzlook") from a byte string: a
// few observers whose bodies are drawn from templates of well-typed look
// statements. Most programs check clean; some fault at run time on purpose
// (bad properties, missing attachments, messages with no id).
type lookGen struct {
	d    []byte
	i    int
	b    strings.Builder
	vars []string // shapes of the observer body being written
	used map[string]bool
	cnt  int
}

func (g *lookGen) n(k int) int {
	if len(g.d) == 0 {
		return 0
	}
	v := int(g.d[g.i%len(g.d)])
	g.i++
	if g.i%len(g.d) == 0 {
		v += g.i // vary a short input on later passes
	}
	return v % k
}

func (g *lookGen) pick(xs ...string) string { return xs[g.n(len(xs))] }

func (g *lookGen) line(indent int, f string, a ...any) {
	g.b.WriteString(strings.Repeat("  ", indent))
	fmt.Fprintf(&g.b, f, a...)
	g.b.WriteByte('\n')
}

// shapeVar returns a shape variable of the body, or "" when there is none; it
// marks it used.
func (g *lookGen) shapeVar() string {
	if len(g.vars) == 0 {
		return ""
	}
	v := g.vars[g.n(len(g.vars))]
	g.used[v] = true
	return v
}

var (
	genColors = []string{"theme.Paper", "theme.Ink", "theme.Line", "theme.Quiet", "theme.Accent", "theme.Good", "theme.Warning", "theme.Critical", "Hot"}
	genNums   = []string{"0", "1", "2", "10", "0.5", "300", "800", "1.15", "5000"}
	genDurs   = []string{"0s", "100ms", "300ms", "450ms", "1s", "2s"}
	genProps  = []string{"x", "y", "scale", "opacity", "rotation", "fill", "stroke", "text", "bogus"}
)

// env tells which variables an event binds.
type env struct {
	node string // a node variable, or ""
	msg  string // a message variable with an id, or ""
	from string // net.Sent and friends
	to   string
	text string // a text variable
}

// key returns a draw-for key expression.
func (g *lookGen) key(e env) string {
	switch g.n(4) {
	case 0:
		if e.node != "" {
			return e.node
		}
	case 1:
		return `"k"`
	case 2:
		return "3"
	}
	if e.node != "" {
		return e.node
	}
	return `"kk"`
}

// ref is a shape-typed expression: a body variable or an attachment lookup.
func (g *lookGen) ref(e env) string {
	if g.n(3) == 0 && e.node != "" {
		return "(card of " + e.node + ")"
	}
	if g.n(6) == 0 {
		return "look.Center"
	}
	if v := g.shapeVar(); v != "" {
		return v
	}
	return "look.Center"
}

func (g *lookGen) newVar() string {
	g.cnt++
	v := fmt.Sprintf("v%d", g.cnt)
	return v
}

// draw writes a draw statement and returns its variable.
func (g *lookGen) draw(ind int, e env) {
	var shape string
	switch g.n(8) {
	case 0:
		shape = "(look.Circle " + g.pick(genNums...) + ")"
	case 1:
		shape = "(look.Rect " + g.pick(genNums...) + " by " + g.pick(genNums...) + ")"
	case 2:
		shape = `(look.Label "hi")`
		if e.text != "" {
			shape = "(look.Label " + e.text + ")"
		}
	case 3:
		if e.node != "" {
			shape = "(look.Card for " + e.node + ")"
		} else {
			shape = "(look.Circle 5)"
		}
	case 4:
		if len(g.vars) >= 2 {
			shape = "(look.Line from " + g.shapeVar() + " to " + g.shapeVar() + ")"
		} else {
			shape = "(look.Rect 10 by 10)"
		}
	case 5:
		if len(g.vars) >= 1 {
			shape = "(look.Group {" + g.shapeVar() + ", " + g.shapeVar() + "})"
		} else {
			shape = "(look.Circle 2)"
		}
	case 6:
		shape = "look.Center"
	default:
		shape = "(look.Circle 7)"
	}
	v := g.newVar()
	forKey := ""
	if g.n(3) == 0 {
		forKey = " for " + g.key(e)
	}
	g.line(ind, "draw %s as %s%s", shape, v, forKey)
	g.vars = append(g.vars, v)
	if g.n(5) == 0 { // a reactive block
		g.line(ind+1, "look.Fill %s with %s", v, g.pick(genColors...))
		if e.node != "" {
			g.line(ind+1, "if (sim.IsUp %s)", e.node)
			g.line(ind+2, "look.Fade %s to %s", v, g.pick("0", "1", "0.5"))
		}
	}
}

func (g *lookGen) stmt(ind int, e env, depth int) {
	switch g.n(20) {
	case 0, 1, 2:
		g.draw(ind, e)
	case 3:
		if v := g.shapeVar(); v != "" {
			g.line(ind, "look.Fill %s with %s", v, g.pick(genColors...))
		}
	case 4:
		if v := g.shapeVar(); v != "" {
			g.line(ind, "look.Stroke %s with %s", v, g.pick(genColors...))
		}
	case 5, 6:
		if v := g.shapeVar(); v != "" {
			p := g.pick(genProps...)
			var to string
			switch p {
			case "fill", "stroke":
				to = g.pick(genColors...)
				if g.n(8) == 0 {
					to = "1"
				}
			case "text":
				to = `"txt"`
			case "x", "y":
				to = g.pick(genNums...)
				if g.n(3) == 0 {
					to = g.ref(e)
				}
			case "opacity":
				to = g.pick("0", "1", "0.5", "2")
			default:
				to = g.pick(genNums...)
			}
			g.line(ind, "look.Tween %q of %s to %s over %s", p, v, to, g.pick(genDurs...))
		}
	case 7:
		if v := g.shapeVar(); v != "" {
			g.line(ind, "look.Effect %q on %s", g.pick("glow", "indicate", "flash", "circumscribe", "ripple", "sweep", "blur", "trail", "bogus"), v)
		}
	case 8:
		if v := g.shapeVar(); v != "" {
			g.line(ind, "look.Place %s %s %s", v, g.pick("above", "below", "beside"), g.ref(e))
		}
	case 9:
		if v := g.shapeVar(); v != "" {
			g.line(ind, "look.Orbit %s around %s at %s", v, g.ref(e), g.pick("0", "0.25", "0.5", "1.5"))
		}
	case 10:
		if v := g.shapeVar(); v != "" && e.msg != "" {
			g.line(ind, "look.Flight %s from %s to %s for %s", v, g.ref(e), g.ref(e), e.msg)
		}
	case 11:
		if v := g.shapeVar(); v != "" {
			g.line(ind, "look.Remove %s", v)
		}
	case 12:
		if v := g.shapeVar(); v != "" {
			g.line(ind, "look.Pulse %s", v)
		}
	case 13:
		if v := g.shapeVar(); v != "" {
			g.line(ind, "look.Travel %s from %s to %s over %s", v, g.ref(e), g.ref(e), g.pick(genDurs...))
		}
	case 14:
		g.line(ind, "look.Layout %q", g.pick("ring", "row", "grid", "bad"))
	case 15, 16:
		if depth < 3 {
			g.line(ind, "after %s", g.pick(genDurs...))
			scope := len(g.vars)
			for k := 1 + g.n(3); k > 0; k-- {
				g.stmt(ind+1, e, depth+1)
			}
			// a block that draws must use what it draws, and is not empty
			for _, v := range g.vars[scope:] {
				if !g.used[v] {
					g.line(ind+1, "look.Fade %s to 1", v)
				}
			}
			g.vars = g.vars[:scope]
			if v := g.shapeVar(); v != "" {
				g.line(ind+1, "look.Fade %s to 1", v)
			} else {
				g.line(ind+1, "look.Layout \"ring\"")
			}
		}
	case 17:
		if e.node != "" {
			g.line(ind, "look.Fill (card of %s) with %s", e.node, g.pick(genColors...))
		}
	case 18:
		if e.node != "" {
			if v := g.shapeVar(); v != "" {
				g.line(ind, "look.Tween \"text\" of %s to (item 0 of (sim.StateRows %s)) over 100ms", v, e.node)
			}
		}
	case 19:
		if e.msg != "" {
			g.draw(ind, env{text: "(net.Type " + e.msg + ")"})
		}
	}
}

// observer writes one when block.
func (g *lookGen) observer() {
	var head string
	var e env
	switch g.n(11) {
	case 0:
		head = "sim.Started"
	case 1:
		head, e.node = "(sim.Added n)", "n"
	case 2:
		head, e.node = "(sim.Crashed n)", "n"
	case 3:
		head, e.node = "(sim.Restarted n)", "n"
	case 4:
		head, e.node = "(sim.Removed n)", "n"
	case 5:
		head, e.msg, e.from, e.to = "(net.Sent m from a to b)", "m", "a", "b"
	case 6:
		head, e.msg, e.from, e.to = "(net.Arrived m from a to b)", "m", "a", "b"
	case 7:
		head, e.msg, e.from, e.to = "(net.Dropped m from a to b because r)", "m", "a", "b"
		e.text = "r"
	case 8:
		head, e.node = "(sim.Emitted x by n)", "n"
	case 9:
		head, e.text = "(sim.Broken rule)", "rule"
	default:
		head, e.node = "(sim.Fired on n)", "n"
	}
	if e.from != "" {
		e.node = e.from
	}
	g.vars, g.used, g.cnt = nil, map[string]bool{}, 0
	// the shapes of the body are written first into a scratch buffer so that
	// unused variables can be given a use
	saved := g.b
	g.b = strings.Builder{}
	for k := 1 + g.n(6); k > 0; k-- {
		g.stmt(1, e, 0)
	}
	for _, v := range g.vars {
		if !g.used[v] {
			g.line(1, "look.Fade %s to 1", v)
		}
	}
	body := g.b.String()
	g.b = saved
	if body == "" {
		body = "  look.Layout \"ring\"\n"
	}
	g.b.WriteString("\nwhen " + head + "\n" + body)
}

// genLook returns the source of package fuzzlook.
func genLook(data []byte) string {
	g := &lookGen{d: data}
	basic := g.n(4) == 0
	imports := []string{"std/look", "std/net", "std/sim", "std/theme"}
	if basic {
		imports = append(imports, "std/look/basic")
	}
	sort.Strings(imports)
	g.b.WriteString("# package fuzzlook\n")
	for _, im := range imports {
		g.b.WriteString("use " + im + "\n")
	}
	g.b.WriteString("\ndefine Hot\n  result #FF6600\n")
	// uses every import
	g.b.WriteString("\nwhen net.Healed\n  draw (look.Circle 1) as keep\n  look.Fill keep with theme.Paper\n")
	g.b.WriteString("\nwhen sim.Started\n  look.Layout \"ring\"\n")
	for k := 1 + g.n(5); k > 0; k-- {
		g.observer()
	}
	return g.b.String()
}

// exampleFiles reads the sources of the examples module into a MapFS rooted
// at /m.
func exampleFiles(t testing.TB) load.MapFS {
	m := load.MapFS{}
	root := filepath.FromSlash(examplesDir)
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		if !strings.HasSuffix(p, ".synod") && filepath.Base(p) != "synod.mod" {
			return nil
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, p)
		m["/m/"+filepath.ToSlash(rel)] = string(b)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return m
}

// withUse adds "use path" to the imports of src, keeping them sorted.
func withUse(src, path string) string {
	lines := strings.Split(src, "\n")
	var at []int
	var uses []string
	for i, l := range lines {
		if strings.HasPrefix(l, "use ") {
			at = append(at, i)
			uses = append(uses, l)
		}
	}
	uses = append(uses, "use "+path)
	sort.Strings(uses)
	if len(at) == 0 {
		return src
	}
	out := append([]string(nil), lines[:at[0]]...)
	out = append(out, uses...)
	return strings.Join(append(out, lines[at[len(at)-1]+1:]...), "\n")
}

// scratch runs one fuzz case: the example dir with the look made from data.
// It reports whether the program checked and ran. Any failed invariant fails
// t; a panic is a bug.
func fuzzScene(t testing.TB, base load.MapFS, dir string, data []byte) (ran bool, diags string) {
	t.Helper()
	key := "/m/" + dir + "/demo.synod"
	src, ok := base[key]
	if !ok {
		return false, "no such example"
	}
	fsys := load.MapFS{}
	for k, v := range base {
		fsys[k] = v
	}
	fsys[key] = withUse(src, "github.com/synodlang/examples/fuzzlook")
	fsys["/m/fuzzlook/fuzzlook.synod"] = genLook(data)
	res := driver.Check(driver.Options{Load: load.Config{FS: fsys}, DefaultLook: true}, []string{"/m/" + dir, "/m/fuzzlook"})
	if len(res.Diags) > 0 || res.Graph == nil {
		var b strings.Builder
		for _, d := range res.Diags {
			b.WriteString(d.String() + "\n")
		}
		return false, b.String()
	}
	var ip string
	for _, p := range res.Graph.Targets {
		if p.IsMain {
			ip = p.Path
		}
	}
	main := res.Programs[ip]
	if main == nil {
		return false, "no main program"
	}
	p := &prog{Prog: main, Look: res.Programs[driver.DefaultLookPath], Graph: res.Graph}
	r := sim.Run(main, sim.Options{MaxSteps: 50_000, TimeLimit: 30_000_000})
	var o Options
	if len(data) > 1 {
		o.Pace = 250 + int(data[0])*15
		if data[1]&1 == 1 {
			o.Mode = Proportional
		}
	}
	o.MaxShapes, o.MaxKeys, o.MaxTimersPerInstant = 5000, 200_000, 500
	in := p.input(r.Facts)
	sc, fl := Build(in, o)
	checkScene(t, sc)
	a := encodeScene(t, sc)
	if fl != nil && !bytes.Contains(a, []byte(`"kind":"failed"`)) {
		t.Fatalf("look fault %v leaves no failed marker", fl.Fault)
	}
	checkSlots(t, r.Facts, sc, o)
	sc2, fl2 := Build(in, o)
	if (fl == nil) != (fl2 == nil) || !bytes.Equal(a, encodeScene(t, sc2)) {
		t.Fatalf("two builds differ")
	}
	if len(data) > 0 && data[0]&7 == 0 {
		dec := p.decodedFacts(t, r.Header, r.Facts)
		sc3, _ := Build(p.input(dec), o)
		if !bytes.Equal(a, encodeScene(t, sc3)) {
			t.Fatalf("Build(facts) differs from Build(Decode(Encode(facts)))")
		}
	}
	return true, ""
}

func (p *prog) decodedFacts(t testing.TB, h trace.Header, facts []trace.Fact) []trace.Fact {
	return p.decoded(t, h, facts)
}

// FuzzScene builds the scene of an example with a random look (plan 5.5):
// never a panic, the same scene twice, the invariants of checkScene and slots
// that respect the scheduling rules.
func FuzzScene(f *testing.F) {
	base := exampleFiles(f)
	var dirs []string
	for k := range base {
		if strings.HasSuffix(k, "/demo.synod") {
			dirs = append(dirs, strings.TrimSuffix(strings.TrimPrefix(k, "/m/"), "/demo.synod"))
		}
	}
	sort.Strings(dirs)
	seeds := [][]byte{{}, {1, 2, 3, 4, 5, 6, 7, 8}, []byte("the quick brown fox jumps over the lazy dog"),
		{200, 17, 99, 3, 250, 8, 77, 64, 5, 5, 5, 31, 90, 12, 140, 2, 60, 61, 62, 63, 64}}
	for _, d := range dirs {
		for _, s := range seeds {
			f.Add(d, s)
		}
	}
	f.Fuzz(func(t *testing.T, dir string, data []byte) {
		if len(data) > 512 {
			t.Skip()
		}
		fuzzScene(t, base, dir, data)
	})
}

// TestFuzzGenerator: the random looks mostly check clean and run, so that the
// fuzzer spends its time in the builder.
func TestFuzzGenerator(t *testing.T) {
	base := exampleFiles(t)
	ran, total := 0, 0
	var first string
	for i := 0; i < 120; i++ {
		data := make([]byte, 24)
		x := uint32(i*2654435761 + 12345)
		for j := range data {
			x = x*1664525 + 1013904223
			data[j] = byte(x >> 24)
		}
		dir := []string{"tcp", "twopc", "wal", "ring", "paxos", "gossip"}[i%6]
		ok, d := fuzzScene(t, base, dir, data)
		total++
		if ok {
			ran++
		} else {
			if first == "" {
				first = d + "\n" + genLook(data)
			}
			t.Log(strings.SplitN(d, "\n", 2)[0])
		}
	}
	t.Logf("%d of %d generated programs ran", ran, total)
	if ran*10 < total*9 {
		t.Errorf("only %d of %d generated programs ran; first failure:\n%s", ran, total, first)
	}
}
