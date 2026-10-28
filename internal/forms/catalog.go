// Package forms holds the catalog of what the tutorial must teach (decision
// 104): the language forms of spec/02, the types of spec/04, the contexts of
// spec/05 and the std packages, and a walk that finds which of them a program
// uses. It serves the coverage check of the documentation, internal/forms/cmd/
// doccheck, and is linked into neither synod nor synod.wasm.
package forms

import (
	"sort"

	"github.com/synodlang/compiler/internal/doc"
)

// Kind tells what a catalog item is.
type Kind string

// The kinds of item. Ids are unique within a kind only (the form "main" and
// the context "main"), so a row of coverage.md names an item as "<kind>:<id>".
const (
	KindForm    Kind = "form"
	KindType    Kind = "type"
	KindContext Kind = "context"
	KindPackage Kind = "package"
)

// Item is one thing a chapter introduces.
type Item struct {
	ID   string
	Kind Kind
	// Spec is where the spec describes the item, "spec/02 §2.2".
	Spec string
	// Productions are the productions of spec/02 §2.1 and §2.2 that the form
	// is a spelling of; empty for items without one.
	Productions []string
	// Terminals are the quoted terminals of those productions that the form is
	// spelled with.
	Terminals []string
	// Prose marks an item that a chapter explains in prose only, because
	// user code cannot use it (define native belongs to std, type value to
	// native patterns): doccheck does not ask for a step that uses it.
	Prose bool
}

// Key is the name of the item in coverage.md and in a Uses: "form:main".
func (i Item) Key() string { return Key(i.Kind, i.ID) }

// Key returns "<kind>:<id>".
func Key(k Kind, id string) string { return string(k) + ":" + id }

func f(id, spec string, prods []string, terms ...string) Item {
	return Item{ID: id, Kind: KindForm, Spec: spec, Productions: prods, Terminals: terms}
}

func p(names ...string) []string { return names }

var formItems = []Item{
	f("header", "spec/06 §6.1", nil),
	f("comment", "spec/02 §2.1", nil),
	f("block", "spec/02 §2.1", p("block")),
	f("paren", "spec/02 §2.1", p("arg"), "(", ")"),
	f("member", "spec/02 §2.1", nil, "."),
	f("list", "spec/02 §2.1", p("list"), "{", "}", ","),
	f("empty", "spec/02 §2.1", nil, "{", "}"),
	f("continuation", "spec/02 §2.1", p("continuation")),
	f("value-number", "spec/04 §4.1", p("value")),
	f("value-duration", "spec/04 §4.1", p("value")),
	f("value-text", "spec/04 §4.1", p("value")),
	f("value-color", "spec/09 §9.1", p("value")),
	f("value-bool", "spec/04 §4.1", p("value"), "true", "false"),

	f("use", "spec/06 §6.2", p("import"), "use"),
	f("use-as", "spec/06 §6.3", p("import"), "as"),
	f("define", "spec/03 §3.1", p("definition", "pattern", "head"), "define"),
	f("define-native", "spec/03 §3.6", p("definition"), "native"),
	f("pattern-hole", "spec/03 §3.1", p("hole", "pattern", "type"), "[", ":", "]", "list", "of", "map", "to"),
	f("pattern-receiver", "spec/03 §3.4", p("pattern")),
	f("pattern-block", "spec/03 §3.5", p("pattern")),
	f("run-block", "spec/03 §3.5", nil),
	f("main", "spec/05 §5.2", p("main"), "main"),
	f("start", "spec/05 §5.3", p("start"), "start"),
	f("has-node", "spec/05 §5.1", p("typedecl"), "has"),
	f("has-record", "spec/04 §4.3", p("typedecl")),
	f("set-local", "spec/01 §1.5", p("assignment", "place"), "set", "to"),
	f("set-field", "spec/05 §5.1", p("place")),
	f("set-item", "spec/04 §4.4", p("place"), "item", "of"),
	f("when-node", "spec/05 §5.4", p("handler"), "when"),
	f("when-trace", "spec/09 §9.3", p("handler")),
	f("when-typed", "spec/05 §5.6", p("handler")),
	f("after", "spec/05 §5.4", p("delay"), "after"),
	f("deliver", "spec/05 §5.4", p("delivery"), "deliver"),
	f("emit", "spec/05 §5.4", p("emission"), "emit", "by"),
	f("if", "spec/02 §2.2", p("choice"), "if"),
	f("else", "spec/02 §2.1", nil, "else"),
	f("each", "spec/04 §4.7", p("loop"), "each", "in"),
	f("each-pair", "spec/04 §4.7", nil),
	f("result", "spec/03 §3.4", p("return"), "result"),
	f("always", "spec/05 §5.7", p("invariant"), "always"),
	f("draw", "spec/09 §9.1", p("drawing"), "draw", "as"),
	f("draw-for", "spec/09 §9.2", nil, "for"),
	f("draw-block", "spec/09 §9.5", nil),
	f("attachment", "spec/09 §9.2", p("attachment")),
	f("message", "spec/04 §4.3", p("record", "setting"), "message", "with"),
	f("message-qualified", "spec/04 §4.3", nil, "."),
	f("item", "spec/04 §4.4", p("element")),

	f("is", "spec/02 §2.3", p("compare"), "is"),
	f("is-not", "spec/02 §2.3", nil, "is not"),
	f("is-in", "spec/04 §4.7", nil, "is in"),
	f("is-not-in", "spec/04 §4.7", nil, "is not in"),
	f("less-than", "spec/02 §2.3", nil, "less than"),
	f("more-than", "spec/02 §2.3", nil, "more than"),
	f("at-least", "spec/02 §2.3", nil, "at least"),
	f("at-most", "spec/02 §2.3", nil, "at most"),
	f("and", "spec/04 §4.5", p("logic"), "and"),
	f("or", "spec/04 §4.5", nil, "or"),
	f("not", "spec/04 §4.5", nil, "not"),
	f("plus", "spec/04 §4.5", p("arithmetic"), "plus"),
	f("minus", "spec/04 §4.5", nil, "minus"),
	f("times", "spec/04 §4.5", nil, "times"),
	f("divided-by", "spec/04 §4.5", nil, "divided by"),
}

