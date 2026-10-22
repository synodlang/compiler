package stdlib

import "github.com/synodlang/compiler/internal/types"

// Ctx is a set of contexts (spec/05 section 5.4).
type Ctx uint8

// The four contexts. Any is all of them.
const (
	Node Ctx = 1 << iota
	Main
	Check
	Look
	Any = Node | Main | Check | Look
)

// Has reports whether c includes every context of o.
func (c Ctx) Has(o Ctx) bool { return c&o == o }

// String lists the contexts the way the catalog does: "node, main", "any".
func (c Ctx) String() string {
	if c == Any {
		return "any"
	}
	s := ""
	for _, e := range []struct {
		c Ctx
		n string
	}{{Node, "node"}, {Main, "main"}, {Check, "check"}, {Look, "look"}} {
		if c&e.c != 0 {
			if s != "" {
				s += ", "
			}
			s += e.n
		}
	}
	return s
}

// Kind tells what a native phrase is.
type Kind uint8

// The kinds of native phrase.
const (
	Phrase     Kind = iota // an ordinary phrase: an action or a value
	NodeEvent              // delivered to one node; used after "when [n: Node]"
	TraceEvent             // recorded in the trace; used after "when"
)

// RuleKind selects how the result type of a native is computed.
type RuleKind uint8

// The result rules. Arguments name inline holes by index; the receiver and
// block holes are not counted.
const (
	RuleNone            RuleKind = iota // an action: no result
	RuleFixed                           // the type T
	RuleSameAs                          // the type of hole Arg
	RuleElemOf                          // the item type of the list in hole Arg
	RuleKeysOf                          // list of the key type of the map in hole Arg
	RuleNodeOfPkg                       // p.Node, p being the package named by hole Arg
	RuleListOfNodeOfPkg                 // list of p.Node
)

// Rule is the result of a native.
type Rule struct {
	Kind RuleKind
	T    types.Type // RuleFixed
	Arg  int        // every kind but RuleNone and RuleFixed
}

// None is the result rule of an action.
var None = Rule{Kind: RuleNone}

// Fixed gives the result the type t.
func Fixed(t types.Type) Rule { return Rule{Kind: RuleFixed, T: t} }

// SameAs gives the result the type of hole i.
func SameAs(i int) Rule { return Rule{Kind: RuleSameAs, Arg: i} }

// ElemOf gives the result the item type of the list in hole i.
func ElemOf(i int) Rule { return Rule{Kind: RuleElemOf, Arg: i} }

// KeysOf gives the result the type "list of K" for the map in hole i.
func KeysOf(i int) Rule { return Rule{Kind: RuleKeysOf, Arg: i} }

// NodeOfPkg gives the result the type p.Node for the package p named by hole
// i. A package that is not a name gives node.
func NodeOfPkg(i int) Rule { return Rule{Kind: RuleNodeOfPkg, Arg: i} }

// ListOfNodeOfPkg is "list of p.Node", or "list of node".
func ListOfNodeOfPkg(i int) Rule { return Rule{Kind: RuleListOfNodeOfPkg, Arg: i} }

// RelKind selects a relation between the arguments of a native that the
// pattern's hole types cannot state.
type RelKind uint8

// The relations. A and B name inline holes by index.
const (
	// RelElemAccepts: the value in hole A is accepted as the item type of the
	// list in hole B (lists.Append, lists.Without).
	RelElemAccepts RelKind = iota
	// RelKeyAccepts: the value in hole A is accepted as the key type of the
	// map in hole B (maps.Without).
	RelKeyAccepts
	// RelSameElem: the item type of the list in hole B is accepted as that of
	// the list in hole A (lists.Extend).
	RelSameElem
	// RelSortable: the items of the list in hole A are numbers, times or text
	// (lists.Sort).
	RelSortable
	// RelExpectResultOn: the type expected for the result is also expected
	// for hole A (disk.Read, spec/04 section 4.4).
	RelExpectResultOn
)

// Rel is one relation.
type Rel struct {
	Kind RelKind
	A, B int
}

// Sig is the typing signature of a native phrase, beyond its pattern.
type Sig struct {
	ID       string
	Kind     Kind
	Contexts Ctx // the contexts the phrase may be used in
	Result   Rule
	Rels     []Rel
	// NamesNode is the index of the inline hole that must name the receiver
	// in node context (spec/05 section 5.4: "from a" of net.Send, "on n" of
	// disk.Read and disk.Write), or -1.
	NamesNode int
}

// Natives holds the signature of every native phrase of std, by native id.
// An id is "<package>.<head with a lower-case first letter>"; the three Place
// patterns are look.placeAbove, look.placeBelow and look.placeBeside.
var Natives = buildNatives()

// Contexts holds the contexts of the phrases that std writes in Synod,
// keyed by "<package>.<skeleton>", where the skeleton is the head and the
// literal words of the pattern, joined by single spaces: "time.Every",
// "lesson.Ask", "look.Fade to", "look.Travel from to over", "theme.Accent".
// The catalog's Context column is enforced at the call site for every std
// phrase, native or not (the bodies of std phrases are not walked).
var Contexts = map[string]Ctx{
	"time.Every":               Node | Main,
	"lesson.Ask":               Main,
	"lesson.Ask choosing":      Main,
	"lesson.Note":              Main,
	"look.Card for":            Look,
	"look.Fade to":             Look,
	"look.Pulse":               Look,
	"look.Travel from to over": Look,
	"look.Remove":              Look,
	"theme.Paper":              Look,
	"theme.Ink":                Look,
	"theme.Line":               Look,
	"theme.Quiet":              Look,
	"theme.Accent":             Look,
	"theme.Good":               Look,
	"theme.Warning":            Look,
	"theme.Critical":           Look,
}

