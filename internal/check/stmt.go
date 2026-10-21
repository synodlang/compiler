package check

import (
	"github.com/synodlang/compiler/internal/resolve"
	"github.com/synodlang/compiler/internal/source"
	"github.com/synodlang/compiler/internal/types"
)

// pass walks every body once. In a non-final pass it only collects the
// sources of the places; the final pass also reports.
func (c *checker) pass(final bool) {
	c.final = final
	c.srcs = map[any][]src{}
	rp := c.rp
	recs := make([]*resolve.Record, 0, len(rp.Records)+1)
	if rp.Node != nil {
		recs = append(recs, rp.Node)
	}
	recs = append(recs, rp.Records...)
	for _, rec := range recs {
		for _, f := range rec.Fields {
			c.file = f.File
			name := f.Name
			if !rec.IsNode {
				name = rec.Name + "." + f.Name
			}
			c.note(f, name)
			if f.Init == nil {
				continue
			}
			var t types.Type
			c.quietly(f.Tainted || c.cycPlace[f], func() { t = c.expr(f.Init, c.ty[f]) })
			c.addSrc(f, name, f.Init.Where(), t, f.Init.Src())
			c.refine(f.Init, c.ty[f])
		}
	}
	for _, ph := range rp.Phrases {
		if ph.Body == nil {
			continue
		}
		c.file = ph.File
		c.phrase = nil
		if ph.Kind == resolve.PhraseNormal && ph.IsValue {
			c.phrase = ph
			c.note(ph, ph.Name)
		}
		c.block(ph.Body)
		c.phrase = nil
		if final && ph.IsValue && ph.Kind == resolve.PhraseNormal && !alwaysResults(ph.Body) && !hasBad(ph.Body) {
			c.errf(ph.Pos, source.MissingResult,
				`"`+ph.Describe("")+`" does not give a result on every path`,
				`end every path with "result …"`)
		}
	}
	for _, h := range rp.Handlers {
		c.file = h.File
		for _, m := range h.Matchers {
			if m.Kind != resolve.MatchEqual || m.Value == nil {
				continue
			}
			t := c.expr(m.Value, nil)
			if h.Event != nil && m.Hole < len(h.Event.Params) {
				ht := h.Event.Params[m.Hole].Type
				if !c.accepts(t, ht) && !c.accepts(ht, t) {
					c.errf(m.Value.Where(), source.TypeMismatch,
						`"`+m.Value.Src()+`" has type `+c.fmtType(t)+`, expected `+c.fmtType(ht), "")
				}
			}
		}
		if h.Body != nil {
			c.block(h.Body)
		}
	}
	for _, inv := range rp.Invariants {
		c.file = inv.File
		c.arg(inv.Cond, types.TBool)
	}
	if final {
		for _, p := range c.order {
			c.fold(p)
		}
	}
	c.phrase = nil
}

// alwaysResults reports that every path through b ends in "result".
func alwaysResults(b *resolve.Block) bool {
	if b == nil {
		return false
	}
	for _, s := range b.Stmts {
		switch x := s.(type) {
		case *resolve.Result:
			return true
		case *resolve.If:
			if x.Else != nil && alwaysResults(x.Then) && alwaysResults(x.Else) {
				return true
			}
		}
	}
	return false
}

// hasBad reports a statement that failed to resolve: the phrase may have
// had its result there.
func hasBad(b *resolve.Block) bool {
	if b == nil {
		return false
	}
	for _, s := range b.Stmts {
		switch x := s.(type) {
		case *resolve.BadStmt:
			return true
		case *resolve.If:
			if hasBad(x.Then) || hasBad(x.Else) {
				return true
			}
		case *resolve.Each:
			if hasBad(x.Body) {
				return true
			}
		}
	}
	return false
}

func (c *checker) block(b *resolve.Block) {
	if b == nil {
		return
	}
	for _, s := range b.Stmts {
		c.stmt(s)
	}
}

