package resolve

import (
	"github.com/synodlang/compiler/internal/source"
	"github.com/synodlang/compiler/internal/types"
)

// This file holds the tree that resolution produces for the bodies of
// phrases, handlers, invariants and initial values. Every node knows its
// position in the file of the body that holds it. The checker annotates the
// tree: expressions carry a Ty slot that resolution leaves nil.

// At is the position of a node.
type At struct{ Pos source.Pos }

// Where returns the position.
func (a At) Where() source.Pos { return a.Pos }

// Node is a node of the body tree.
type Node interface{ Where() source.Pos }

// Stmt is a statement.
type Stmt interface {
	Node
	stmt()
}

// Expr is an expression.
type Expr interface {
	Node
	// Src is the item as printed from the source ("n.Known",
	// "(lists.Size n.peers)").
	Src() string
	// Type is the type the checker gave the expression (nil before).
	Type() types.Type
	// SetType records the type (the checker).
	SetType(types.Type)
	expr()
}

// ExprBase is embedded in every expression.
type ExprBase struct {
	At
	Text string
	// Ty is set by the checker.
	Ty types.Type
}

// Src implements Expr.
func (e *ExprBase) Src() string { return e.Text }

// Type implements Expr.
func (e *ExprBase) Type() types.Type { return e.Ty }

// SetType implements Expr.
func (e *ExprBase) SetType(t types.Type) { e.Ty = t }
func (*ExprBase) expr()                  {}

// Block is a sequence of statements with its own scope.
type Block struct {
	At
	Stmts []Stmt
}

// Symbol is a name that an expression can refer to: a *Local, a *Param or a
// *Binding.
type Symbol interface{ SymName() string }

// LocalKind tells how a local was introduced.
type LocalKind uint8

// The kinds of local.
const (
	LocalSet  LocalKind = iota // "set x to …"
	LocalEach                  // an "each" name
	LocalDraw                  // a "draw … as w" name
)

// Local is a local variable, an each name or a draw name. A local is one
// place for the checker: one type variable across all its sets.
type Local struct {
	Name string
	Kind LocalKind
	Pos  source.Pos
	// Used reports a read, or use as the base of "set item … of x".
	Used bool
	// Attached is set for a draw name with "for": the shape is also
	// attached to the node, text or number given.
	Attached bool
	// Tainted is set when a sentence that failed to resolve names the local:
	// the uses that would settle its type are lost, so the checker stays quiet
	// about what it cannot infer.
	Tainted bool

	refs      []source.Pos
	eachIdx   int // 0 or 1 for the names of an "each a, b"; -1 otherwise
	eachOther string
}

// SymName implements Symbol.
func (l *Local) SymName() string { return l.Name }

// ---- statements ----

// Set is "set place to value". NewLocal is the local that this statement
// creates, or nil when the place already exists.
type Set struct {
	At
	Place    Place
	Value    Expr
	NewLocal *Local
}

// CallStmt is a phrase call that stands as a statement.
type CallStmt struct {
	At
	Call *Call
}

// RunBlock runs a block parameter of the enclosing phrase ("b").
type RunBlock struct {
	At
	Param *Param
}

// If is "if cond" with its blocks; Else is nil without an "else".
type If struct {
	At
	Cond Expr
	Then *Block
	Else *Block
}

// Each is "each a in x" (B is nil) or "each a, b in x". Over a list the
// names are the index and the item (a single name is the item); over a map
// they are the key and the value (a single name is the key).
type Each struct {
	At
	A, B *Local
	Over Expr
	Body *Block
}

// After is "after delay" with its block.
type After struct {
	At
	Delay Expr
	Body  *Block
}

// Deliver is "deliver rec to node".
type Deliver struct {
	At
	Rec, To Expr
}

// Emit is "emit rec by node".
type Emit struct {
	At
	Rec, By Expr
}

// Result is "result value".
type Result struct {
	At
	Value Expr
}

// Draw is "draw shape as name [for key] [block]". Rebind reports a name that
// already was a shape local of the same scope: the local is reused.
type Draw struct {
	At
	Shape  Expr
	Name   *Local
	Rebind bool
	For    Expr
	Body   *Block
}

// ExprStmt is a value form (an operator, "message", "item", an attachment, a
// name, a literal) standing as a statement; the checker reports it.
type ExprStmt struct {
	At
	X Expr
}

// BadStmt stands for a statement that failed to resolve.
type BadStmt struct{ At }

