// Package doc documents a package from its source, as the compiler loads and
// checks it (decisions 90 to 92). It builds the model that synod doc prints
// in its short, rich, JSON and catalog forms, and that the Mac app's phrase
// completion reads (synod-doc/1, implementation/04-formats.md).
package doc

import (
	"strings"

	"github.com/synodlang/compiler/internal/check"
	"github.com/synodlang/compiler/internal/driver"
	"github.com/synodlang/compiler/internal/parse"
	"github.com/synodlang/compiler/internal/resolve"
	"github.com/synodlang/compiler/internal/stdlib"
	"github.com/synodlang/compiler/internal/types"
)

// Format is the value of the "format" key of the JSON document.
const Format = "synod-doc/1"

// Item is one item of a pattern: a word or a hole.
type Item struct {
	Word string `json:"word,omitempty"`
	Hole string `json:"hole,omitempty"`
	Type string `json:"type,omitempty"`
}

// Block is a block hole with the word before it ("" for an unnamed one).
type Block struct {
	Word string `json:"word"`
	Hole string `json:"hole"`
}

// Receiver is the hole before the head of a receiver phrase.
type Receiver struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

// Def is the position of a declaration.
type Def struct {
	File string `json:"file"`
	Line int    `json:"line"`
	Col  int    `json:"col"`
}

// Phrase is one phrase, event included.
type Phrase struct {
	Pattern  string    `json:"pattern"`
	Items    []Item    `json:"items"`
	Blocks   []Block   `json:"blocks"`
	Receiver *Receiver `json:"receiver"`
	Gives    string    `json:"gives"`
	Contexts []string  `json:"contexts"`
	Kind     string    `json:"kind"`
	Native   bool      `json:"native"`
	Exported bool      `json:"exported"`
	Doc      string    `json:"doc"`
	Def      Def       `json:"def"`

	ctx stdlib.Ctx // the contexts as a set; 0 when not std
	std bool
}

// Field is a record field or a node field.
type Field struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Exported bool   `json:"exported"`
}

// Record is a record type.
type Record struct {
	Name   string  `json:"name"`
	Doc    string  `json:"doc"`
	Fields []Field `json:"fields"`
}

// Node is the node type of a package.
type Node struct {
	Doc    string  `json:"doc"`
	Fields []Field `json:"fields"`
}

// Package is the documentation of one package.
type Package struct {
	Path      string   `json:"path"`
	Name      string   `json:"name"`
	Qualifier string   `json:"qualifier"`
	Title     string   `json:"title"`
	Std       bool     `json:"std"`
	Activates bool     `json:"activates"`
	Phrases   []Phrase `json:"phrases"`
	Records   []Record `json:"records"`
	Node      *Node    `json:"node"`

	// Observers and Invariants count what importing the package activates.
	Observers  int `json:"-"`
	Invariants int `json:"-"`

	// colors holds the std/theme values by head, for the catalog.
	colors map[string]string
}

// View says how a package is read: which names are listed and how they are
// spelled.
type View struct {
	// All lists every name, unexported ones included.
	All bool
	// Qualifier is how the reader writes the package ("" for bare heads).
	Qualifier string
	// From is the import path of the reader's package, whose own types print
	// bare ("" for none).
	From string
	// Qual returns the qualifier the reader writes for another package; nil
	// or "" means the last segment of its path.
	Qual func(path string) string
}

// ViewOf is the view of a reader who is in no package and writes the
// package by the last segment of its path (synod doc without --scope).
func ViewOf(rp *resolve.Package) View {
	return View{Qualifier: rp.Name}
}

var allCtx = []struct {
	c stdlib.Ctx
	n string
}{{stdlib.Node, "node"}, {stdlib.Main, "main"}, {stdlib.Check, "check"}, {stdlib.Look, "look"}}

func ctxNames(c stdlib.Ctx) []string {
	out := []string{}
	for _, e := range allCtx {
		if c&e.c != 0 {
			out = append(out, e.n)
		}
	}
	return out
}

// ImportPath is the import path of rp (a main package has "#main" appended).
func ImportPath(rp *resolve.Package) string { return rp.Path }

