package natives

import (
	"sort"

	"github.com/synodlang/compiler/internal/ir"
	"github.com/synodlang/compiler/internal/source"
	"github.com/synodlang/compiler/internal/value"
)

func init() {
	Table["lists.size"] = func(h Host, c *ir.Call, args []value.Value) (value.Value, *value.Fault) {
		return value.Number(float64(len(args[0].List().Items))), nil
	}
	Table["lists.append"] = listsAppend
	Table["lists.extend"] = listsExtend
	Table["lists.without"] = listsWithout
	Table["lists.take"] = listsTake
	Table["lists.range"] = listsRange
	Table["lists.sort"] = listsSort
}

func tooLong(c *ir.Call, what string) *value.Fault {
	return fault(c, source.RunLimit, what+" would make a list of more than 1000000 items")
}

// listsAppend: Append [x] to [l].
func listsAppend(h Host, c *ir.Call, args []value.Value) (value.Value, *value.Fault) {
	old := args[1].List().Items
	if len(old)+1 > MaxItems {
		return value.Value{}, tooLong(c, "lists.Append")
	}
	items := make([]value.Value, len(old), len(old)+1)
	copy(items, old)
	return value.ListOf(append(items, args[0])), nil
}

// listsExtend: Extend [a] with [b].
func listsExtend(h Host, c *ir.Call, args []value.Value) (value.Value, *value.Fault) {
	a, b := args[0].List().Items, args[1].List().Items
	if len(a)+len(b) > MaxItems {
		return value.Value{}, tooLong(c, "lists.Extend")
	}
	items := make([]value.Value, 0, len(a)+len(b))
	items = append(items, a...)
	return value.ListOf(append(items, b...)), nil
}

// listsWithout: Without [x] from [l].
func listsWithout(h Host, c *ir.Call, args []value.Value) (value.Value, *value.Fault) {
	old := args[1].List().Items
	items := make([]value.Value, 0, len(old))
	for _, it := range old {
		if !value.Equal(it, args[0]) {
			items = append(items, it)
		}
	}
	return value.ListOf(items), nil
}

// listsTake: Take [n] from [l].
func listsTake(h Host, c *ir.Call, args []value.Value) (value.Value, *value.Fault) {
	n := args[0].Num
	if !whole(n) || n < 0 {
		return value.Value{}, fault(c, source.BadArgument, "lists.Take needs a whole count of 0 or more")
	}
	old := args[1].List().Items
	if n >= float64(len(old)) {
		return args[1], nil
	}
	return value.ListOf(append([]value.Value{}, old[:int(n)]...)), nil
}

// listsRange: Range [a] to [b].
func listsRange(h Host, c *ir.Call, args []value.Value) (value.Value, *value.Fault) {
	a, b := args[0].Num, args[1].Num
	if !whole(a) || !whole(b) {
		return value.Value{}, fault(c, source.BadArgument, "lists.Range needs whole numbers")
	}
	if a > b {
		return value.ListOf([]value.Value{}), nil
	}
	if b-a+1 > MaxItems {
		return value.Value{}, tooLong(c, "lists.Range")
	}
	items := make([]value.Value, int(b-a)+1)
	for i := range items {
		items[i] = value.Number(a + float64(i))
	}
	return value.ListOf(items), nil
}

// listsSort: Sort [l], ascending and stable.
func listsSort(h Host, c *ir.Call, args []value.Value) (value.Value, *value.Fault) {
	old := args[0].List().Items
	items := make([]value.Value, len(old))
	copy(items, old)
	for _, it := range items {
		if (it.K != value.KNumber && it.K != value.KTime && it.K != value.KText) || it.K != items[0].K {
			return value.Value{}, fault(c, source.BadArgument, "lists.Sort needs items that are all numbers, all times or all text")
		}
	}
	sort.SliceStable(items, func(i, j int) bool { return value.Compare(items[i], items[j]) < 0 })
	return value.ListOf(items), nil
}
