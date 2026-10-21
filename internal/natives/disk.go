package natives

import (
	"github.com/synodlang/compiler/internal/ir"
	"github.com/synodlang/compiler/internal/source"
	"github.com/synodlang/compiler/internal/types"
	"github.com/synodlang/compiler/internal/value"
)

func init() {
	Table["disk.write"] = diskWrite
	Table["disk.read"] = diskRead
}

// diskWrite: Write [v] at [k] on [n]. The value is stored with the static
// type of the argument expression.
func diskWrite(h Host, c *ir.Call, args []value.Value) (value.Value, *value.Fault) {
	if f := nobody(c, args[2], "disk.Write on"); f != nil {
		return value.Value{}, f
	}
	h.DiskWrite(args[2].NodeID(), args[1].Str, args[0], c.Args[0].Type())
	return value.Value{}, nil
}

// diskRead: Read [k] on [n] or [d].
func diskRead(h Host, c *ir.Call, args []value.Value) (value.Value, *value.Fault) {
	if f := nobody(c, args[1], "disk.Read on"); f != nil {
		return value.Value{}, f
	}
	v, t, ok := h.DiskRead(args[1].NodeID(), args[0].Str)
	if !ok {
		return args[2], nil
	}
	want := c.Args[2].Type()
	if !h.Accepts(t, want) {
		return value.Value{}, fault(c, source.DiskType,
			"disk value at "+value.Format(args[0], value.FormatOpts{})+" is a "+
				types.Format(t, "", nil)+", not a "+types.Format(want, "", nil))
	}
	return v, nil
}
