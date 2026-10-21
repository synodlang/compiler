package natives

import (
	"strings"
	"unicode/utf8"

	"github.com/synodlang/compiler/internal/ir"
	"github.com/synodlang/compiler/internal/source"
	"github.com/synodlang/compiler/internal/value"
)

func init() {
	Table["text.from"] = func(h Host, c *ir.Call, args []value.Value) (value.Value, *value.Fault) {
		return value.Text(value.Format(args[0], value.FormatOpts{TopText: true})), nil
	}
	Table["text.join"] = textJoin
	Table["text.size"] = func(h Host, c *ir.Call, args []value.Value) (value.Value, *value.Fault) {
		return value.Number(float64(utf8.RuneCountInString(args[0].Str))), nil
	}
}

func textJoin(h Host, c *ir.Call, args []value.Value) (value.Value, *value.Fault) {
	var b strings.Builder
	for _, it := range args[0].List().Items {
		if b.Len()+len(it.Str) > MaxItems {
			return value.Value{}, fault(c, source.RunLimit, "text.Join would make a text of more than 1000000 characters")
		}
		b.WriteString(it.Str)
	}
	return value.Text(b.String()), nil
}
