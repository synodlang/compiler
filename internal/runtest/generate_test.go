package runtest

import (
	"fmt"
	"math/rand"
	"strings"
	"testing"

	"github.com/synodlang/compiler/internal/load"
)

// The generator builds small valid programs from templates: arithmetic with
// division by zero, items of lists and maps, sends, afters, crashes and
// recursion. Every program must check clean and run twice to the same trace
// without a panic; run-time errors are fine (the trace ends in sim.Failed).

const genLib = `# package p
use std/net
use std/sim
use std/time

R has
  set v to 0
  set t to 0s

Go has
  set k to 0

Msg has
  set k to 0

Node has
  set Peers to {}
  set Count to 0

define Meet [ns: list of Node]
  each n in ns
    set n.Peers to ns

define Fact of [n: number]
  if (n at most 1)
    result 1
  result (n times (Fact of (n minus 1)))

define Spin of [n: number]
  result (Spin of (n plus 1))

define [n: Node] start
  time.Every 7ms
    set n.Count to (n.Count plus 1)

when [n: Node] (sim.Delivered Go)
  each q in n.Peers
    net.Send (message Msg with k Go.k) from n to q

when [n: Node] (net.Received Msg from sender)
  set n.Count to (n.Count plus Msg.k)
  if (Msg.k more than 0)
    net.Send (message Msg with k (Msg.k minus 1)) from n to sender
`

type gen struct {
	r     *rand.Rand
	lines []string
	nodes int
	uniq  int
}

// name returns a fresh variable name.
func (g *gen) name(base string) string {
	g.uniq++
	return fmt.Sprintf("%s%d", base, g.uniq)
}

func (g *gen) pick(xs ...string) string { return xs[g.r.Intn(len(xs))] }

func (g *gen) num() string {
	return g.pick("0", "1", "2", "3", "5", "7", "-1", "-2", "0.5", "1.5", "100", "1000000")
}

// arith returns a number expression of the given depth.
func (g *gen) arith(depth int) string {
	if depth == 0 || g.r.Intn(4) == 0 {
		return g.num()
	}
	op := g.pick("plus", "minus", "times", "divided by")
	return fmt.Sprintf("(%s %s %s)", g.arith(depth-1), op, g.arith(depth-1))
}

func (g *gen) dur() string {
	return g.pick("0s", "1ms", "5ms", "20ms", "100ms", "1s", "2500ms")
}

func (g *gen) add(indent int, s string) {
	g.lines = append(g.lines, strings.Repeat("  ", indent)+s)
}

func (g *gen) emitNum(indent int, e string) {
	g.add(indent, fmt.Sprintf("emit (message p.R with v %s) by sim.Nobody", e))
}

// stmt adds one statement at the given indent.
func (g *gen) stmt(indent, depth int) {
	switch g.r.Intn(12) {
	case 0, 1:
		g.emitNum(indent, g.arith(3))
	case 2:
		xs := g.name("xs")
		g.add(indent, "set "+xs+" to {"+g.num()+", "+g.num()+", "+g.num()+"}")
		g.emitNum(indent, fmt.Sprintf("(item %d of %s)", g.r.Intn(6)-1, xs))
	case 3:
		m := g.name("m")
		g.add(indent, "set "+m+" to {}")
		g.add(indent, fmt.Sprintf("set item \"k%d\" of %s to %s", g.r.Intn(3), m, g.arith(1)))
		g.emitNum(indent, fmt.Sprintf("(item \"k%d\" of %s)", g.r.Intn(3), m))
	case 4:
		g.emitNum(indent, fmt.Sprintf("(p.Fact of %d)", g.r.Intn(25)))
	case 5:
		if g.r.Intn(6) == 0 {
			g.emitNum(indent, "(p.Spin of 0)")
		} else {
			g.emitNum(indent, fmt.Sprintf("(p.Fact of %d)", g.r.Intn(10)))
		}
	case 6:
		g.add(indent, fmt.Sprintf("emit (message p.R with t (%s times %s)) by sim.Nobody", g.dur(), g.num()))
		g.add(indent, fmt.Sprintf("emit (message p.R with t (%s divided by %s)) by sim.Nobody", g.dur(), g.num()))
	case 7:
		if g.nodes > 0 {
			g.add(indent, fmt.Sprintf("deliver (message p.Go with k %d) to (item %d of nodes)", g.r.Intn(4), g.r.Intn(g.nodes+1)))
		}
	case 8:
		if g.nodes > 0 {
			g.add(indent, fmt.Sprintf("sim.%s (item %d of nodes)", g.pick("Crash", "Restart", "Remove"), g.r.Intn(g.nodes+1)))
		}
	case 9, 10:
		if depth > 0 {
			g.add(indent, "after "+g.dur())
			before := len(g.lines)
			for i := 0; i <= g.r.Intn(3); i++ {
				g.stmt(indent+1, depth-1)
			}
			if len(g.lines) == before {
				g.emitNum(indent+1, g.arith(1))
			}
		}
	case 11:
		g.add(indent, fmt.Sprintf("if (%s %s %s)", g.arith(1), g.pick("is", "is not", "more than", "less than"), g.arith(1)))
		g.emitNum(indent+1, g.arith(2))
	}
}

