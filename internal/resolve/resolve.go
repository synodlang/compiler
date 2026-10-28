// Package resolve turns the parse trees of one package into a resolved
// program: it declares the package's records, node fields and phrases,
// matches every sentence to the phrase or language form it calls, binds
// names, and produces the fixes that only a resolver can decide (settings
// order, lower-case names, redundant own qualifiers).
//
// Resolution is syntactic. It never looks at the types of values: a sentence
// calls the one phrase whose shape matches (spec/03 section 3.3), and the
// checker (internal/check) decides afterwards whether the arguments have
// accepted types. Where only a type can tell what a name means (a member
// read "n.Known" is a node field or a zero-hole receiver phrase), the result
// is a Select that the checker settles.
//
// The output is deterministic: declarations are in file order (files in name
// order, then line order), and every table is a slice or is read through a
// method that does not depend on map order.
//
// The diagnostics are those of the codes unknown-phrase, ambiguous-phrase,
// duplicate-name, shadowing, reserved-word, bad-pattern, bad-receiver,
// bad-place (syntactic cases), unexported, unused-name, unused-import,
// unneeded-alias, missing-main, extra-main, color-literal,
// native-outside-std and context (file root, has blocks, result placement,
// constant initial values), plus non-canonical for names that cannot be
// renamed by a fix. The checker owns the rest.
package resolve

import (
	"sort"
	"strings"

	"github.com/synodlang/compiler/internal/format"
	"github.com/synodlang/compiler/internal/load"
	"github.com/synodlang/compiler/internal/parse"
	"github.com/synodlang/compiler/internal/source"
	"github.com/synodlang/compiler/internal/stdlib"
	"github.com/synodlang/compiler/internal/types"
)

// Package is the resolved form of one load.Package (a library package or a
// main file).
type Package struct {
	Load   *load.Package
	Path   string // import path ("std/net"); a main package has "#main" appended
	Name   string // qualifier: last segment of Path, or "main"
	IsMain bool
	IsStd  bool
	// IsTheme reports a theme package (spec/06 section 6.7): it declares no
	// phrase, record or node field, exports nothing, and its files are read
	// by the checker's theme pass. Node is nil.
	IsTheme bool

	// Files are the files of the package in name order.
	Files []*File
	// Imports lists the imports of all files, in file order then line order.
	Imports []*Import

	// Records are the record types in file order (Node is not among them).
	Records []*Record
	// Node is the node type of a library package: always non-nil there,
	// without fields if there is no "Node has". It is nil for a main package.
	Node *Record
	// Phrases are all defined phrases in file order, including natives,
	// event phrases, Start and Main.
	Phrases []*Phrase
	// Start is "define [n: Node] start", or nil. Main is "define main", or
	// nil. Both are also in Phrases.
	Start, Main *Phrase
	// Handlers are the "when" blocks in file order.
	Handlers []*Handler
	// Invariants are the "always" lines in file order.
	Invariants []*Invariant

	// Fixes are the changes to canonical form that this resolution decided,
	// per file, for format.CheckWith and format.FormatWith.
	Fixes map[*load.File][]format.Fix
	// Tokens are the classified tokens of each file, in position order.
	Tokens map[*load.File][]Token

	// Observers reports that the package has an invariant or a trace-event
	// handler; importing such a package uses it (decision 31).
	Observers bool

	heads   map[string][]*Phrase
	records map[string]*Record
	fields  map[string]*Field
	names   map[string]*decl
}

// File is a source file of a Package with its imports.
type File struct {
	Load *load.File
	// Path is the path shown in diagnostics.
	Path string
	// Imports are the "use" lines of the file.
	Imports []*Import
	byQual  map[string]*Import
}

// Import is one "use" line, resolved.
type Import struct {
	File *File
	Imp  *load.Import
	// Qual is the qualifier: the alias, or the last segment of the path.
	Qual string
	// Pkg is the imported package, or nil when the import failed to load or
	// its package is broken; references to a nil Pkg resolve silently.
	Pkg *Package
	// Used reports that the file refers to the qualifier.
	Used bool
	// refs are the positions of the qualifier in the file.
	refs     []source.Pos
	typeRefs bool
	Pos      source.Pos // the path
	// Sp is the span of the path as written, version included.
	Sp source.Span
}

// Record is a record type declared with "T has", or the node type of the
// package ("Node has").
type Record struct {
	Pkg      *Package
	File     *File
	Name     string
	Exported bool
	IsNode   bool
	// Fields are in declaration order.
	Fields []*Field
	Pos    source.Pos
	// Sp is the span of the declaring line, "Ping has".
	Sp source.Span
}

