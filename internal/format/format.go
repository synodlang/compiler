// Package format prints Synod source in canonical form (spec/10).
//
// Formatting has two separate steps. canonicalize rewrites a parsed tree into
// canonical content: token spellings, parentheses, negations and the order of
// imports. emit then lays the tree out as text: indentation, spacing, blank
// lines and comments. Splitting them lets the tests exercise layout without
// the rewrites.
package format

import (
	"github.com/synodlang/compiler/internal/parse"
	"github.com/synodlang/compiler/internal/source"
)

// Format returns the canonical form of f. When f does not parse, it returns
// the syntax or indent diagnostic and no output. Format is idempotent: the
// result formats to itself.
func Format(f *source.File) ([]byte, *source.Diagnostic) {
	tree, d := parse.Parse(f)
	if d != nil {
		return nil, d
	}
	canonicalize(tree)
	return emit(tree), nil
}
