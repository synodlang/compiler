// Package ir is the typed, resolved program that the simulator executes.
//
// The checker (internal/check) lowers a program that has no diagnostics into
// this form. Everything the simulator needs is explicit here, so it never
// looks at source text, phrase patterns, names or scopes:
//
//   - every name is a *Var with a frame Slot, a *Field, a *Proc or a *Package;
//   - every expression has a type (Type) and a position (Pos);
//   - every phrase call names the *Proc it calls, and a native phrase is a
//     *Proc with a Native id and no Body;
//   - member reads are split into NodeField (state of a node), RecordField
//     (a field of an immutable record) and Call (a receiver phrase with no
//     holes); node fields and record fields carry their field index;
//   - "{}" is an Empty that carries its list or map type; "message T with …"
//     is a MakeRecord with one value per field.
//
// # Programs
//
// A Program is the entry package (a main file or a library) together with
// the packages it uses. Packages are in dependency order. "Running" a
// package (spec/05 section 5.1) runs every package of Package.Runs: a node
// gets the NodeFields of each of them, and its start and event handlers run
// in that order.
//
// # Frames and closures
//
// A Proc, Handler or Invariant has a frame of Slots values. Receiver,
// Params, BlockParams, bindings and locals all have a Slot in that frame;
// the numbers are unique within one body. An After statement captures a copy
// of the frame when it runs (spec/07 section 7.4), and its Body uses the
// frame's slot numbers. A Call with Blocks passes each block together with a
// copy of the caller's frame; RunBlock{Var} in the callee runs the block
// held in that BlockParams variable, with the captured frame.
//
// # Contexts
//
// The checker has already proved that every statement is allowed in its
// context (node, main, check or look), that node code touches only its
// receiver's state, and that all types fit. The simulator may rely on this:
// the only run-time errors left are those of spec/11 section 11.3 (missing
// key, index range, disk type, division by zero, nobody, time limit).
package ir

import "github.com/synodlang/compiler/internal/types"

// Pos is a source position, for run-time errors and traces. The trace
// records the start only; EndLine and EndCol (exclusive, code points; zero
// when unknown) give the extent for the diagnostics of run-time errors.
type Pos struct {
	File            string
	Line, Col       int
	EndLine, EndCol int
	// Std is true for a position in a std package (decision 89).
	Std bool
}

// Program is one entry package with the packages it needs.
type Program struct {
	// Name is the import path of the entry package; a main package has
	// "#main" appended.
	Name string
	// Main is "define main", or nil for a library.
	Main *Proc
	// Packages are the entry package and everything it uses, transitively,
	// dependencies first.
	Packages []*Package
	// Observers are the trace-event handlers of all packages, in package
	// order and then file order.
	Observers []*Handler
	// Invariants are the always lines of all packages, in the same order.
	Invariants []*Invariant
}

// Package is the lowered form of one package.
type Package struct {
	Path, Name string
	IsStd      bool
	IsMain     bool
	// Uses are the packages this one imports, sorted by Path.
	Uses []*Package
	// Runs is the transitive closure of Uses including the package itself,
	// in dependency order: the packages a node of this package's type runs,
	// in start order and handler order.
	Runs []*Package
	// NodeFields are the fields of Node, in declaration order (Field.Index).
	NodeFields []*Field
	// Records are the record types, in declaration order.
	Records []*Record
	// Start is "define [n: Node] start", or nil.
	Start *Proc
	// NodeHandlers are the node-event whens (net.Received, sim.Delivered),
	// in file order.
	NodeHandlers []*Handler
	// Observers are the trace-event whens (look context), in file order.
	Observers []*Handler
	// Invariants are the always lines, in file order.
	Invariants []*Invariant
	// Phrases are all phrases except start and main: natives, shared phrases
	// and receiver phrases, in file order.
	Phrases []*Proc
}

// Record is a record type declared with "has".
type Record struct {
	Pkg      *Package
	Name     string
	Exported bool
	Fields   []*Field
}

// Field is a node field (Record is nil) or a record field.
type Field struct {
	Pkg      *Package
	Record   *Record
	Name     string
	Exported bool
	// Index is the position in Package.NodeFields or Record.Fields.
	Index int
	Type  types.Type
	// Init is the initial value, a constant expression.
	Init Expr
}

// ProcKind tells what a Proc is.
type ProcKind uint8

// The kinds of Proc.
const (
	ProcPhrase ProcKind = iota // a defined phrase (with or without receiver)
	ProcStart                  // define [n: Node] start
	ProcMain                   // define main
	ProcNative                 // a native phrase; Native names the primitive
)