// Build documents rp for a reader described by v.
func Build(res driver.Result, rp *resolve.Package, v View) *Package {
	p := &Package{
		Path: ImportPath(rp), Name: rp.Name, Qualifier: v.Qualifier, Std: rp.IsStd,
		Activates: rp.Observers, Invariants: len(rp.Invariants),
		Phrases: []Phrase{}, Records: []Record{},
	}
	if rp.IsStd {
		p.Title = Title(p.Path)
	}
	for _, h := range rp.Handlers {
		if h.Kind == stdlib.TraceEvent {
			p.Observers++
		}
	}
	info := res.Infos[rp.Path]
	docs := docIndex(rp)
	typeText := func(t types.Type) string {
		if t == nil {
			return "value"
		}
		return types.Format(t, v.From, v.Qual)
	}
	fieldsOf := func(r *resolve.Record) []Field {
		out := []Field{}
		for _, f := range r.Fields {
			if !v.All && !f.Exported {
				continue
			}
			var t types.Type
			if info != nil {
				t = info.FieldTy[f]
			}
			out = append(out, Field{Name: f.Name, Type: typeText(t), Exported: f.Exported})
		}
		return out
	}
	for _, ph := range rp.Phrases {
		// start and main are not phrases a sentence can call.
		if ph.Kind != resolve.PhraseNormal || !v.All && !ph.Exported {
			continue
		}
		p.Phrases = append(p.Phrases, buildPhrase(ph, v, info, docs, typeText))
		if rp.IsStd && rp.Path == "std/theme" {
			if c := colorOf(ph); c != "" {
				if p.colors == nil {
					p.colors = map[string]string{}
				}
				p.colors[ph.Head] = c
			}
		}
	}
	for _, r := range rp.Records {
		if !v.All && !r.Exported {
			continue
		}
		name := r.Name
		if v.Qualifier != "" {
			name = v.Qualifier + "." + name
		}
		p.Records = append(p.Records, Record{Name: name, Doc: docs[lineKey{r.File.Path, r.Pos.Line}], Fields: fieldsOf(r)})
	}
	if n := rp.Node; n != nil && n.File != nil {
		p.Node = &Node{Doc: docs[lineKey{n.File.Path, n.Pos.Line}], Fields: fieldsOf(n)}
	}
	return p
}

func buildPhrase(ph *resolve.Phrase, v View, info *check.Info, docs map[lineKey]string, typeText func(types.Type) string) Phrase {
	out := Phrase{
		Pattern:  ph.DescribeAs(v.Qualifier, v.From, v.Qual),
		Items:    []Item{},
		Blocks:   []Block{},
		Kind:     "phrase",
		Native:   ph.Native != "",
		Exported: ph.Exported,
		Doc:      docs[lineKey{ph.File.Path, ph.Pos.Line}],
		Def:      Def{File: ph.File.Path, Line: ph.Pos.Line, Col: ph.Pos.Col},
		std:      ph.Pkg.IsStd,
	}
	var holes []string
	for _, pa := range ph.Params {
		holes = append(holes, pa.Name)
	}
	if ph.Receiver != nil {
		out.Receiver = &Receiver{Name: ph.Receiver.Name, Type: ph.Receiver.HoleType(v.From, v.Qual)}
	}
	for i, it := range ph.Items {
		switch {
		case it.Param != nil:
			out.Items = append(out.Items, Item{Hole: it.Param.Name, Type: it.Param.HoleType(v.From, v.Qual)})
		case i == 0 && v.Qualifier != "":
			out.Items = append(out.Items, Item{Word: v.Qualifier + "." + it.Word})
		default:
			out.Items = append(out.Items, Item{Word: it.Word})
		}
	}
	for _, bs := range ph.Blocks {
		out.Blocks = append(out.Blocks, Block{Word: bs.Word, Hole: bs.Param.Name})
	}
	switch {
	case ph.IsEvent && ph.Event == stdlib.NodeEvent:
		out.Kind = "node-event"
	case ph.IsEvent:
		out.Kind = "trace-event"
	}
	out.Gives = Action
	switch {
	case ph.Sig != nil:
		out.Gives = GivesText(ph.Sig.Result, holes)
	case ph.IsValue:
		var t types.Type
		if info != nil {
			t = info.ResultTy[ph]
		}
		out.Gives = typeText(t)
	}
	if ph.Pkg.IsStd {
		c := ph.Contexts
		if ph.Sig != nil {
			c = ph.Sig.Contexts
		}
		out.ctx = c
		out.Contexts = ctxNames(c)
	}
	return out
}

type lineKey struct {
	file string
	line int
}

// docIndex maps the top-level lines of the package to their doc comments.
func docIndex(rp *resolve.Package) map[lineKey]string {
	m := map[lineKey]string{}
	for _, f := range rp.Files {
		if f.Load.Tree == nil {
			continue
		}
		for _, n := range f.Load.Tree.Body {
			if l, ok := n.(*parse.Line); ok {
				m[lineKey{f.Path, l.Pos.Line}] = DocText(l.Leading)
			}
		}
	}
	return m
}

// DocText joins the comment lines of a doc comment with single spaces
// (spec/06 section 6.4).
func DocText(cs []*parse.Comment) string {
	var parts []string
	for _, c := range cs {
		parts = append(parts, c.Text)
	}
	return strings.Join(parts, " ")
}

// colorOf returns the color of a zero-hole phrase whose body is "result
// #RRGGBB", or "".
func colorOf(ph *resolve.Phrase) string {
	if ph.Body == nil || len(ph.Body.Stmts) != 1 {
		return ""
	}
	if r, ok := ph.Body.Stmts[0].(*resolve.Result); ok {
		if c, ok := r.Value.(*resolve.ColorLit); ok {
			return c.Hex
		}
	}
	return ""
}