func (c *checker) stmt(s resolve.Stmt) {
	switch x := s.(type) {
	case *resolve.Set:
		c.set(x)
	case *resolve.CallStmt:
		if t := c.callExpr(x.Call, nil); t != nil && !isInvalid(t) {
			c.valueAsStatement(x.Where(), x.Call.Src())
		}
	case *resolve.If:
		c.arg(x.Cond, types.TBool)
		c.block(x.Then)
		c.block(x.Else)
	case *resolve.Each:
		c.each(x)
	case *resolve.After:
		c.arg(x.Delay, types.TTime)
		c.block(x.Body)
	case *resolve.Deliver:
		c.arg(x.Rec, types.TMsg)
		c.arg(x.To, types.TNode)
	case *resolve.Emit:
		c.arg(x.Rec, types.TMsg)
		c.arg(x.By, types.TNode)
	case *resolve.Result:
		var want types.Type
		if c.phrase != nil {
			want = c.ty[c.phrase]
		}
		t := c.expr(x.Value, want)
		if c.phrase != nil {
			c.addSrc(c.phrase, c.phrase.Name, x.Value.Where(), t, x.Value.Src())
		}
	case *resolve.Draw:
		c.draw(x)
	case *resolve.ExprStmt:
		if _, bad := x.X.(*resolve.Bad); !bad {
			if t := c.expr(x.X, nil); !isInvalid(t) {
				c.valueAsStatement(x.Where(), x.X.Src())
			}
		}
	}
}

func (c *checker) valueAsStatement(pos source.Pos, text string) {
	c.errf(pos, source.ValueAsStatement, `"`+text+`" gives a value but stands as a statement`,
		`use it, as in "set x to (…)"`)
}

func (c *checker) each(x *resolve.Each) {
	ot := c.expr(x.Over, nil)
	var first, second types.Type = types.TInvalid, types.TInvalid
	switch t := ot.(type) {
	case types.List:
		if x.B != nil {
			first, second = types.TNumber, t.Elem
		} else {
			first = t.Elem
		}
	case types.Map:
		first, second = t.Key, t.Val
	default:
		if !isInvalid(ot) && !isEmpty(ot) {
			c.errf(x.Over.Where(), source.TypeMismatch,
				`"`+x.Over.Src()+`" has type `+c.fmtType(ot)+`, expected a list or a map`, "")
		}
	}
	if x.A != nil {
		c.addSrc(x.A, x.A.Name, x.A.Pos, first, "")
	}
	if x.B != nil {
		c.addSrc(x.B, x.B.Name, x.B.Pos, second, "")
	}
	c.block(x.Body)
}

func (c *checker) draw(x *resolve.Draw) {
	c.arg(x.Shape, types.TShape)
	c.addSrc(x.Name, x.Name.Name, x.Name.Pos, types.TShape, "")
	if x.For != nil {
		kt := c.expr(x.For, nil)
		if !c.accepts(kt, types.TNode) && !c.accepts(kt, types.TText) && !c.accepts(kt, types.TNumber) {
			c.errf(x.For.Where(), source.TypeMismatch,
				`"`+x.For.Src()+`" has type `+c.fmtType(kt)+`, expected node, text or number`, "")
		}
	}
	c.block(x.Body)
}

// ---- set ----

// quietly runs f; with tainted set, "{}" ambiguities are not reported, as
// the uses that would settle them are in sentences that failed to resolve.
func (c *checker) quietly(tainted bool, f func()) {
	if tainted {
		c.quiet++
		defer func() { c.quiet-- }()
	}
	f()
}

func (c *checker) set(s *resolve.Set) {
	switch pl := s.Place.(type) {
	case *resolve.LocalPlace:
		l := pl.Local
		c.note(l, l.Name)
		var t types.Type
		c.quietly(l.Tainted || c.cycPlace[l], func() { t = c.expr(s.Value, c.ty[l]) })
		c.addSrc(l, l.Name, s.Value.Where(), t, s.Value.Src())
	case *resolve.SelectPlace:
		f := c.nodeFieldPlace(pl.Select)
		if f == nil {
			c.expr(s.Value, nil)
			return
		}
		ft := c.fieldType(f)
		var t types.Type
		c.quietly(f.Tainted || c.cycPlace[f], func() { t = c.expr(s.Value, ft) })
		c.flow(f, f.Name, s.Value, t)
	case *resolve.ItemPlace:
		c.itemPlace(s, pl)
	default:
		c.expr(s.Value, nil)
	}
}

