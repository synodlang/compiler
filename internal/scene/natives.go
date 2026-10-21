package scene

import (
	"github.com/synodlang/compiler/internal/ir"
	"github.com/synodlang/compiler/internal/natives"
	"github.com/synodlang/compiler/internal/rng"
	"github.com/synodlang/compiler/internal/source"
	"github.com/synodlang/compiler/internal/trace"
	"github.com/synodlang/compiler/internal/types"
	"github.com/synodlang/compiler/internal/value"
)

// Func is one look-only native phrase. b is the running look (an opaque
// handle outside this package); c and args are as for natives.Func.
type Func func(b *builder, c *ir.Call, args []value.Value) (value.Value, *value.Fault)

// Table maps the id of every look-only native (look.*, sim.stateRows, net.id,
// net.type: those of stdlib.Natives whose Contexts are exactly stdlib.Look)
// to its implementation. The other natives of look context (sim.nodes,
// text.from, …) come from natives.Table, running on lookHost.
var Table = map[string]Func{}

func init() {
	Table["look.circle"] = lookCircle
	Table["look.rect"] = lookRect
	Table["look.label"] = lookLabel
	Table["look.line"] = lookLine
	Table["look.group"] = lookGroup
	Table["look.center"] = lookCenter
	Table["look.fill"] = lookPaint(trace.PFill)
	Table["look.stroke"] = lookPaint(trace.PStroke)
	Table["look.tween"] = lookTween
	Table["look.effect"] = lookEffect
	Table["look.layout"] = lookLayout
	Table["look.placeAbove"] = lookPlace("above")
	Table["look.placeBelow"] = lookPlace("below")
	Table["look.placeBeside"] = lookPlace("beside")
	Table["look.orbit"] = lookOrbit
	Table["look.flight"] = lookFlight
	Table["look.delete"] = lookDelete
	Table["sim.stateRows"] = simStateRows
	Table["net.id"] = netID
	Table["net.type"] = netType
}

// wf turns an error of the world into a fault at the call.
func (b *builder) wf(err error, c *ir.Call) *value.Fault {
	if err == nil {
		return nil
	}
	return b.worldFault(err, c.Pos())
}

func badArgument(c *ir.Call, format string, a ...any) *value.Fault {
	return fault(source.BadArgument, c.Pos(), format, a...)
}

// newShape creates a shape of the running activation; a shape drawn by a
// reactive block belongs to its instance.
func (b *builder) newShape(sp shapeSpec) (int, error) {
	i, err := b.world.newShape(sp, b.act.next(), b.now)
	if err == nil && b.inst != nil {
		b.inst.owned = append(b.inst.owned, i)
	}
	return i, err
}

func lookShape(b *builder, c *ir.Call, sp shapeSpec) (value.Value, *value.Fault) {
	i, err := b.newShape(sp)
	if err != nil {
		return value.Value{}, b.wf(err, c)
	}
	return value.Shape(i), nil
}

func lookCircle(b *builder, c *ir.Call, args []value.Value) (value.Value, *value.Fault) {
	return lookShape(b, c, shapeSpec{Kind: trace.KindCircle, R: args[0].Num})
}

func lookRect(b *builder, c *ir.Call, args []value.Value) (value.Value, *value.Fault) {
	return lookShape(b, c, shapeSpec{Kind: trace.KindRect, W: args[0].Num, H: args[1].Num})
}

func lookLabel(b *builder, c *ir.Call, args []value.Value) (value.Value, *value.Fault) {
	return lookShape(b, c, shapeSpec{Kind: trace.KindLabel, Text: args[0].Str})
}

func lookLine(b *builder, c *ir.Call, args []value.Value) (value.Value, *value.Fault) {
	return lookShape(b, c, shapeSpec{Kind: trace.KindLine, From: int(args[0].Int), To: int(args[1].Int)})
}

func lookGroup(b *builder, c *ir.Call, args []value.Value) (value.Value, *value.Fault) {
	items := args[0].List().Items
	ms := make([]int, len(items))
	for i, it := range items {
		ms[i] = int(it.Int)
	}
	g, err := b.world.group(ms, b.act.next(), b.now)
	if err != nil {
		return value.Value{}, b.wf(err, c)
	}
	if b.inst != nil {
		b.inst.owned = append(b.inst.owned, g)
	}
	return value.Shape(g), nil
}