// program builds one main file.
func (g *gen) program() string {
	g.lines = nil
	g.uniq = 0
	g.nodes = g.r.Intn(5)
	g.add(0, "# package main")
	g.add(0, "use example.com/g/p")
	g.add(0, "use std/sim")
	g.add(0, "")
	g.add(0, "define main")
	if g.r.Intn(2) == 0 {
		g.add(1, fmt.Sprintf("sim.Seed %d", g.r.Intn(1000)))
	}
	g.emitNum(1, "0")
	if g.nodes > 0 {
		g.add(1, fmt.Sprintf("set nodes to (sim.Cluster of %d running p)", g.nodes))
		g.add(1, "p.Meet nodes")
	}
	for i := 0; i < 1+g.r.Intn(6); i++ {
		g.stmt(1, 2)
	}
	if g.r.Intn(6) != 0 {
		g.add(1, "sim.StopAt "+g.pick("100ms", "1s", "5s"))
	}
	return strings.Join(g.lines, "\n") + "\n"
}

// TestGeneratedPrograms builds 1,000 programs with a fixed seed.
func TestGeneratedPrograms(t *testing.T) {
	g := &gen{r: rand.New(rand.NewSource(20240607))}
	for i := 0; i < 1000; i++ {
		src := g.program()
		fsys := load.MapFS{
			"/m/synod.mod":       "module example.com/g\nsynod 0.2\n",
			"/m/p/p.synod":       genLib,
			"/m/main/main.synod": src,
		}
		if ok, d := runMap(t, fsys, "/m/main"); !ok {
			t.Fatalf("program %d does not check:\n%s\n%s", i, src, d)
		}
	}
}

// lookStmt returns one random look statement over the shapes card (a card)
// and c (a circle) and the node n. Some of them fault on purpose.
func (g *gen) lookStmt() string {
	return g.pick(
		"look.Fill card with theme."+g.pick("Accent", "Good", "Critical", "Quiet"),
		"look.Stroke c with theme."+g.pick("Ink", "Line", "Warning"),
		fmt.Sprintf("look.Tween %s of card to %s over %s", g.pick(`"x"`, `"y"`, `"opacity"`, `"scale"`, `"rotation"`, `"nope"`), g.pick("0", "1", "-1", "0.5", "300", "900", "2"), g.dur()),
		"look.Tween \"x\" of c to card over "+g.dur(),
		"look.Fade card to "+g.pick("0", "1", "0.5", "2", "-1"),
		"look.Pulse c",
		"look.Effect "+g.pick(`"glow"`, `"blur"`, `"trail"`, `"nope"`)+" on card",
		"look.Layout "+g.pick(`"ring"`, `"row"`, `"grid"`, `"nope"`),
		"look.Place c "+g.pick("above", "below", "beside")+" card",
		"look.Place card "+g.pick("above", "below", "beside")+" c",
		"look.Remove c",
		"look.Orbit c around card at "+g.pick("0", "0.25", "1.5"),
		"draw (look.Label \"t\") as c for \"c\"",
		"draw (look.Circle "+g.pick("4", "0", "-1", "30")+") as c2\n    look.Place c2 above c",
		"draw (look.Line from c to card) as ln",
		"look.Fill (card of \"nope\") with theme.Accent",
	)
}

// lookPackage builds a look with random statements, in the shapes of the
// default look: cards for nodes, a circle per message that flies.
func (g *gen) lookPackage() string {
	var b strings.Builder
	b.WriteString("# package lk\nuse example.com/g/p\nuse std/look\nuse std/net\nuse std/sim\nuse std/theme\n\n")
	b.WriteString("when (sim.Added n)\n  draw (look.Card for n) as card for n\n  draw (look.Circle 5) as c\n")
	for i := 0; i < g.r.Intn(4); i++ {
		b.WriteString("  " + g.lookStmt() + "\n")
	}
	b.WriteString("\nwhen (sim.Crashed n)\n  look.Fill (card of n) with theme.Quiet\n")
	b.WriteString("\nwhen (net.Sent m from a to b)\n  draw (look.Circle 6) as dot for (net.Id m)\n  look.Flight dot from (card of a) to (card of b) for m\n")
	if g.r.Intn(2) == 0 {
		b.WriteString("\nwhen (net.Arrived m from a to b)\n  look.Remove (dot of (net.Id m))\n")
	}
	b.WriteString("\nwhen (sim.Emitted p.R by who)\n  draw (look.Circle 2) as c\n  draw (look.Rect 40 by 20) as card\n  " + g.lookStmt() + "\n")
	return b.String()
}

// TestGeneratedLooks runs generated programs with generated looks: no panic,
// scenes that encode, are stable and obey the invariants, or a look failure
// with a partial scene.
func TestGeneratedLooks(t *testing.T) {
	g := &gen{r: rand.New(rand.NewSource(20260101))}
	ran := 0
	lookFaults = 0
	for i := 0; i < 300; i++ {
		src := strings.Replace(g.program(), "use example.com/g/p\n", "use example.com/g/lk\nuse example.com/g/p\n", 1)
		lk := g.lookPackage()
		fsys := load.MapFS{
			"/m/synod.mod":       "module example.com/g\nsynod 0.2\n",
			"/m/p/p.synod":       genLib,
			"/m/lk/lk.synod":     lk,
			"/m/main/main.synod": src,
		}
		ok, d := runMap(t, fsys, "/m/main")
		if !ok {
			t.Fatalf("program %d does not check:\n%s\n%s\n%s", i, src, lk, d)
		}
		ran++
	}
	t.Logf("%d programs with looks ran, %d look faults in the runs", ran, lookFaults)
	if lookFaults == 0 {
		t.Error("no generated look faulted")
	}
}
