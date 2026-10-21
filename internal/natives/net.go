package natives

import (
	"github.com/synodlang/compiler/internal/ir"
	"github.com/synodlang/compiler/internal/source"
	"github.com/synodlang/compiler/internal/value"
)

func init() {
	Table["net.send"] = netSend
	Table["net.latency"] = netLatency
	Table["net.loss"] = netLoss
	Table["net.partition"] = netPartition
	Table["net.heal"] = netHeal
	Table["net.dropNext"] = netDropNext
}

// netSend: Send [m] from [a] to [b].
func netSend(h Host, c *ir.Call, args []value.Value) (value.Value, *value.Fault) {
	if f := nobody(c, args[1], "net.Send from"); f != nil {
		return value.Value{}, f
	}
	if f := nobody(c, args[2], "net.Send to"); f != nil {
		return value.Value{}, f
	}
	h.Send(args[0].Record(), args[1].NodeID(), args[2].NodeID())
	return value.Value{}, nil
}

func netLatency(h Host, c *ir.Call, args []value.Value) (value.Value, *value.Fault) {
	a, b := args[0].Int, args[1].Int
	if a < 0 || b < a {
		return value.Value{}, fault(c, source.BadArgument, "net.Latency needs 0 ≤ a ≤ b")
	}
	h.SetLatency(a, b)
	return value.Value{}, nil
}

func netLoss(h Host, c *ir.Call, args []value.Value) (value.Value, *value.Fault) {
	p := args[0].Num
	if !(p >= 0 && p <= 1) {
		return value.Value{}, fault(c, source.BadArgument, "net.Loss needs 0 ≤ p ≤ 1")
	}
	h.SetLoss(p)
	return value.Value{}, nil
}

func netPartition(h Host, c *ir.Call, args []value.Value) (value.Value, *value.Fault) {
	a, f := nodeIDs(c, args[0], "net.Partition with")
	if f != nil {
		return value.Value{}, f
	}
	b, f := nodeIDs(c, args[1], "net.Partition with")
	if f != nil {
		return value.Value{}, f
	}
	h.Partition(a, b)
	return value.Value{}, nil
}

func netHeal(h Host, c *ir.Call, args []value.Value) (value.Value, *value.Fault) {
	h.Heal()
	return value.Value{}, nil
}

func netDropNext(h Host, c *ir.Call, args []value.Value) (value.Value, *value.Fault) {
	if f := nobody(c, args[1], "net.DropNext from"); f != nil {
		return value.Value{}, f
	}
	if f := nobody(c, args[2], "net.DropNext to"); f != nil {
		return value.Value{}, f
	}
	h.DropNext(args[0].Str, args[1].NodeID(), args[2].NodeID())
	return value.Value{}, nil
}