func lookCenter(b *builder, c *ir.Call, args []value.Value) (value.Value, *value.Fault) {
	i, err := b.world.center()
	if err != nil {
		return value.Value{}, b.wf(err, c)
	}
	return value.Shape(i), nil
}

func lookPaint(p uint16) Func {
	return func(b *builder, c *ir.Call, args []value.Value) (value.Value, *value.Fault) {
		return value.Value{}, b.wf(b.world.set(int(args[0].Int), p, pv{S: args[1].Str}, b.now), c)
	}
}

// props are the property names look.Tween accepts.
var props = map[string]uint16{
	"x": trace.PX, "y": trace.PY, "scale": trace.PScale, "opacity": trace.POpacity,
	"rotation": trace.PRotation, "fill": trace.PFill, "stroke": trace.PStroke, "text": trace.PText,
}

// ms converts a time in microseconds to presentation milliseconds at the
// pace of the scene: floor to ms, then scaled (negative stays negative).
func (b *builder) ms(us int64) int64 {
	ms := floorDiv(us, 1000)
	if ms > maxDelayMs {
		ms = maxDelayMs
	}
	return b.clock.scale(ms)
}

func lookTween(b *builder, c *ir.Call, args []value.Value) (value.Value, *value.Fault) {
	p, ok := props[args[0].Str]
	if !ok {
		return value.Value{}, badArgument(c, "look.Tween has no property %q", args[0].Str).
			WithLabel("use x, y, scale, opacity, rotation, fill, stroke or text")
	}
	s := int(args[1].Int)
	v := args[2]
	var to pv
	switch p {
	case trace.PX, trace.PY:
		switch v.K {
		case value.KNumber:
			to.N = v.Num
		case value.KShape:
			x, y := b.world.localPosOf(s, int(v.Int), b.now)
			to.N = x
			if p == trace.PY {
				to.N = y
			}
		default:
			return value.Value{}, badArgument(c, "Tween of %q needs a number or a shape", args[0].Str)
		}
	case trace.PFill, trace.PStroke:
		if v.K != value.KColor {
			return value.Value{}, badArgument(c, "Tween of %q needs a color", args[0].Str)
		}
		to.S = v.Str
	case trace.PText:
		if v.K != value.KText {
			return value.Value{}, badArgument(c, "Tween of %q needs a text", args[0].Str)
		}
		to.S = v.Str
	default:
		if v.K != value.KNumber {
			return value.Value{}, badArgument(c, "Tween of %q needs a number", args[0].Str)
		}
		to.N = v.Num
	}
	return value.Value{}, b.wf(b.world.tween(s, p, to, b.now, b.ms(args[3].Int)), c)
}

func lookEffect(b *builder, c *ir.Call, args []value.Value) (value.Value, *value.Fault) {
	switch args[0].Str {
	case "glow", "blur", "trail":
	default:
		return value.Value{}, badArgument(c, "look.Effect needs \"glow\", \"blur\" or \"trail\", not %q", args[0].Str)
	}
	return value.Value{}, b.wf(b.world.effect(int(args[1].Int), args[0].Str, b.now), c)
}

func lookLayout(b *builder, c *ir.Call, args []value.Value) (value.Value, *value.Fault) {
	return value.Value{}, b.wf(b.world.setLayout(args[0].Str), c)
}

func lookPlace(rel string) Func {
	return func(b *builder, c *ir.Call, args []value.Value) (value.Value, *value.Fault) {
		return value.Value{}, b.wf(b.world.place(int(args[0].Int), rel, int(args[1].Int), b.now), c)
	}
}

func lookOrbit(b *builder, c *ir.Call, args []value.Value) (value.Value, *value.Fault) {
	return value.Value{}, b.wf(b.world.orbit(int(args[0].Int), int(args[1].Int), args[2].Num, b.now), c)
}

func lookFlight(b *builder, c *ir.Call, args []value.Value) (value.Value, *value.Fault) {
	m := args[3]
	if m.Int <= 0 {
		return value.Value{}, badArgument(c, "look.Flight needs a message bound by a net event").
			WithLabel("this message was not bound by a net event")
	}
	return value.Value{}, b.wf(b.world.flightStart(int(args[0].Int), int(args[1].Int), int(args[2].Int), int(m.Int), b.now), c)
}

