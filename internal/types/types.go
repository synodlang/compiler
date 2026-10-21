// Package types holds the type terms of Synod, the accepted-types relation
// (spec/04 section 4.7), the join used for places with several assignments,
// and the printing of types as the spec writes them.
//
// The package is standalone: it knows nothing of packages, files or the
// syntax tree. Package identity is an import path string, and the relation
// "running p runs q" is supplied by the caller as a Runs function.
package types

import "strings"

// Type is a type term. The implementations are Basic, NodeOf, Record, List,
// Map and Empty. All of them are comparable values.
type Type interface{ isType() }

// Kind names a basic type.
type Kind uint8

// The basic types of spec/04 section 4.2, plus Invalid.
const (
	// Invalid is the type of an expression after an error. It is accepted
	// everywhere, so that one error never cascades.
	Invalid Kind = iota
	Text
	Number
	Bool
	Time
	AnyNode // "node": any node, identity only
	Msg
	Shape
	Color
	Field
	Package
	Block
	Value // "value": any type, in native patterns only
)

var kindNames = [...]string{
	Invalid: "invalid",
	Text:    "text",
	Number:  "number",
	Bool:    "bool",
	Time:    "time",
	AnyNode: "node",
	Msg:     "msg",
	Shape:   "shape",
	Color:   "color",
	Field:   "field",
	Package: "package",
	Block:   "block",
	Value:   "value",
}

// String returns the spelling of the type in source code.
func (k Kind) String() string {
	if int(k) < len(kindNames) {
		return kindNames[k]
	}
	return "invalid"
}

// Basic is a type without parameters.
type Basic struct{ K Kind }

// NodeOf is "Node" or "p.Node": a node running the package with import
// path Pkg.
type NodeOf struct{ Pkg string }

// Record is a record type declared with "has" in the package with import
// path Pkg.
type Record struct{ Pkg, Name string }

// List is "list of T".
type List struct{ Elem Type }

// Map is "map of K to V".
type Map struct{ Key, Val Type }

// Empty is the type of an uninferred "{}". It never escapes the checker.
type Empty struct{}

func (Basic) isType()  {}
func (NodeOf) isType() {}
func (Record) isType() {}
func (List) isType()   {}
func (Map) isType()    {}
func (Empty) isType()  {}

// The basic types as values.
var (
	TInvalid = Basic{Invalid}
	TText    = Basic{Text}
	TNumber  = Basic{Number}
	TBool    = Basic{Bool}
	TTime    = Basic{Time}
	TNode    = Basic{AnyNode}
	TMsg     = Basic{Msg}
	TShape   = Basic{Shape}
	TColor   = Basic{Color}
	TField   = Basic{Field}
	TPackage = Basic{Package}
	TBlock   = Basic{Block}
	TValue   = Basic{Value}
)

// Runs reports whether running package p runs package q: p is q or uses q,
// directly or transitively (spec/05 section 5.1). Import paths are the
// arguments. A nil Runs means that only p == q runs.
type Runs func(p, q string) bool

func (r Runs) runs(p, q string) bool {
	if p == q {
		return true
	}
	return r != nil && r(p, q)
}

// Equal reports whether a and b are the same type. Nil types are equal only
// to each other.
func Equal(a, b Type) bool {
	switch x := a.(type) {
	case nil:
		return b == nil
	case Basic:
		y, ok := b.(Basic)
		return ok && x == y
	case NodeOf:
		y, ok := b.(NodeOf)
		return ok && x == y
	case Record:
		y, ok := b.(Record)
		return ok && x == y
	case List:
		y, ok := b.(List)
		return ok && Equal(x.Elem, y.Elem)
	case Map:
		y, ok := b.(Map)
		return ok && Equal(x.Key, y.Key) && Equal(x.Val, y.Val)
	case Empty:
		_, ok := b.(Empty)
		return ok
	}
	return false
}

func isInvalid(t Type) bool { b, ok := t.(Basic); return ok && b.K == Invalid }

