package natives

import (
	"sort"

	"github.com/synodlang/compiler/internal/ir"
	"github.com/synodlang/compiler/internal/value"
)

func init() {
	Table["maps.size"] = func(h Host, c *ir.Call, args []value.Value) (value.Value, *value.Fault) {
		return value.Number(float64(args[0].Map().Len())), nil
	}
	Table["maps.keys"] = func(h Host, c *ir.Call, args []value.Value) (value.Value, *value.Fault) {
		return value.ListOf(append([]value.Value{}, args[0].Map().Keys...)), nil
	}
	Table["maps.without"] = func(h Host, c *ir.Call, args []value.Value) (value.Value, *value.Fault) {
		return value.MapValue(args[1].Map().Without(args[0])), nil
	}
	Table["maps.keysByValue"] = mapsKeysByValue
}

// mapsKeysByValue orders keys by ascending value, ties by key; the keys of a
// map are already ascending, so a stable sort on the values is enough.
func mapsKeysByValue(h Host, c *ir.Call, args []value.Value) (value.Value, *value.Fault) {
	m := args[0].Map()
	idx := make([]int, m.Len())
	for i := range idx {
		idx[i] = i
	}
	sort.SliceStable(idx, func(i, j int) bool { return value.Compare(m.Vals[idx[i]], m.Vals[idx[j]]) < 0 })
	keys := make([]value.Value, len(idx))
	for i, k := range idx {
		keys[i] = m.Keys[k]
	}
	return value.ListOf(keys), nil
}