// Type returns the type of the record's values: NodeOf for a node type.
func (r *Record) Type() types.Type {
	if r.IsNode {
		return types.NodeOf{Pkg: r.Pkg.Path}
	}
	return types.Record{Pkg: r.Pkg.Path, Name: r.Name}
}

// Field is a record field or a node field.
type Field struct {
	Owner    *Record
	Name     string
	Exported bool
	// Init is the initial value, a constant expression.
	Init  Expr
	Index int
	Pos   source.Pos
	// Sp is the span of the field name in its declaration.
	Sp   source.Span
	File *File
	// Tainted is set when a sentence that failed to resolve names the field
	// (see Local.Tainted).
	Tainted bool
}

// Lookup returns the field called name, or nil.
func (r *Record) Lookup(name string) *Field {
	for _, f := range r.Fields {
		if f.Name == name {
			return f
		}
	}
	return nil
}

// ParamKind tells what a Param is.
type ParamKind uint8

// The kinds of parameter.
const (
	ParamHole     ParamKind = iota // an inline hole
	ParamReceiver                  // the hole before the head
	ParamBlock                     // a block hole
)

// Param is a hole of a pattern.
type Param struct {
	Name string
	Type types.Type
	Kind ParamKind
	// Index counts the inline holes (neither the receiver nor block
	// holes), as stdlib.Sig does; -1 for the receiver and block holes.
	Index int
	Pos   source.Pos
	// Sp is the span of the name in its hole; HoleSp the span of the whole
	// hole.
	Sp, HoleSp source.Span
	// Source is the declared type as written, "list of node".
	Source string
	// refs are the positions where the body names the parameter.
	refs []source.Pos
}

// SymName implements Symbol.
func (p *Param) SymName() string { return p.Name }

// PatItem is one inline item of a pattern: a literal word or an inline hole.
type PatItem struct {
	Word  string // the literal word; "" for a hole
	Param *Param // the hole, or nil for a word
	Pos   source.Pos
	Sp    source.Span
}

// BlockSlot is a block hole with the continuation word before it ("" for the
// first block of a sentence).
type BlockSlot struct {
	Word  string
	Param *Param
}

// PhraseKind tells what a Phrase is.
type PhraseKind uint8

// The kinds of phrase.
const (
	PhraseNormal PhraseKind = iota
	PhraseStart             // define [n: Node] start
	PhraseMain              // define main
)

// Phrase is a defined phrase, native or written in Synod.
type Phrase struct {
	Pkg  *Package
	File *File
	Pos  source.Pos
	// Sp is the span of the pattern from the word "define" to its last item.
	Sp   source.Span
	Kind PhraseKind
	// Head is the first word; Skeleton is the head and the literal words
	// joined by single spaces ("Learn at"); Name is the skeleton as the spec
	// writes it, with "…" for the holes between words ("Learn … at").
	Head, Skeleton, Name string
	Exported             bool
	// Receiver is the hole before the head, or nil.
	Receiver *Param
	// Items are the inline items of the pattern in order, the head first.
	Items []PatItem
	// Params are the inline holes in order (Index 0, 1, …).
	Params []*Param
	// Blocks are the block holes in order.
	Blocks []BlockSlot
	// Native is the id of a native phrase, or "". Sig is its signature, or
	// nil for a phrase written in Synod.
	Native string
	Sig    *stdlib.Sig
	// Body is the body of a phrase written in Synod; nil for a native.
	Body *Block
	// Locals are all locals declared in the body, in declaration order.
	Locals []*Local
	// IsValue reports that the phrase gives a value: a native with a result
	// rule, or a body with a "result" statement.
	IsValue bool
	// Event is the kind of an event phrase (stdlib.NodeEvent or
	// stdlib.TraceEvent) and false for an ordinary phrase.
	Event   stdlib.Kind
	IsEvent bool
	// Contexts is where a std phrase may be used (stdlib.Any for others).
	Contexts stdlib.Ctx
	// Zero reports a pattern of the head alone: no inline holes, no blocks.
	Zero bool
	// Line is the "define" line.
	Line *parse.Line

	dup  bool
	form *form
}

