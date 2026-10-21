package check

import (
	"github.com/synodlang/compiler/internal/resolve"
	"github.com/synodlang/compiler/internal/source"
	"github.com/synodlang/compiler/internal/types"
)

// walkStmts calls fn for every statement of every body of rp, nested ones
// included, with the file that holds it.
func walkStmts(rp *resolve.Package, fn func(f *resolve.File, s resolve.Stmt)) {
	var block func(f *resolve.File, b *resolve.Block)
	block = func(f *resolve.File, b *resolve.Block) {
		if b == nil {
			return
		}
		for _, s := range b.Stmts {
			fn(f, s)
			switch x := s.(type) {
			case *resolve.If:
				block(f, x.Then)
				block(f, x.Else)
			case *resolve.Each:
				block(f, x.Body)
			case *resolve.After:
				block(f, x.Body)
			case *resolve.Draw:
				block(f, x.Body)
			case *resolve.CallStmt:
				for _, cb := range x.Call.Blocks {
					block(f, cb)
				}
			}
		}
	}
	for _, ph := range rp.Phrases {
		block(ph.File, ph.Body)
	}
	for _, h := range rp.Handlers {
		block(h.File, h.Body)
	}
}

// AdjustHints corrects the hint of the resolver's unused-name diagnostics for
// the names of "each a, b in x": the resolver cannot tell a list from a map.
// Over a list the one-name form binds the item, so only an unused index can
// be dropped; over a map it binds the key, so only an unused value can.
// Where no one-name form helps, the hint is removed. The diagnostics are
// changed in place.
func AdjustHints(rp *resolve.Package, diags []source.Diagnostic) {
	if len(diags) == 0 {
		return
	}
	at := map[[3]any]int{}
	for i, d := range diags {
		if d.Code == source.UnusedName {
			at[[3]any{d.File, d.Line, d.Col}] = i
		}
	}
	if len(at) == 0 {
		return
	}
	walkStmts(rp, func(f *resolve.File, s resolve.Stmt) {
		e, ok := s.(*resolve.Each)
		if !ok || e.B == nil || e.A == nil {
			return
		}
		var dropFirst, dropSecond bool // the name whose absence leaves a valid one-name form
		switch e.Over.Type().(type) {
		case types.List:
			dropFirst = true // the index
		case types.Map:
			dropSecond = true // the value
		default:
			return
		}
		fix := func(l, other *resolve.Local, droppable bool) {
			i, ok := at[[3]any{f.Path, l.Pos.Line, l.Pos.Col}]
			if !ok {
				return
			}
			if droppable {
				diags[i].Hint = `write "each ` + other.Name + ` in …"`
			} else {
				diags[i].Hint = ""
			}
		}
		fix(e.A, e.B, dropFirst)
		fix(e.B, e.A, dropSecond)
	})
}