// Proc is a phrase: its signature and, for a phrase written in Synod, its
// body.
type Proc struct {
	// Name is the pattern as the spec writes it ("Learn … at").
	Name string
	Pkg  *Package
	Kind ProcKind
	// Receiver is the node hole before the head, or nil.
	Receiver *Var
	// Params are the inline holes, in order; BlockParams the block holes.
	Params, BlockParams []*Var
	// Result is the type of the value the phrase gives; nil for an action.
	Result types.Type
	// Native is the native id ("net.send"); empty for a phrase in Synod.
	Native string
	// Body is nil for a native.
	Body *Block
	// Slots is the size of the frame; Vars lists its variables by slot.
	Slots int
	Vars  []*Var
	Pos   Pos
}

// VarKind tells how a Var was introduced.
type VarKind uint8

// The kinds of Var.
const (
	VarReceiver VarKind = iota
	VarParam
	VarBlock   // a block hole
	VarLocal   // set
	VarEach    // an each name
	VarDraw    // a draw name (type shape)
	VarBinding // a name bound by a when
)

// Var is a slot of a frame.
type Var struct {
	Name string
	Kind VarKind
	Type types.Type // nil for VarBlock
	Slot int
}

// Block is a sequence of statements.
type Block struct{ Stmts []Stmt }

// Handler is a "when". For a node event (NodeEvent true) it runs on every
// node that runs Pkg, with Receiver bound to the node, in node context. For
// a trace event it runs once per occurrence, in look context, with no
// receiver.
type Handler struct {
	Pkg *Package
	// Event is the native id of the event ("net.received", "sim.changed").
	Event     string
	NodeEvent bool
	Receiver  *Var
	// Match has one entry per inline hole of the event phrase, in order.
	Match []Matcher
	Body  *Block
	Slots int
	Vars  []*Var
	Pos   Pos
}

// MatchKind tells how an event hole is matched (spec/05 section 5.6).
type MatchKind uint8

// The kinds of matcher.
const (
	MatchBind     MatchKind = iota // bind Var to the value
	MatchRecord                    // the value is a record of Record; bind Var to it
	MatchNodeType                  // the node runs NodePkg; bind Var to it
	MatchField                     // the changed field is Field
	MatchEqual                     // the value equals Value
)

// Matcher is the treatment of one hole of an event.
type Matcher struct {
	Hole    int
	Kind    MatchKind
	Var     *Var     // MatchBind, MatchRecord, MatchNodeType
	Record  *Record  // MatchRecord
	NodePkg *Package // MatchNodeType
	Field   *Field   // MatchField
	Value   Expr     // MatchEqual
}

// Invariant is an "always" line: Cond is evaluated in check context after
// every step.
type Invariant struct {
	Pkg   *Package
	Cond  Expr
	Slots int
	// Rule is the text recorded in sim.Broken: "<path>:<line>: always <arg>".
	Rule string
	Pos  Pos
}

// ---- statements ----

// Stmt is a statement.
type Stmt interface {
	// Pos is the position of the statement.
	Pos() Pos
	stmt()
}

// StmtPos is embedded in every statement.
type StmtPos struct{ At Pos }

// Pos implements Stmt.
func (s StmtPos) Pos() Pos { return s.At }
func (StmtPos) stmt()      {}

// Place is something a set can assign: a local, a node field, or an item
// of one of them.
type Place interface {
	Type() types.Type
	place()
}

// VarPlace is a local variable.
type VarPlace struct{ Var *Var }

// FieldPlace is a node field of Node.
type FieldPlace struct {
	Node  Expr
	Field *Field
}

// Type implements Place.
func (p *VarPlace) Type() types.Type { return p.Var.Type }

// Type implements Place.
func (p *FieldPlace) Type() types.Type { return p.Field.Type }
func (*VarPlace) place()               {}
func (*FieldPlace) place()             {}

// SetLocal is "set x to v" for a local.
type SetLocal struct {
	StmtPos
	Var *Var
	X   Expr
}

// SetField is "set n.F to v" for a node field. Node evaluates to the node.
type SetField struct {
	StmtPos
	Node  Expr
	Field *Field
	X     Expr
}

// SetItem is "set item k of P to v": it replaces the value of Base with a
// copy updated at Key. For a list the key is an index; for a map, a key.
type SetItem struct {
	StmtPos
	Base  Place
	Key   Expr
	X     Expr
	IsMap bool
}

// CallStmt calls an action phrase.
type CallStmt struct {
	StmtPos
	Call *Call
}

// RunBlock runs the block held by a block parameter of the enclosing Proc.
type RunBlock struct {
	StmtPos
	Var *Var
}