// nodeFieldPlace settles the select of a place and returns the node field it
// ends at, or nil after reporting bad-place.
func (c *checker) nodeFieldPlace(sel *resolve.Select) *resolve.Field {
	c.selType(sel)
	si := c.info.Sel[sel]
	if si == nil || si.Bad || len(si.Steps) == 0 {
		return nil
	}
	last := si.Steps[len(si.Steps)-1]
	switch last.Kind {
	case StepNodeField:
		// Only the last step may be a node field: a record in between would
		// be immutable, and the checker has settled the rest.
		return last.Field
	case StepRecordField:
		c.errf(sel.Where(), source.BadPlace, `cannot set "`+sel.Src()+`": it is a record field`, "")
	case StepRecvPhrase:
		c.errf(sel.Where(), source.BadPlace, `cannot set "`+sel.Src()+`": it is a phrase`, "")
	default:
		c.errf(sel.Where(), source.BadPlace, `"`+sel.Src()+`" is not a node field`, "")
	}
	return nil
}

// flow sends the value x of type t into the field f: a source of its type
// when f belongs to this package, a check against its type otherwise.
func (c *checker) flow(f *resolve.Field, name string, x resolve.Expr, t types.Type) {
	if f.Owner.Pkg == c.rp {
		c.addSrc(f, name, x.Where(), t, x.Src())
		return
	}
	c.need(x, t, c.fieldType(f))
}

func validKey(t types.Type) bool {
	switch x := t.(type) {
	case types.Basic:
		return x.K == types.Text || x.K == types.Number || x.K == types.Bool || x.K == types.AnyNode || x.K == types.Invalid
	case types.NodeOf:
		return true
	}
	return false
}

func (c *checker) itemPlace(s *resolve.Set, pl *resolve.ItemPlace) {
	var key any
	var name string
	var pt types.Type
	var own, tainted bool
	switch b := pl.Base.(type) {
	case *resolve.LocalPlace:
		key, name, own, tainted = b.Local, b.Local.Name, true, b.Local.Tainted || c.cycPlace[b.Local]
		c.note(b.Local, name)
		pt = c.ty[b.Local]
	case *resolve.SelectPlace:
		f := c.nodeFieldPlace(b.Select)
		if f == nil {
			c.expr(pl.Key, nil)
			c.expr(s.Value, nil)
			return
		}
		key, name, own, tainted = f, f.Name, f.Owner.Pkg == c.rp, f.Tainted || c.cycPlace[f]
		pt = c.fieldType(f)
		if own {
			pt = c.ty[f]
		}
	default:
		c.expr(pl.Key, nil)
		c.expr(s.Value, nil)
		return
	}
	kt := c.expr(pl.Key, nil)
	var want types.Type
	switch t := pt.(type) {
	case types.List:
		want = t.Elem
	case types.Map:
		want = t.Val
	}
	var vt types.Type
	c.quietly(tainted, func() { vt = c.expr(s.Value, want) })
	text := "item " + pl.Key.Src() + " of " + name
	if own {
		switch {
		case isInvalid(kt):
		case isT(kt, types.Number):
			c.addWeak(key, name, s.Value.Where(), kt, vt, text)
		default:
			if !validKey(kt) {
				c.errf(pl.Key.Where(), source.TypeMismatch,
					`"`+pl.Key.Src()+`" has type `+c.fmtType(kt)+`, but a map key is text, number, bool or a node`, "")
				return
			}
			c.addItem(key, name, s.Value.Where(), types.Map{Key: kt, Val: vt}, text)
		}
		return
	}
	switch t := pt.(type) {
	case types.List:
		c.need(pl.Key, kt, types.TNumber)
		c.need(s.Value, vt, t.Elem)
	case types.Map:
		c.need(pl.Key, kt, t.Key)
		c.need(s.Value, vt, t.Val)
	default:
		if !isInvalid(pt) {
			c.errf(pl.Where(), source.TypeMismatch, `"`+name+`" has type `+c.fmtType(pt)+`, expected a list or a map`, "")
		}
	}
}
