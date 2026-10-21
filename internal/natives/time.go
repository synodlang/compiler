package natives

import (
	"github.com/synodlang/compiler/internal/ir"
	"github.com/synodlang/compiler/internal/value"
)

func init() {
	Table["time.now"] = func(h Host, c *ir.Call, args []value.Value) (value.Value, *value.Fault) {
		return value.Time(h.Now()), nil
	}
}