func buildNatives() map[string]*Sig {
	m := map[string]*Sig{}
	add := func(id string, k Kind, ctx Ctx, res Rule, names int, rels ...Rel) {
		if _, dup := m[id]; dup {
			panic("stdlib: duplicate native id " + id) // unreachable: guarded by TestNatives
		}
		m[id] = &Sig{ID: id, Kind: k, Contexts: ctx, Result: res, Rels: rels, NamesNode: names}
	}
	// act adds an action, val a value phrase, node and trace events.
	act := func(id string, ctx Ctx) { add(id, Phrase, ctx, None, -1) }
	val := func(id string, ctx Ctx, t types.Type) { add(id, Phrase, ctx, Fixed(t), -1) }
	node := func(id string) { add(id, NodeEvent, Node, None, -1) }
	trace := func(id string) { add(id, TraceEvent, Look, None, -1) }

	num, txt, flag, tm := types.TNumber, types.TText, types.TBool, types.TTime
	nodes := types.List{Elem: types.TNode}
	shape := types.TShape

	// std/sim
	act("sim.seed", Main)
	act("sim.stopAt", Main)
	add("sim.spawn", Phrase, Main, NodeOfPkg(0), -1)
	add("sim.cluster", Phrase, Main, ListOfNodeOfPkg(1), -1)
	act("sim.crash", Main)
	act("sim.restart", Main)
	act("sim.remove", Main)
	val("sim.id", Any, num)
	val("sim.name", Any, txt)
	val("sim.nobody", Any, types.TNode)
	val("sim.nodes", Main|Check|Look, nodes)
	add("sim.nodesRunning", Phrase, Main|Check|Look, ListOfNodeOfPkg(0), -1)
	val("sim.isUp", Main|Check|Look, flag)
	val("sim.stateRows", Look, types.List{Elem: txt})
	node("sim.delivered")
	for _, id := range []string{"started", "ended", "added", "removed", "crashed", "restarted",
		"changed", "injected", "fired", "broken", "emitted", "failed"} {
		trace("sim." + id)
	}

	// std/net
	add("net.send", Phrase, Node, None, 1)
	act("net.latency", Main)
	act("net.loss", Main)
	act("net.partition", Main)
	act("net.heal", Main)
	act("net.dropNext", Main)
	val("net.id", Look, num)
	val("net.type", Look, txt)
	node("net.received")
	for _, id := range []string{"sent", "arrived", "dropped", "partitioned", "healed"} {
		trace("net." + id)
	}

	// std/time, std/random
	val("time.now", Node|Main|Check, tm)
	add("random.pick", Phrase, Node|Main, ElemOf(0), -1)
	val("random.between", Node|Main, num)
	val("random.timeBetween", Node|Main, tm)
	val("random.chance", Node|Main, flag)

	// std/lists: holes are indexed in pattern order.
	val("lists.size", Any, num)
	add("lists.append", Phrase, Any, SameAs(1), -1, Rel{Kind: RelElemAccepts, A: 0, B: 1})
	add("lists.extend", Phrase, Any, SameAs(0), -1, Rel{Kind: RelSameElem, A: 0, B: 1})
	add("lists.without", Phrase, Any, SameAs(1), -1, Rel{Kind: RelElemAccepts, A: 0, B: 1})
	add("lists.take", Phrase, Any, SameAs(1), -1)
	val("lists.range", Any, types.List{Elem: num})
	add("lists.sort", Phrase, Any, SameAs(0), -1, Rel{Kind: RelSortable, A: 0})

	// std/maps
	val("maps.size", Any, num)
	add("maps.keys", Phrase, Any, KeysOf(0), -1)
	add("maps.without", Phrase, Any, SameAs(1), -1, Rel{Kind: RelKeyAccepts, A: 0, B: 1})
	add("maps.keysByValue", Phrase, Any, KeysOf(0), -1)

	// std/math, std/text
	for _, id := range []string{"floor", "ceil", "min", "max", "remainder", "hash"} {
		val("math."+id, Any, num)
	}
	val("text.from", Any, txt)
	val("text.join", Any, txt)
	val("text.size", Any, num)

	// std/disk
	add("disk.write", Phrase, Node, None, 2)
	add("disk.read", Phrase, Node, SameAs(2), 1, Rel{Kind: RelExpectResultOn, A: 2})
	trace("disk.written")

	// std/lesson: choices is unexported, used by Ask … choosing.
	add("lesson.choices", Phrase, Main, SameAs(0), -1)

	// std/look: every phrase is look-only.
	for _, id := range []string{"circle", "rect", "label", "line", "group", "center"} {
		val("look."+id, Look, shape)
	}
	for _, id := range []string{"fill", "stroke", "tween", "effect", "layout",
		"placeAbove", "placeBelow", "placeBeside", "orbit"} {
		act("look."+id, Look)
	}
	act("look.flight", Look)
	act("look.delete", Look)
	return m
}
