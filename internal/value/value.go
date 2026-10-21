// Package value is the simulator's run-time data: immutable values, their
// equality and total order, and their text forms (numbers, times, Synod
// text, trace text).
//
// Values are persistent by construction. Lists and maps are never changed
// after they are built; every update copies. A frame is copied with a plain
// copy of the []Value (shallow, which is safe because of that).
package value

import (
	"fmt"
	"sort"

	"github.com/synodlang/compiler/internal/ir"
	"github.com/synodlang/compiler/internal/source"
)

// Kind is the run-time kind of a Value.
type Kind uint8

// The kinds of Value, in the order Compare sorts different kinds.
const (
	KInvalid Kind = iota // the zero Value
	KText
	KNumber
	KBool
	KTime
	KNode
	KList
	KMap
	KRecord
	KPackage // Ref is *ir.Package
	KField   // Ref is *ir.Field
	KBlock   // Ref is *Closure
)

// Value is one Synod value. Which fields are used depends on K:
// text uses Str; number uses Num (never -0); time uses Int (microseconds);
// node uses Int (the node id, 0 is sim.Nobody); bool uses Int (0 or 1);
// list, map, record, package, field and block use Ref (*List, *Map,
// *Record, *ir.Package, *ir.Field, *Closure). Values are compared with
// Equal, never with ==.
type Value struct {
	K   Kind
	Num float64
	Int int64
	Str string
	Ref any
}

// List is an immutable list. Never modify Items after construction.
type List struct{ Items []Value }

// Map is an immutable map kept sorted by Compare on the keys. Keys and Vals
// are parallel. Never modify them after construction.
type Map struct{ Keys, Vals []Value }

// Record is an immutable record; Fields has one entry per field of Type, in
// declaration order.
type Record struct {
	Type   *ir.Record
	Fields []Value
}

// Closure is a block with the frame it captured.
type Closure struct {
	Body  *ir.Block
	Frame []Value
}

// Fault is a run-time error: it stops the run and is recorded as sim.Failed.
type Fault struct {
	Code    source.Code
	Message string
	// Pos is where the error was raised; it is only meaningful when HasPos.
	Pos    ir.Pos
	HasPos bool
}

// Error implements error.
func (f *Fault) Error() string { return string(f.Code) + ": " + f.Message }

// Text returns a text value.
func Text(s string) Value { return Value{K: KText, Str: s} }

// Number returns a number value; -0 becomes 0. f must be finite.
func Number(f float64) Value {
	if f == 0 {
		f = 0
	}
	return Value{K: KNumber, Num: f}
}

// Bool returns a bool value.
func Bool(b bool) Value {
	if b {
		return Value{K: KBool, Int: 1}
	}
	return Value{K: KBool}
}

// Time returns a time value of us microseconds.
func Time(us int64) Value { return Value{K: KTime, Int: us} }

// Node returns the node with the given id (0 is sim.Nobody).
func Node(id int) Value { return Value{K: KNode, Int: int64(id)} }

// ListOf returns a list value over items, which the caller gives up.
func ListOf(items []Value) Value { return Value{K: KList, Ref: &List{Items: items}} }

// MapValue returns a map value over m.
func MapValue(m *Map) Value { return Value{K: KMap, Ref: m} }

// RecordOf returns a record value of type t with the given fields, which the
// caller gives up.
func RecordOf(t *ir.Record, fields []Value) Value {
	return Value{K: KRecord, Ref: &Record{Type: t, Fields: fields}}
}

// PackageOf returns a package value.
func PackageOf(p *ir.Package) Value { return Value{K: KPackage, Ref: p} }

// FieldOf returns a field value.
func FieldOf(f *ir.Field) Value { return Value{K: KField, Ref: f} }

// BlockOf returns a block value.
func BlockOf(c *Closure) Value { return Value{K: KBlock, Ref: c} }

// Bool reports the bool held by v.
func (v Value) Bool() bool { return v.Int != 0 }

// NodeID returns the node id held by v.
func (v Value) NodeID() int { return int(v.Int) }

// List returns the list held by v.
func (v Value) List() *List { return v.Ref.(*List) }

// Map returns the map held by v.
func (v Value) Map() *Map { return v.Ref.(*Map) }

// Record returns the record held by v.
func (v Value) Record() *Record { return v.Ref.(*Record) }

// Closure returns the block held by v.
func (v Value) Closure() *Closure { return v.Ref.(*Closure) }

// Get returns the value under k.
func (m *Map) Get(k Value) (Value, bool) {
	i, ok := m.find(k)
	if !ok {
		return Value{}, false
	}
	return m.Vals[i], true
}