// If is "if cond"; Else is nil without an else.
type If struct {
	StmtPos
	Cond       Expr
	Then, Else *Block
}

// Each is a loop. Over a list, Key is the index (nil when the loop has one
// name) and Val the item; over a map, Key is the key and Val the value (nil
// when the loop has one name). Maps iterate in ascending key order.
type Each struct {
	StmtPos
	Key, Val *Var
	Over     Expr
	IsMap    bool
	Body     *Block
}

// After schedules Body at now plus Delay, with a copy of the frame.
type After struct {
	StmtPos
	Delay Expr
	Body  *Block
}

// Deliver injects a record into a node (main context).
type Deliver struct {
	StmtPos
	Rec, To Expr
}

// Emit records a trace fact "sim.Emitted" by a node.
type Emit struct {
	StmtPos
	Rec, By Expr
}

// Result ends a value phrase with X.
type Result struct {
	StmtPos
	X Expr
}

// Draw creates a shape into Var (look context). For attaches it to a node,
// text or number key. Reactive, if not nil, is re-run whenever state it reads
// changes.
type Draw struct {
	StmtPos
	Shape    Expr
	Var      *Var
	For      Expr
	Name     string // the attachment name when For is set
	Rebind   bool
	Reactive *Block
}

// ---- expressions ----

// Expr is an expression.
type Expr interface {
	Type() types.Type
	Pos() Pos
	expr()
}

// Base is embedded in every expression.
type Base struct {
	Ty types.Type
	At Pos
}

// Type implements Expr.
func (b Base) Type() types.Type { return b.Ty }

// Pos implements Expr.
func (b Base) Pos() Pos { return b.At }
func (Base) expr()      {}

// Const is a literal. V is a string (text), float64 (number), bool, int64
// (time, in microseconds) or, for a color, a string "#RRGGBB" with Type
// color.
type Const struct {
	Base
	V any
}

// Empty is "{}" with the list or map type it was given.
type Empty struct{ Base }

// ListLit is "{a, b}".
type ListLit struct {
	Base
	Items []Expr
}

// Load reads a variable.
type Load struct {
	Base
	Var *Var
}

// NodeField reads a field of a node: the state of Node at this moment.
type NodeField struct {
	Base
	Node  Expr
	Field *Field
}

// RecordField reads a field of a record value.
type RecordField struct {
	Base
	Rec   Expr
	Field *Field
}

// MakeRecord is "message T with …". Values has one entry per field of
// Record, in field order; nil means that the field keeps its initial value.
type MakeRecord struct {
	Base
	Record *Record
	Values []Expr
}

// Item is "item k of coll".
type Item struct {
	Base
	Coll, Key Expr
	IsMap     bool
}

// ArithOp is an arithmetic operator.
type ArithOp uint8

// The arithmetic operators.
const (
	Plus ArithOp = iota
	Minus
	Times
	Divide
)

// Arith is "a plus b", "a minus b", "a times b", "a divided by b". Operand
// types tell the unit: number or time (A.Type(), B.Type()); the result type
// is Type().
type Arith struct {
	Base
	Op   ArithOp
	A, B Expr
}

// CmpOp is a comparison operator.
type CmpOp uint8

// The comparison operators.
const (
	Eq CmpOp = iota
	Ne
	Lt // less than
	Gt // more than
	Le // at most
	Ge // at least
)

// Compare is "is", "is not", "less than", "more than", "at most",
// "at least". Equality is structural: texts, numbers, bools, times, nodes
// (identity), records and collections compare by value.
type Compare struct {
	Base
	Op   CmpOp
	A, B Expr
}

// Contains is "x is in coll" (list items or map keys); Not is "is not in".
type Contains struct {
	Base
	Elem, Coll Expr
	IsMap      bool
	Not        bool
}

// Logic is "a and b" (And) or "a or b"; both sides are evaluated lazily.
type Logic struct {
	Base
	And  bool
	A, B Expr
}

// Not is "not x".
type Not struct {
	Base
	X Expr
}

// Call calls a phrase. Recv is the receiver (for a receiver phrase), Args
// match Proc.Params, Blocks match Proc.BlockParams; each block is passed
// with a copy of the caller's frame.
type Call struct {
	Base
	Proc   *Proc
	Recv   Expr
	Args   []Expr
	Blocks []*Block
}

// PackageVal is a package used as a value (a "package" hole).
type PackageVal struct {
	Base
	Pkg *Package
}

// FieldVal is a node field named as a value (a "field" hole).
type FieldVal struct {
	Base
	Field *Field
}

// Attachment is "name of key": the shape attached under Name to Key (look
// context).
type Attachment struct {
	Base
	Name string
	Key  Expr
}
