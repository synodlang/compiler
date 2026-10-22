package doc

import (
	"strconv"
	"strings"

	"github.com/synodlang/compiler/internal/stdlib"
	"github.com/synodlang/compiler/internal/types"
)

// Action is the Gives text of an action phrase.
const Action = "—"

// GivesText is the Gives text of a native with result rule r, the one
// spelling per rule of std/README.md (decision 91). holes are the names of
// the inline holes in pattern order.
func GivesText(r stdlib.Rule, holes []string) string {
	hole := func() string {
		if r.Arg >= 0 && r.Arg < len(holes) {
			return holes[r.Arg]
		}
		return "?" + strconv.Itoa(r.Arg)
	}
	switch r.Kind {
	case stdlib.RuleNone:
		return Action
	case stdlib.RuleFixed:
		return types.Format(r.T, "", nil)
	case stdlib.RuleSameAs:
		return "same as `" + hole() + "`"
	case stdlib.RuleElemOf:
		return "item type of `" + hole() + "`"
	case stdlib.RuleKeysOf:
		return "list of the key type of `" + hole() + "`"
	case stdlib.RuleNodeOfPkg:
		return hole() + ".Node"
	case stdlib.RuleListOfNodeOfPkg:
		return "list of " + hole() + ".Node"
	}
	return "?"
}

// markdownGives writes a Gives text as a catalog cell: a type with several
// words or a dot is in code font, as std/README.md does.
func markdownGives(g string) string {
	if strings.Contains(g, "`") || !strings.ContainsAny(g, " .") {
		return g
	}
	return "`" + g + "`"
}
