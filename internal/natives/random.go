package natives

import (
	"math"

	"github.com/synodlang/compiler/internal/ir"
	"github.com/synodlang/compiler/internal/source"
	"github.com/synodlang/compiler/internal/value"
)

func init() {
	Table["random.pick"] = randomPick
	Table["random.between"] = randomBetween
	Table["random.timeBetween"] = randomTimeBetween
	Table["random.chance"] = randomChance
}

// Every random native that draws consumes exactly one value of the
// generator; pick on an empty list draws none (it faults first).

func randomPick(h Host, c *ir.Call, args []value.Value) (value.Value, *value.Fault) {
	items := args[0].List().Items
	if len(items) == 0 {
		return value.Value{}, fault(c, source.IndexRange, "random.Pick of an empty list")
	}
	return items[h.RNG().Index(len(items))], nil
}

func randomBetween(h Host, c *ir.Call, args []value.Value) (value.Value, *value.Fault) {
	a, b := args[0].Num, args[1].Num
	if b < a {
		return value.Value{}, fault(c, source.BadArgument, "random.Between needs a ≤ b")
	}
	r := h.RNG().Between(a, b)
	if math.IsInf(r, 0) || math.IsNaN(r) {
		return value.Value{}, fault(c, source.NumberRange, "the result is too large for a number")
	}
	return value.Number(r), nil
}

func randomTimeBetween(h Host, c *ir.Call, args []value.Value) (value.Value, *value.Fault) {
	a, b := args[0].Int, args[1].Int
	if b < a {
		return value.Value{}, fault(c, source.BadArgument, "random.TimeBetween needs a ≤ b")
	}
	return value.Time(h.RNG().TimeBetween(a, b)), nil
}

func randomChance(h Host, c *ir.Call, args []value.Value) (value.Value, *value.Fault) {
	return value.Bool(h.RNG().Chance(args[0].Num)), nil
}