var typeItems = []Item{
	{ID: "text", Spec: "spec/04 §4.1"},
	{ID: "number", Spec: "spec/04 §4.1"},
	{ID: "bool", Spec: "spec/04 §4.1"},
	{ID: "time", Spec: "spec/04 §4.1"},
	{ID: "list", Spec: "spec/04 §4.2"},
	{ID: "map", Spec: "spec/04 §4.2"},
	{ID: "node", Spec: "spec/04 §4.2"},
	{ID: "Node", Spec: "spec/04 §4.2"},
	{ID: "msg", Spec: "spec/04 §4.2"},
	{ID: "record", Spec: "spec/04 §4.2"},
	{ID: "shape", Spec: "spec/04 §4.2"},
	{ID: "color", Spec: "spec/04 §4.2"},
	{ID: "field", Spec: "spec/04 §4.2"},
	{ID: "package", Spec: "spec/04 §4.2"},
	{ID: "block", Spec: "spec/04 §4.2"},
	{ID: "value", Spec: "spec/04 §4.2"},
}

var contextItems = []Item{
	{ID: "node", Spec: "spec/05 §5.4"},
	{ID: "main", Spec: "spec/05 §5.4"},
	{ID: "check", Spec: "spec/05 §5.4"},
	{ID: "look", Spec: "spec/05 §5.4"},
}

// proseOnly lists the items that need no step.
var proseOnly = map[string]bool{"form:define-native": true, "type:value": true}

// Catalog returns every item: the forms, the types, the contexts and the std
// packages (those of doc.StdPackages, in byte order).
func Catalog() []Item {
	var out []Item
	out = append(out, formItems...)
	for _, it := range typeItems {
		it.Kind = KindType
		out = append(out, it)
	}
	for _, it := range contextItems {
		it.Kind = KindContext
		out = append(out, it)
	}
	pkgs := doc.StdPackages()
	sort.Strings(pkgs)
	for _, path := range pkgs {
		out = append(out, Item{ID: path, Kind: KindPackage, Spec: "std/README.md"})
	}
	for i := range out {
		out[i].Prose = proseOnly[out[i].Key()]
	}
	return out
}

// ByKey indexes items by Key.
func ByKey(items []Item) map[string]Item {
	m := make(map[string]Item, len(items))
	for _, it := range items {
		m[it.Key()] = it
	}
	return m
}