// Describe prints the pattern as the spec does: "net.Send [m: msg] from
// [a: node] to [b: node]" qualified as from package from ("" = current), and
// "[n: Node] Learn [k: node] at [v: number]" for a receiver phrase.
func (p *Phrase) Describe(qual string) string {
	var b strings.Builder
	if p.Receiver != nil {
		b.WriteString("[" + p.Receiver.Name + ": " + p.Receiver.Source + "] ")
	}
	for i, it := range p.Items {
		if i > 0 {
			b.WriteByte(' ')
		}
		switch {
		case it.Param != nil:
			b.WriteString("[" + it.Param.Name + ": " + it.Param.Source + "]")
		case i == 0 && qual != "":
			b.WriteString(qual + "." + it.Word)
		default:
			b.WriteString(it.Word)
		}
	}
	for _, bs := range p.Blocks {
		if bs.Word != "" {
			b.WriteString(" " + bs.Word)
		}
		b.WriteString(" [" + bs.Param.Name + ": block]")
	}
	return b.String()
}

// Handler is a "when" block.
type Handler struct {
	Pkg  *Package
	File *File
	Pos  source.Pos
	// Event is the event phrase (a std native), Kind its kind.
	Event *Phrase
	Kind  stdlib.Kind
	// Receiver is the "[n: Node]" binding of a node event, or nil.
	Receiver *Binding
	// Matchers are one per inline hole of the event phrase, in order.
	Matchers []Matcher
	// Bindings are all names the handler binds, the receiver first.
	Bindings []*Binding
	Body     *Block
	// Locals are all locals declared in the body.
	Locals []*Local
}

// MatchKind tells how an event hole is matched (spec/05 section 5.6).
type MatchKind uint8

// The kinds of matcher.
const (
	// MatchBind binds a new name to the value in the hole.
	MatchBind MatchKind = iota
	// MatchRecord filters msg by a record type and binds the record under
	// the spelling written ("Prepare", "api.Get").
	MatchRecord
	// MatchNodeType filters node holes to nodes running NodePkg and binds
	// the node with that type ("[n: raft.Node]").
	MatchNodeType
	// MatchField filters a field hole by a node field.
	MatchField
	// MatchEqual filters by equality with Value.
	MatchEqual
)

// Matcher is the treatment of one inline hole of an event phrase.
type Matcher struct {
	Hole int // index of the inline hole
	Kind MatchKind
	// Binding is set for MatchBind, MatchRecord and MatchNodeType.
	Binding *Binding
	Record  *Record  // MatchRecord
	NodePkg *Package // MatchNodeType
	Field   *Field   // MatchField
	Value   Expr     // MatchEqual
	Pos     source.Pos
}

// BindKind tells how a Binding was introduced.
type BindKind uint8

// The kinds of binding.
const (
	BindReceiver BindKind = iota // "[n: Node]" of a when
	BindHole                     // a new bare word in an event hole
	BindRecord                   // a record type word
	BindNodeType                 // "[n: p.Node]"
)

// Binding is a name bound by a "when" (spec/05 section 5.6).
type Binding struct {
	Name string
	Kind BindKind
	Pos  source.Pos
	Sp   source.Span
	// Type is the type of the bound value: the hole's type, the record
	// type, or the node type. It is value for the old and new holes of a
	// Changed event; then OfField, if set, says that the type is that of the
	// filtered field.
	Type    types.Type
	Record  *Record
	OfField *Field
	// Hole is the event hole the binding fills (-1 for the receiver).
	Hole int
	refs []source.Pos
}

// SymName implements Symbol.
func (b *Binding) SymName() string { return b.Name }

// Invariant is an "always" line.
type Invariant struct {
	Pkg  *Package
	File *File
	Pos  source.Pos
	Cond Expr
	// Rule is the text recorded in sim.Broken: "<path>:<line>: always <arg>".
	Rule string
}

// Namespace queries for the checker.

// Head returns the phrases whose head is h, in definition order.
func (p *Package) Head(h string) []*Phrase { return p.heads[h] }

// Record returns the record type called name, or nil.
func (p *Package) Record(name string) *Record { return p.records[name] }

// NodeField returns the node field called name, or nil.
func (p *Package) NodeField(name string) *Field { return p.fields[name] }

// ReceiverPhrase returns the receiver phrase with head h, or nil.
func (p *Package) ReceiverPhrase(h string) *Phrase {
	for _, ph := range p.heads[h] {
		if ph.Receiver != nil && ph.Kind == PhraseNormal {
			return ph
		}
	}
	return nil
}

// Import returns the import of f (qualifier or alias) q, or nil.
func (f *File) Import(q string) *Import { return f.byQual[q] }

// Exported reports whether name starts with an upper-case letter.
func Exported(name string) bool {
	return name != "" && name[0] >= 'A' && name[0] <= 'Z'
}

// Position helpers.

// sortedKeys returns the keys of m in order.
func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