// Accepts reports whether a value of type s is accepted where type t is
// expected (spec/04 section 4.7). It is true in exactly these cases:
//
//  1. s equals t;
//  2. t is value;
//  3. s or t is invalid;
//  4. s is a node type (Node or p.Node) and t is node;
//  5. s is p.Node, t is q.Node, and running p runs q;
//  6. s is a record type and t is msg;
//  7. both are lists and the item type of s is accepted as that of t;
//  8. both are maps and the key and value types of s are accepted as those of t;
//  9. s is the type of "{}" and t is a list or a map.
func Accepts(s, t Type, runs Runs) bool {
	if Equal(s, t) || isInvalid(s) || isInvalid(t) {
		return true
	}
	if b, ok := t.(Basic); ok && b.K == Value {
		return true
	}
	switch x := s.(type) {
	case NodeOf:
		switch y := t.(type) {
		case Basic:
			return y.K == AnyNode
		case NodeOf:
			return runs.runs(x.Pkg, y.Pkg)
		}
	case Record:
		b, ok := t.(Basic)
		return ok && b.K == Msg
	case List:
		if y, ok := t.(List); ok {
			return Accepts(x.Elem, y.Elem, runs)
		}
	case Map:
		if y, ok := t.(Map); ok {
			return Accepts(x.Key, y.Key, runs) && Accepts(x.Val, y.Val, runs)
		}
	case Empty:
		switch t.(type) {
		case List, Map:
			return true
		}
	}
	return false
}

// Join returns the type that accepts both a and b, or false when there is
// none:
//
//   - if a is accepted as b, the result is b; else if b is accepted as a, it
//     is a;
//   - two lists join item-wise, and two maps join key-wise and value-wise;
//   - otherwise the join fails. Two unrelated node types do not join to node.
//
// An invalid operand joins to the other operand, so that errors do not
// cascade.
func Join(a, b Type, runs Runs) (Type, bool) {
	switch {
	case isInvalid(a):
		return b, true
	case isInvalid(b):
		return a, true
	case Accepts(a, b, runs):
		return b, true
	case Accepts(b, a, runs):
		return a, true
	}
	switch x := a.(type) {
	case List:
		if y, ok := b.(List); ok {
			if e, ok := Join(x.Elem, y.Elem, runs); ok {
				return List{e}, true
			}
		}
	case Map:
		if y, ok := b.(Map); ok {
			k, kok := Join(x.Key, y.Key, runs)
			v, vok := Join(x.Val, y.Val, runs)
			if kok && vok {
				return Map{k, v}, true
			}
		}
	}
	return nil, false
}

// Format prints t as the spec does: "text", "list of map of text to number",
// "Node" or "Prompt" for the types of package from (an import path), and
// "raft.Node" or "api.Get" for those of another package. qual returns the
// qualifier under which the current file knows a package; if it is nil or
// returns "", the last segment of the import path is used.
func Format(t Type, from string, qual func(pkgPath string) string) string {
	var b strings.Builder
	format(&b, t, from, qual)
	return b.String()
}

func format(b *strings.Builder, t Type, from string, qual func(string) string) {
	named := func(pkg, name string) {
		if pkg != from && pkg != "" {
			q := ""
			if qual != nil {
				q = qual(pkg)
			}
			if q == "" {
				q = pkg[strings.LastIndexByte(pkg, '/')+1:]
			}
			b.WriteString(q)
			b.WriteByte('.')
		}
		b.WriteString(name)
	}
	switch x := t.(type) {
	case nil:
		b.WriteString("invalid")
	case Basic:
		b.WriteString(x.K.String())
	case NodeOf:
		named(x.Pkg, "Node")
	case Record:
		named(x.Pkg, x.Name)
	case List:
		b.WriteString("list of ")
		format(b, x.Elem, from, qual)
	case Map:
		b.WriteString("map of ")
		format(b, x.Key, from, qual)
		b.WriteString(" to ")
		format(b, x.Val, from, qual)
	case Empty:
		b.WriteString("{}")
	}
}
