package natives

import (
	"math"

	"github.com/synodlang/compiler/internal/ir"
	"github.com/synodlang/compiler/internal/source"
	"github.com/synodlang/compiler/internal/value"
)

// MaxItems bounds the size of a list or text a native may build; a larger
// result is a run-limit fault.
const MaxItems = 1_000_000

// maxSeed is the largest seed sim.Seed accepts: 2^53, the largest range in
// which every whole number is exactly representable.
const maxSeed = 1 << 53

// fault builds a run-time fault positioned at the call.
func fault(c *ir.Call, code source.Code, msg string) *value.Fault {
	return &value.Fault{Code: code, Message: msg, Pos: c.Pos(), HasPos: true}
}

// whole reports whether f is a finite whole number.
func whole(f float64) bool { return !math.IsInf(f, 0) && f == math.Floor(f) }

// nobody faults when the node argument is sim.Nobody; what names the use,
// for example "sim.Crash of".
func nobody(c *ir.Call, v value.Value, what string) *value.Fault {
	if v.NodeID() == 0 {
		return fault(c, source.Nobody, what+" sim.Nobody")
	}
	return nil
}

// nodeList is a list of node values.
func nodeList(ids []int) value.Value {
	items := make([]value.Value, len(ids))
	for i, id := range ids {
		items[i] = value.Node(id)
	}
	return value.ListOf(items)
}

// nodeIDs reads a list of nodes; a Nobody item faults with what.
func nodeIDs(c *ir.Call, v value.Value, what string) ([]int, *value.Fault) {
	items := v.List().Items
	ids := make([]int, len(items))
	for i, it := range items {
		if f := nobody(c, it, what); f != nil {
			return nil, f
		}
		ids[i] = it.NodeID()
	}
	return ids, nil
}