// find returns the index of k, or where it would go.
func (m *Map) find(k Value) (int, bool) {
	i := sort.Search(len(m.Keys), func(i int) bool { return Compare(m.Keys[i], k) >= 0 })
	return i, i < len(m.Keys) && Compare(m.Keys[i], k) == 0
}

// With returns a copy of m with k set to v.
func (m *Map) With(k, v Value) *Map {
	i, ok := m.find(k)
	if ok {
		vals := append([]Value(nil), m.Vals...)
		vals[i] = v
		return &Map{Keys: m.Keys, Vals: vals}
	}
	keys := make([]Value, 0, len(m.Keys)+1)
	vals := make([]Value, 0, len(m.Keys)+1)
	keys = append(append(append(keys, m.Keys[:i]...), k), m.Keys[i:]...)
	vals = append(append(append(vals, m.Vals[:i]...), v), m.Vals[i:]...)
	return &Map{Keys: keys, Vals: vals}
}

// Without returns a copy of m without k (m itself when k is absent).
func (m *Map) Without(k Value) *Map {
	i, ok := m.find(k)
	if !ok {
		return m
	}
	keys := append(append([]Value(nil), m.Keys[:i]...), m.Keys[i+1:]...)
	vals := append(append([]Value(nil), m.Vals[:i]...), m.Vals[i+1:]...)
	return &Map{Keys: keys, Vals: vals}
}

// Len returns the number of entries.
func (m *Map) Len() int { return len(m.Keys) }

// NodeName returns the trace name of a node: "n<id>", or "nobody" for 0.
func NodeName(id int) string {
	if id == 0 {
		return "nobody"
	}
	return fmt.Sprintf("n%d", id)
}

// Equal reports structural equality. Blocks are equal only to themselves.
func Equal(a, b Value) bool {
	if a.K == KBlock && b.K == KBlock {
		return a.Ref == b.Ref
	}
	return Compare(a, b) == 0
}

// Compare is a total order on values: different kinds by Kind; numbers
// numerically; times and nodes by integer; text by bytes; false before true;
// lists lexicographically; maps lexicographically over (key, value) pairs;
// records by (package path, name) and then field by field; packages by path;
// fields by package path, record name and name. Blocks are unordered and
// compare equal to each other (the checker never lets them be compared).
func Compare(a, b Value) int {
	if a.K != b.K {
		return cmpInt(int64(a.K), int64(b.K))
	}
	switch a.K {
	case KText:
		return cmpStr(a.Str, b.Str)
	case KNumber:
		switch {
		case a.Num < b.Num:
			return -1
		case a.Num > b.Num:
			return 1
		}
		return 0
	case KBool, KTime, KNode:
		return cmpInt(a.Int, b.Int)
	case KList:
		x, y := a.List().Items, b.List().Items
		for i := 0; i < len(x) && i < len(y); i++ {
			if c := Compare(x[i], y[i]); c != 0 {
				return c
			}
		}
		return cmpInt(int64(len(x)), int64(len(y)))
	case KMap:
		x, y := a.Map(), b.Map()
		for i := 0; i < len(x.Keys) && i < len(y.Keys); i++ {
			if c := Compare(x.Keys[i], y.Keys[i]); c != 0 {
				return c
			}
			if c := Compare(x.Vals[i], y.Vals[i]); c != 0 {
				return c
			}
		}
		return cmpInt(int64(len(x.Keys)), int64(len(y.Keys)))
	case KRecord:
		x, y := a.Record(), b.Record()
		if x.Type != y.Type {
			if c := cmpStr(x.Type.Pkg.Path, y.Type.Pkg.Path); c != 0 {
				return c
			}
			if c := cmpStr(x.Type.Name, y.Type.Name); c != 0 {
				return c
			}
		}
		for i := 0; i < len(x.Fields) && i < len(y.Fields); i++ {
			if c := Compare(x.Fields[i], y.Fields[i]); c != 0 {
				return c
			}
		}
		return cmpInt(int64(len(x.Fields)), int64(len(y.Fields)))
	case KPackage:
		return cmpStr(a.Ref.(*ir.Package).Path, b.Ref.(*ir.Package).Path)
	case KField:
		x, y := a.Ref.(*ir.Field), b.Ref.(*ir.Field)
		if c := cmpStr(x.Pkg.Path, y.Pkg.Path); c != 0 {
			return c
		}
		if c := cmpStr(recName(x), recName(y)); c != 0 {
			return c
		}
		return cmpStr(x.Name, y.Name)
	}
	return 0
}

func recName(f *ir.Field) string {
	if f.Record == nil {
		return ""
	}
	return f.Record.Name
}

func cmpInt(a, b int64) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

func cmpStr(a, b string) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}