func (*Set) stmt()      {}
func (*CallStmt) stmt() {}
func (*RunBlock) stmt() {}
func (*If) stmt()       {}
func (*Each) stmt()     {}
func (*After) stmt()    {}
func (*Deliver) stmt()  {}
func (*Emit) stmt()     {}
func (*Result) stmt()   {}
func (*Draw) stmt()     {}
func (*ExprStmt) stmt() {}
func (*BadStmt) stmt()  {}

// ---- places ----

// Place is the target of a set.
type Place interface {
	Node
	place()
}

// LocalPlace is a local variable.
type LocalPlace struct {
	At
	Local *Local
}

// SelectPlace is a member: a node field "n.Known" (the checker rejects
// anything else).
type SelectPlace struct {
	At
	Select *Select
}

// ItemPlace is "item key of base".
type ItemPlace struct {
	At
	Base Place
	Key  Expr
}

func (*LocalPlace) place()  {}
func (*SelectPlace) place() {}
func (*ItemPlace) place()   {}

// BadPlace stands for a place that failed to resolve.
type BadPlace struct{ At }

func (*BadPlace) place() {}

// ---- expressions ----

// LitKind tells what a Lit is.
type LitKind uint8

// The kinds of literal.
const (
	LitText LitKind = iota
	LitNumber
	LitBool
	LitTime
)

// Lit is a text, number, boolean or duration literal.
type Lit struct {
	ExprBase
	Kind LitKind
	// Str is the text without quotes and with escapes resolved.
	Str string
	// Num is the value of a number.
	Num float64
	// Bool is the value of true or false.
	Bool bool
	// Micros is the value of a duration in microseconds.
	Micros int64
}

// ColorLit is a color literal; it stands only as the result of a phrase
// with no holes.
type ColorLit struct {
	ExprBase
	Hex string // "#F4A340"
}

// Ref is a read of a local, a hole parameter, a receiver or a binding.
type Ref struct {
	ExprBase
	Sym Symbol
}

// PkgRef is an imported package, or the current package by its own name,
// used as a value (a "package" hole).
type PkgRef struct {
	ExprBase
	Pkg *Package
}

// FieldRef names a node field in a field hole ("Role", "raft.Role").
type FieldRef struct {
	ExprBase
	Field *Field
}

// EmptyLit is "{}".
type EmptyLit struct{ ExprBase }

// ListLit is "{a, b}".
type ListLit struct {
	ExprBase
	Items []Expr
}

// Part is one name of a member after the first.
type Part struct {
	Name string
	Pos  source.Pos
	// Pkg is set when the name is the qualifier of an imported package
	// ("n.gossip.Known": the part "gossip"). It is only ever Path[0].
	Pkg *Package
}

// Select is a member read: Base followed by Path. Base is a Ref (a local, a
// hole, a receiver, a binding) or a Call of a zero-hole phrase. The checker
// decides from the type of the base whether each part is a node field, a
// record field or a zero-hole receiver phrase. A qualifier of the own
// package ("n.raft.Role" inside raft) is dropped from Path, with a fix.
type Select struct {
	ExprBase
	Base Expr
	Path []Part
}

// Call is a phrase call: a statement or an expression. Args has one entry
// per inline hole of Phrase.Params, Blocks one per block hole. For a
// receiver phrase Receiver is the value that fills the receiver hole.
type Call struct {
	ExprBase
	Phrase   *Phrase
	Receiver Expr
	Args     []Expr
	Blocks   []*Block
	// Qual is the import through which the head was written, or nil.
	Qual *Import
}

// RecordLit is "message T with a x, b y".
type RecordLit struct {
	ExprBase
	Record   *Record
	Settings []Setting
}

// Setting is one "field value" of a RecordLit, in source order.
type Setting struct {
	Field *Field
	Value Expr
	Pos   source.Pos
}

// Item is "item key of coll".
type Item struct {
	ExprBase
	Coll, Key Expr
}

// Binary is an operator form: "is", "is not", "is in", "is not in", "less
// than", "more than", "at least", "at most", "and", "or", "plus", "minus",
// "times", "divided by". Op is the operator as written.
type Binary struct {
	ExprBase
	Op   string
	A, B Expr
	// OpPos is the position of the operator's first word.
	OpPos source.Pos
}

// Not is "not x".
type Not struct {
	ExprBase
	X Expr
}

// Attachment is "name of key": the shape attached under name (look
// context).
type Attachment struct {
	ExprBase
	Name string
	Key  Expr
}

// ActionExpr is a statement form that stands in argument position
// ("(set x to 1)"); it gives no value, and the checker reports it.
type ActionExpr struct {
	ExprBase
	Stmt Stmt
}

// Bad stands for an expression that failed to resolve. It has type Invalid.
type Bad struct{ ExprBase }
