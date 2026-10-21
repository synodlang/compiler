package natives

import (
	"github.com/synodlang/compiler/internal/ir"
	"github.com/synodlang/compiler/internal/source"
	"github.com/synodlang/compiler/internal/value"
)

func init() {
	Table["sim.seed"] = simSeed
	Table["sim.stopAt"] = simStopAt
	Table["sim.spawn"] = simSpawn
	Table["sim.cluster"] = simCluster
	Table["sim.crash"] = nodeAction("sim.Crash of", Host.Crash)
	Table["sim.restart"] = nodeAction("sim.Restart of", Host.Restart)
	Table["sim.remove"] = nodeAction("sim.Remove of", Host.Remove)
	Table["sim.id"] = simID
	Table["sim.name"] = simName
	Table["sim.nobody"] = simNobody
	Table["sim.nodes"] = simNodes
	Table["sim.nodesRunning"] = simNodesRunning
	Table["sim.isUp"] = simIsUp
}

func simSeed(h Host, c *ir.Call, args []value.Value) (value.Value, *value.Fault) {
	n := args[0].Num
	if !whole(n) || n < 0 || n > maxSeed {
		return value.Value{}, withArg(fault(c, source.BadArgument, "sim.Seed needs a whole number from 0 to 2^53"), c, args, 0, "is "+show(args[0]))
	}
	h.Seed(uint64(n))
	return value.Value{}, nil
}

func simStopAt(h Host, c *ir.Call, args []value.Value) (value.Value, *value.Fault) {
	if args[0].Int < 0 {
		return value.Value{}, withArg(fault(c, source.BadArgument, "sim.StopAt needs a time of 0 or more"), c, args, 0, "is "+show(args[0]))
	}
	h.StopAt(args[0].Int)
	return value.Value{}, nil
}

func simSpawn(h Host, c *ir.Call, args []value.Value) (value.Value, *value.Fault) {
	return value.Node(h.Spawn(args[0].Ref.(*ir.Package))), nil
}

func simCluster(h Host, c *ir.Call, args []value.Value) (value.Value, *value.Fault) {
	n := args[0].Num
	if !whole(n) || n < 0 {
		return value.Value{}, withArg(fault(c, source.BadArgument, "sim.Cluster needs a whole count of 0 or more"), c, args, 0, "is "+show(args[0]))
	}
	if n > MaxItems {
		return value.Value{}, fault(c, source.RunLimit, "sim.Cluster of more than 1000000 nodes")
	}
	p := args[1].Ref.(*ir.Package)
	items := make([]value.Value, int(n))
	for i := range items {
		items[i] = value.Node(h.Spawn(p))
	}
	return value.ListOf(items), nil
}

func nodeAction(what string, f func(Host, int)) Func {
	return func(h Host, c *ir.Call, args []value.Value) (value.Value, *value.Fault) {
		if fl := nobody(c, args, 0, what); fl != nil {
			return value.Value{}, fl
		}
		f(h, args[0].NodeID())
		return value.Value{}, nil
	}
}

func simID(h Host, c *ir.Call, args []value.Value) (value.Value, *value.Fault) {
	return value.Number(float64(args[0].NodeID())), nil
}

func simName(h Host, c *ir.Call, args []value.Value) (value.Value, *value.Fault) {
	return value.Text(value.NodeName(args[0].NodeID())), nil
}

func simNobody(h Host, c *ir.Call, args []value.Value) (value.Value, *value.Fault) {
	return value.Node(0), nil
}

func simNodes(h Host, c *ir.Call, args []value.Value) (value.Value, *value.Fault) {
	return nodeList(h.Nodes()), nil
}

func simNodesRunning(h Host, c *ir.Call, args []value.Value) (value.Value, *value.Fault) {
	return nodeList(h.NodesRunning(args[0].Ref.(*ir.Package))), nil
}

func simIsUp(h Host, c *ir.Call, args []value.Value) (value.Value, *value.Fault) {
	id := args[0].NodeID()
	return value.Bool(id != 0 && h.IsUp(id)), nil
}
