package natives

import (
	"github.com/synodlang/compiler/internal/ir"
	"github.com/synodlang/compiler/internal/source"
	"github.com/synodlang/compiler/internal/value"
)

func init() {
	Table["lesson.choices"] = lessonChoices
}

// lessonChoices: choices [c], the check behind Ask [q] choosing [c]. It
// returns c unchanged when c holds 2 to 6 different, non-empty texts.
func lessonChoices(h Host, c *ir.Call, args []value.Value) (value.Value, *value.Fault) {
	items := args[0].List().Items
	ok := len(items) >= 2 && len(items) <= 6
	for i := 0; ok && i < len(items); i++ {
		if items[i].Str == "" {
			ok = false
		}
		for j := 0; ok && j < i; j++ {
			if value.Equal(items[i], items[j]) {
				ok = false
			}
		}
	}
	if !ok {
		return value.Value{}, withArg(fault(c, source.BadArgument, "lesson.Ask … choosing needs 2 to 6 different, non-empty choices"), c, args, 0, "is "+show(args[0]))
	}
	return args[0], nil
}