func lookDelete(b *builder, c *ir.Call, args []value.Value) (value.Value, *value.Fault) {
	b.world.delete(int(args[0].Int), b.now)
	return value.Value{}, nil
}

// simStateRows is sim.StateRows: one line per exported field of the packages
// n runs, "<Field> <value as text>", with "<package>.<Field>" when two of the
// packages export the same name. It reads every exported field of n.
func simStateRows(b *builder, c *ir.Call, args []value.Value) (value.Value, *value.Fault) {
	n := args[0].NodeID()
	if n == 0 {
		return value.Value{}, fault(source.Nobody, c.Pos(), "sim.StateRows of sim.Nobody").
			WithAlso(c.Args[0].Pos(), "this is sim.Nobody")
	}
	rows := b.store.rows(n)
	count := map[string]int{}
	for _, r := range rows {
		count[r.Field.Name]++
	}
	items := make([]value.Value, len(rows))
	for i, r := range rows {
		b.read(readKey{int32(n), b.store.slotOf(r.Field)})
		name := r.Field.Name
		if count[name] > 1 {
			name = b.qual(r.Field.Pkg) + "." + name
		}
		items[i] = value.Text(name + " " + value.Format(r.Value, value.FormatOpts{TopText: true}))
	}
	return value.ListOf(items), nil
}

func netID(b *builder, c *ir.Call, args []value.Value) (value.Value, *value.Fault) {
	if args[0].Int <= 0 {
		return value.Value{}, badArgument(c, "net.Id needs a message bound by a net event").
			WithLabel("this message was not bound by a net event")
	}
	return value.Number(float64(args[0].Int)), nil
}

func netType(b *builder, c *ir.Call, args []value.Value) (value.Value, *value.Fault) {
	return value.Text(args[0].Record().Type.Name), nil
}

// lookHost is the natives.Host of a look: the node queries read the replay
// store and, inside a reactive block, record what they read. Everything that
// acts on a simulation is not available in look context and panics.
type lookHost struct{ b *builder }

var _ natives.Host = (*lookHost)(nil)

func (h *lookHost) Nodes() []int {
	h.b.read(readKey{0, slotMembership})
	return h.b.store.nodes()
}

func (h *lookHost) NodesRunning(p *ir.Package) []int {
	h.b.read(readKey{0, slotMembership})
	return h.b.store.nodesRunning(p)
}

func (h *lookHost) IsUp(n int) bool {
	h.b.read(readKey{int32(n), slotUp})
	return h.b.store.isUp(n)
}

func (h *lookHost) Accepts(a, t types.Type) bool {
	return types.Accepts(a, t, func(p, q string) bool {
		pk := h.b.byPath[p]
		if pk == nil {
			return false
		}
		for _, r := range pk.Runs {
			if r.Path == q {
				return true
			}
		}
		return false
	})
}

func bug(what string) string { return "compiler bug: " + what + " in a look" }

func (h *lookHost) Now() int64      { panic(bug("Now")) }
func (h *lookHost) RNG() *rng.PCG32 { panic(bug("RNG")) }
func (h *lookHost) Seed(uint64)     { panic(bug("Seed")) }
func (h *lookHost) StopAt(int64)    { panic(bug("StopAt")) }
func (h *lookHost) Spawn(*ir.Package) int {
	panic(bug("Spawn"))
}
func (h *lookHost) Crash(int)                    { panic(bug("Crash")) }
func (h *lookHost) Restart(int)                  { panic(bug("Restart")) }
func (h *lookHost) Remove(int)                   { panic(bug("Remove")) }
func (h *lookHost) Send(*value.Record, int, int) { panic(bug("Send")) }
func (h *lookHost) SetLatency(int64, int64)      { panic(bug("SetLatency")) }
func (h *lookHost) SetLoss(float64)              { panic(bug("SetLoss")) }
func (h *lookHost) Partition([]int, []int)       { panic(bug("Partition")) }
func (h *lookHost) Heal()                        { panic(bug("Heal")) }
func (h *lookHost) DropNext(string, int, int)    { panic(bug("DropNext")) }
func (h *lookHost) DiskWrite(int, string, value.Value, types.Type) {
	panic(bug("DiskWrite"))
}
func (h *lookHost) DiskRead(int, string) (value.Value, types.Type, bool) {
	panic(bug("DiskRead"))
}
