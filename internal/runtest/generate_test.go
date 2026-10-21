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
