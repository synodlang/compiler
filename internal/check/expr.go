package check

import (
	"math/big"
	"strconv"
	"strings"

	"github.com/synodlang/compiler/internal/resolve"
	"github.com/synodlang/compiler/internal/source"
	"github.com/synodlang/compiler/internal/stdlib"
	"github.com/synodlang/compiler/internal/types"
)

// expr types e, giving "{}" the type expected where it stands, and records
// the type in the tree. It never returns nil: an expression that has no
// value, or failed, has type Invalid, which is accepted everywhere.
func (c *checker) expr(e resolve.Expr, want types.Type) types.Type {
	if el, ok := e.(*resolve.EmptyLit); ok {
		return c.emptyLit(el, want)
	}
	t := c.expr1(e, want)
	if t == nil {
		t = types.TInvalid
	}
	e.SetType(t)
	return t
}

// arg types e as the argument of a hole of type want, and requires the
// type to be accepted.
func (c *checker) arg(e resolve.Expr, want types.Type) types.Type {
	t := c.expr(e, want)
	c.need(e, t, want)
	c.refine(e, want)
	return t
}

// need reports a type that is not accepted where want is expected.
func (c *checker) need(e resolve.Expr, got, want types.Type, more ...source.Extra) {
	if want == nil || c.accepts(got, want) {
		return
	}
	c.errf(e.Span(), source.TypeMismatch,
		`"`+e.Src()+`" has type `+c.fmtType(got)+`, expected `+c.fmtType(want), "",
		append([]source.Extra{c.hasType(got)}, more...)...)
}

// expects is the label at the hole of a phrase written in Synod that the
// argument does not fit.
func (c *checker) expects(ph *resolve.Phrase, prm *resolve.Param, want types.Type) source.Extra {
	if ph == nil || prm == nil || ph.Pkg == nil || ph.Pkg.IsStd {
		return func(*source.Diagnostic) {}
	}
	return alsoIn(ph.File, prm.HoleSp, "expects "+c.fmtType(want))
}

// refine records how a place is used: passing a read of a place to a hole
// of a list or map type says what shape the place has.
func (c *checker) refine(e resolve.Expr, want types.Type) {
	if want == nil || !isCollection(want) {
		return
	}
	switch x := e.(type) {
	case *resolve.Ref:
		if l, ok := x.Sym.(*resolve.Local); ok {
			c.addHi(l, want)
		}
	case *resolve.Select:
		if f := c.ownField(x); f != nil {
			c.addHi(f, want)
		}
	}
}

// ownField is the field of this package that a settled Select ends at, or
// nil.
func (c *checker) ownField(s *resolve.Select) *resolve.Field {
	si := c.info.Sel[s]
	if si == nil || si.Bad || len(si.Steps) == 0 {
		return nil
	}
	last := si.Steps[len(si.Steps)-1]
	if (last.Kind == StepNodeField || last.Kind == StepRecordField) && last.Field.Owner.Pkg == c.rp {
		return last.Field
	}
	return nil
}

func (c *checker) emptyLit(e *resolve.EmptyLit, want types.Type) types.Type {
	var t types.Type = types.Empty{}
	if !c.final {
		return t
	}
	switch {
	case want != nil && isCollection(want) && !hasEmpty(want) && !hasValue(want):
		e.SetType(want)
	case want != nil && isInvalid(want):
		e.SetType(types.TInvalid)
	case want != nil && !isCollection(want) && !isEmpty(want) && !hasValue(want):
		// A type mismatch is reported where the "{}" flows.
		e.SetType(types.TInvalid)
	case c.quiet > 0:
		// A failed sentence hides the uses that would settle this type.
		e.SetType(types.TInvalid)
	default:
		msg := "cannot tell the type of {}"
		if want != nil && isCollection(want) {
			msg = "cannot tell the item type of {}"
		}
		c.errf(e.Span(), source.AmbiguousEmpty, msg,
			"use it with std/lists or std/maps, or assign it where the type is known")
		e.SetType(types.TInvalid)
	}
	return t
}

func isEmpty(t types.Type) bool { _, ok := t.(types.Empty); return ok }

func (c *checker) expr1(e resolve.Expr, want types.Type) types.Type {
	switch x := e.(type) {
	case *resolve.Lit:
		return c.lit(x)
	case *resolve.ColorLit:
		return types.TColor
	case *resolve.Ref:
		return c.ref(x)
	case *resolve.PkgRef:
		return types.TPackage
	case *resolve.FieldRef:
		return types.TField
	case *resolve.ListLit:
		return c.listLit(x, want)
	case *resolve.Select:
		return c.selType(x)
	case *resolve.Call:
		t := c.callExpr(x, want)
		if t == nil {
			c.errf(x.Span(), source.ActionAsValue, `"`+x.Src()+`" gives no value`, "", c.definedWithout(x.Phrase))
			return types.TInvalid
		}
		return t
	case *resolve.RecordLit:
		return c.recordLit(x)
	case *resolve.Item:
		return c.item(x)
	case *resolve.Binary:
		return c.binary(x)
	case *resolve.Not:
		c.arg(x.X, types.TBool)
		return types.TBool
	case *resolve.Attachment:
		kt := c.expr(x.Key, nil)
		if !c.accepts(kt, types.TNode) && !c.accepts(kt, types.TText) && !c.accepts(kt, types.TNumber) {
			c.errf(x.Key.Span(), source.TypeMismatch,
				`"`+x.Key.Src()+`" has type `+c.fmtType(kt)+`, expected node, text or number`, "", c.hasType(kt))
		}
		return types.TShape
	case *resolve.ActionExpr:
		c.errf(x.Span(), source.ActionAsValue, `"`+x.Src()+`" gives no value`, "")
		c.stmt(x.Stmt)
		return types.TInvalid
	}
	return types.TInvalid
}

func (c *checker) lit(x *resolve.Lit) types.Type {
	switch x.Kind {
	case resolve.LitText:
		return types.TText
	case resolve.LitBool:
		return types.TBool
	case resolve.LitNumber:
		if c.final {
			if _, err := strconv.ParseFloat(x.Text, 64); err != nil {
				if ne, ok := err.(*strconv.NumError); ok && ne.Err == strconv.ErrRange {
					c.errf(x.Span(), source.Syntax, "number out of 64-bit float range", "")
				}
			}
		}
		return types.TNumber
	case resolve.LitTime:
		if c.final {
			c.checkDuration(x)
		}
		return types.TTime
	}
	return types.TInvalid
}

// checkDuration reports a duration that is not a whole number of
// microseconds (spec/04 section 4.1).
func (c *checker) checkDuration(x *resolve.Lit) {
	num, mult := strings.TrimSuffix(x.Text, "ms"), int64(1000)
	if num == x.Text {
		num, mult = strings.TrimSuffix(x.Text, "s"), 1000000
	}
	r, ok := new(big.Rat).SetString(num)
	if !ok {
		return
	}
	r.Mul(r, new(big.Rat).SetInt64(mult))
	switch {
	case !r.IsInt():
		c.errf(x.Span(), source.Syntax, `duration "`+x.Text+`" is not a whole number of microseconds`, "")
	case r.Num().BitLen() > 62:
		c.errf(x.Span(), source.Syntax, `duration "`+x.Text+`" is out of range`, "")
	}
}

func (c *checker) ref(x *resolve.Ref) types.Type {
	switch s := x.Sym.(type) {
	case *resolve.Local:
		c.note(s, s.Name)
		return c.placeType(s)
	case *resolve.Param:
		if s.Kind == resolve.ParamBlock {
			c.errf(x.Span(), source.ActionAsValue, `"`+x.Src()+`" gives no value`, "",
				alsoIn(c.file, s.HoleSp, "a block hole, it runs a block"))
			return types.TInvalid
		}
		return s.Type
	case *resolve.Binding:
		if s.OfField != nil {
			return c.fieldType(s.OfField)
		}
		return s.Type
	}
	return types.TInvalid
}

func (c *checker) listLit(x *resolve.ListLit, want types.Type) types.Type {
	var ew types.Type
	if l, ok := want.(types.List); ok {
		ew = l.Elem
	}
	var acc, firstT types.Type
	for _, it := range x.Items {
		t := c.expr(it, ew)
		if acc == nil {
			acc, firstT = t, t
			continue
		}
		j, ok := c.join(acc, t)
		if !ok {
			c.errf(it.Span(), source.TypeMismatch,
				`"`+it.Src()+`" has type `+c.fmtType(t)+`, but the list has item type `+c.fmtType(acc), "",
				c.hasType(t), c.alsoAt(x.Items[0].Span(), "the first item has type "+c.fmtType(firstT)))
			continue
		}
		acc = j
	}
	if acc == nil {
		acc = types.TInvalid
	}
	return types.List{Elem: acc}
}

func (c *checker) recordLit(x *resolve.RecordLit) types.Type {
	for _, st := range x.Settings {
		ft := c.fieldType(st.Field)
		var want types.Type
		if !isInvalid(ft) {
			want = ft
		}
		t := c.expr(st.Value, want)
		if st.Field.Owner.Pkg == c.rp {
			c.addSrc(st.Field, st.Field.Owner.Name+"."+st.Field.Name, st.Value.Span(), t, st.Value.Src())
			c.refine(st.Value, want)
		} else {
			c.need(st.Value, t, want)
		}
	}
	return x.Record.Type()
}

func (c *checker) item(x *resolve.Item) types.Type {
	ct := c.expr(x.Coll, nil)
	kt := c.expr(x.Key, nil)
	switch t := ct.(type) {
	case types.List:
		c.need(x.Key, kt, types.TNumber)
		return t.Elem
	case types.Map:
		c.need(x.Key, kt, t.Key)
		return t.Val
	case types.Empty:
		return types.TInvalid
	}
	if !isInvalid(ct) {
		c.errf(x.Coll.Span(), source.TypeMismatch,
			`"item" takes a list or a map, not `+c.fmtType(ct), "", c.hasType(ct))
	}
	return types.TInvalid
}

// ---- operators (spec/04 section 4.5) ----

func isT(t types.Type, k types.Kind) bool {
	b, ok := t.(types.Basic)
	return ok && b.K == k
}

func (c *checker) binary(x *resolve.Binary) types.Type {
	var wb types.Type
	var a, b types.Type
	switch x.Op {
	case "and", "or":
		a = c.expr(x.A, types.TBool)
		b = c.expr(x.B, types.TBool)
	case "is", "is not":
		if _, ok := x.A.(*resolve.EmptyLit); ok {
			b = c.expr(x.B, nil)
			a = c.expr(x.A, b)
			break
		}
		a = c.expr(x.A, nil)
		wb = a
		if isEmpty(a) {
			wb = nil
		}
		b = c.expr(x.B, wb)
	default:
		a = c.expr(x.A, nil)
		b = c.expr(x.B, nil)
	}
	bad := isInvalid(a) || isInvalid(b)
	mismatch := func(forms string) types.Type {
		if !bad {
			c.errf(opSpan(x), source.TypeMismatch,
				`"`+x.Op+`" takes `+forms+`, not `+c.fmtType(a)+` and `+c.fmtType(b), "",
				c.alsoAt(x.A.Span(), "has type "+c.fmtType(a)), c.alsoAt(x.B.Span(), "has type "+c.fmtType(b)))
		}
		return types.TInvalid
	}
	switch x.Op {
	case "plus", "minus":
		switch {
		case isT(a, types.Number) && isT(b, types.Number):
			return types.TNumber
		case isT(a, types.Time) && isT(b, types.Time):
			return types.TTime
		case bad:
			if !isInvalid(a) {
				return a
			}
			return b
		}
		return mismatch("number and number, or time and time")
	case "times":
		switch {
		case isT(a, types.Number) && isT(b, types.Number):
			return types.TNumber
		case isT(a, types.Time) && isT(b, types.Number), isT(a, types.Number) && isT(b, types.Time):
			return types.TTime
		case bad:
			return types.TInvalid
		}
		return mismatch("number and number, time and number, or number and time")
	case "divided by":
		switch {
		case isT(a, types.Number) && isT(b, types.Number), isT(a, types.Time) && isT(b, types.Time):
			return types.TNumber
		case isT(a, types.Time) && isT(b, types.Number):
			return types.TTime
		case bad:
			return types.TInvalid
		}
		mismatch("number and number, time and number, or time and time")
		return types.TInvalid
	case "less than", "more than", "at least", "at most":
		switch {
		case isT(a, types.Number) && isT(b, types.Number), isT(a, types.Time) && isT(b, types.Time), bad:
			return types.TBool
		}
		mismatch("number and number, or time and time")
		return types.TBool
	case "is", "is not":
		if !bad && !c.accepts(a, b) && !c.accepts(b, a) {
			c.errf(x.Span(), source.TypeMismatch, "cannot compare "+c.fmtType(a)+" with "+c.fmtType(b), "",
				c.alsoAt(x.A.Span(), "has type "+c.fmtType(a)), c.alsoAt(x.B.Span(), "has type "+c.fmtType(b)))
		}
		return types.TBool
	case "is in", "is not in":
		switch coll := b.(type) {
		case types.List:
			if !bad && !c.accepts(a, coll.Elem) && !c.accepts(coll.Elem, a) {
				c.errf(x.Span(), source.TypeMismatch, "cannot compare "+c.fmtType(a)+" with the items of "+c.fmtType(b), "",
					c.alsoAt(x.A.Span(), "has type "+c.fmtType(a)), c.alsoAt(x.B.Span(), "has type "+c.fmtType(b)))
			}
		case types.Map:
			if !bad && !c.accepts(a, coll.Key) && !c.accepts(coll.Key, a) {
				c.errf(x.Span(), source.TypeMismatch, "cannot compare "+c.fmtType(a)+" with the keys of "+c.fmtType(b), "",
					c.alsoAt(x.A.Span(), "has type "+c.fmtType(a)), c.alsoAt(x.B.Span(), "has type "+c.fmtType(b)))
			}
		default:
			if !bad && !isEmpty(b) {
				mismatch("a value and a list or a map")
			}
		}
		return types.TBool
	case "and", "or":
		c.need(x.A, a, types.TBool)
		c.need(x.B, b, types.TBool)
		return types.TBool
	}
	return types.TInvalid
}

// ---- member reads ----

func (c *checker) recordOf(t types.Record) *resolve.Record {
	if p := c.pkgs[t.Pkg]; p != nil {
		return p.Record(t.Name)
	}
	return nil
}

func (c *checker) unexported(s *resolve.Select, sp source.Span, pkg *resolve.Package, defFile *resolve.File, def source.Span) {
	c.errf(sp, source.Unexported, `"`+s.Src()+`" is not exported by `+pkg.Name,
		"only names starting upper-case are visible outside "+pkg.Name,
		alsoIn(defFile, def, "lower-case: stays inside "+pkg.Name))
}

// definedWithout is the label at the definition of an action that stands
// where a value is needed.
func (c *checker) definedWithout(ph *resolve.Phrase) source.Extra {
	if ph == nil || ph.Pkg != c.rp || ph.Sig != nil || ph.IsValue {
		return func(*source.Diagnostic) {}
	}
	return alsoIn(ph.File, ph.Sp, "defined without a result")
}

// opSpan is the span of the operator words of x.
func opSpan(x *resolve.Binary) source.Span {
	if x.OpSpan.Start != x.OpSpan.End {
		return x.OpSpan
	}
	return source.At(x.OpPos)
}

// didYouMean is the help of a misspelled name: the hint, with the edit that
// writes the candidate over sp when it is a single word.
func didYouMean(sp source.Span, cn string) []source.Extra {
	if cn == "" {
		return nil
	}
	text := `did you mean "` + cn + `"?`
	if strings.ContainsAny(cn, " \t") || sp.Start == sp.End {
		return []source.Extra{source.HelpText(text)}
	}
	return []source.Extra{source.HelpEdit(text, source.EditAt(sp, cn))}
}

// selType settles a member read from the type of its base, part by part.
func (c *checker) selType(e *resolve.Select) types.Type {
	cur := c.expr(e.Base, nil)
	si := &SelInfo{}
	c.info.Sel[e] = si
	fail := func() types.Type { si.Bad = true; return types.TInvalid }
	for i := 0; i < len(e.Path); i++ {
		part := e.Path[i]
		if isInvalid(cur) {
			return fail()
		}
		switch t := cur.(type) {
		case types.NodeOf:
			target := c.rp
			if part.Pkg != nil {
				target = part.Pkg
				if !c.runs(t.Pkg, target.Path) {
					c.errf(e.Base.Span(), source.TypeMismatch,
						`"`+e.Base.Src()+`" is a `+c.fmtType(t)+`, which does not run `+target.Name, "", c.hasType(t))
					return fail()
				}
				si.Steps = append(si.Steps, Step{Kind: StepPkg, Pkg: target, Pos: part.Pos, Sp: part.Sp})
				i++
				if i >= len(e.Path) {
					return fail()
				}
				part = e.Path[i]
			} else {
				if c.rp.IsMain {
					c.errf(part.Sp, source.UnknownPhrase,
						`"`+e.Base.Src()+`" has no field "`+part.Name+`"`,
						`name the package: "`+e.Base.Src()+`.<package>.`+part.Name+`"`)
					return fail()
				}
				if !c.runs(t.Pkg, c.rp.Path) {
					c.errf(e.Base.Span(), source.TypeMismatch,
						`"`+e.Base.Src()+`" is a `+c.fmtType(t)+`, which does not run `+c.rp.Name, "", c.hasType(t))
					return fail()
				}
			}
			if fl := target.NodeField(part.Name); fl != nil {
				if !fl.Exported && target != c.rp {
					c.unexported(e, part.Sp, target, fl.File, fl.Sp)
					return fail()
				}
				cur = c.fieldType(fl)
				si.Steps = append(si.Steps, Step{Kind: StepNodeField, Field: fl, Type: cur, Pos: part.Pos, Sp: part.Sp})
				c.mark(part.Pos, resolve.RoleField, fl.File, fl.Pos)
				continue
			}
			if ph := target.ReceiverPhrase(part.Name); ph != nil && ph.Zero {
				if !ph.Exported && target != c.rp {
					c.unexported(e, part.Sp, target, ph.File, ph.Sp)
					return fail()
				}
				rt := c.resultType(ph)
				if ph.Sig != nil {
					rt = nil
				}
				if rt == nil {
					c.errf(e.Span(), source.ActionAsValue, `"`+e.Src()+`" gives no value`, "", c.definedWithout(ph))
					return fail()
				}
				cur = rt
				si.Steps = append(si.Steps, Step{Kind: StepRecvPhrase, Phrase: ph, Type: cur, Pos: part.Pos, Sp: part.Sp})
				c.mark(part.Pos, resolve.RolePhrase, ph.File, ph.Pos)
				continue
			}
			hint := ""
			var more []source.Extra
			var names []string
			for _, f := range target.Node.Fields {
				if f.Exported || target == c.rp {
					names = append(names, f.Name)
				}
			}
			for _, ph := range target.Phrases {
				if ph.Receiver != nil && ph.Zero && ph.Kind == resolve.PhraseNormal {
					names = append(names, ph.Head)
				}
			}
			if cn := closeName(part.Name, names); cn != "" {
				hint = `did you mean "` + cn + `"?`
				more = didYouMean(part.Sp, cn)
			}
			more = append(more, alsoIn(target.Node.File, target.Node.Sp, "the fields are defined here"))
			c.errf(part.Sp, source.UnknownPhrase, `"`+c.fmtType(t)+`" has no field "`+part.Name+`"`, hint, more...)
			return fail()
		case types.Record:
			rec := c.recordOf(t)
			if rec == nil {
				return fail()
			}
			fl := rec.Lookup(part.Name)
			if fl == nil {
				hint := ""
				var more []source.Extra
				var names []string
				for _, f := range rec.Fields {
					names = append(names, f.Name)
				}
				if cn := closeName(part.Name, names); cn != "" {
					hint = `did you mean "` + cn + `"?`
					more = didYouMean(part.Sp, cn)
				}
				more = append(more, alsoIn(rec.File, rec.Sp, "the fields are defined here"))
				c.errf(part.Sp, source.UnknownPhrase, `record "`+rec.Name+`" has no field "`+part.Name+`"`, hint, more...)
				return fail()
			}
			if !fl.Exported && rec.Pkg != c.rp {
				c.unexported(e, part.Sp, rec.Pkg, fl.File, fl.Sp)
				return fail()
			}
			cur = c.fieldType(fl)
			si.Steps = append(si.Steps, Step{Kind: StepRecordField, Field: fl, Type: cur, Pos: part.Pos, Sp: part.Sp})
			c.mark(part.Pos, resolve.RoleField, fl.File, fl.Pos)
		case types.Basic:
			switch t.K {
			case types.AnyNode:
				c.errf(e.Base.Span(), source.TypeMismatch, "a node has no fields; use a Node or p.Node value", "", c.hasType(t))
			case types.Msg:
				c.errf(e.Base.Span(), source.TypeMismatch, "a msg has no readable fields; use a record type", "", c.hasType(t))
			default:
				c.errf(part.Sp, source.UnknownPhrase, `"`+e.Base.Src()+`" has no field "`+part.Name+`"`, "")
			}
			return fail()
		default:
			if !isEmpty(cur) {
				c.errf(part.Sp, source.UnknownPhrase, `"`+e.Base.Src()+`" has no field "`+part.Name+`"`, "")
			}
			return fail()
		}
	}
	return cur
}

// ---- calls ----

func (c *checker) hasRel(sig *stdlib.Sig, k stdlib.RelKind, a int) bool {
	if sig == nil {
		return false
	}
	for _, r := range sig.Rels {
		if r.Kind == k && r.A == a {
			return true
		}
	}
	return false
}

// relIndex finds the relation that gives hole i its type from another hole,
// or -1.
func (c *checker) relIndex(sig *stdlib.Sig, i int) int {
	if sig == nil {
		return -1
	}
	for k, r := range sig.Rels {
		switch r.Kind {
		case stdlib.RelElemAccepts:
			if r.B == i {
				return k
			}
		case stdlib.RelSameElem:
			if r.A == i || r.B == i {
				return k
			}
		}
	}
	return -1
}

// sameAs returns the hole whose type decides the type of "{}" in hole i, or
// -1: the item for the list of lists.Append, the other list for lists.Extend.
func (c *checker) sameAs(sig *stdlib.Sig, i int) int {
	k := c.relIndex(sig, i)
	if k < 0 {
		return -1
	}
	r := sig.Rels[k]
	switch r.Kind {
	case stdlib.RelElemAccepts:
		return r.A
	case stdlib.RelSameElem:
		if r.A == i {
			return r.B
		}
		return r.A
	}
	return -1
}

func isUnknownColl(t types.Type) bool {
	switch x := t.(type) {
	case types.Empty:
		return true
	case types.List:
		return isEmpty(x.Elem)
	}
	return false
}

// callExpr types a phrase call and returns the type of the value it gives,
// nil for an action.
func (c *checker) callExpr(e *resolve.Call, want types.Type) types.Type {
	ph := e.Phrase
	if ph == nil {
		return types.TInvalid
	}
	sig := ph.Sig
	if ph.Receiver != nil && e.Receiver != nil {
		rt := c.expr(e.Receiver, nil)
		c.need(e.Receiver, rt, ph.Receiver.Type, c.expects(ph, ph.Receiver, ph.Receiver.Type))
	}
	argT := make([]types.Type, len(e.Args))
	typeArg := func(i int, ew types.Type) {
		a := e.Args[i]
		prm := ph.Params[i]
		if ew == nil {
			ew = prm.Type
			if c.hasRel(sig, stdlib.RelExpectResultOn, i) && want != nil && !isInvalid(want) {
				ew = want
			}
		}
		t := c.expr(a, ew)
		argT[i] = t
		c.need(a, t, prm.Type, c.expects(ph, prm, prm.Type))
		c.refine(a, prm.Type)
	}
	// A "{}" given where the items of another argument decide its type
	// ("lists.Append x to {}") is typed after that argument.
	var later []int
	for i, a := range e.Args {
		if i >= len(ph.Params) {
			c.expr(a, nil)
			continue
		}
		if _, isEmpty := a.(*resolve.EmptyLit); isEmpty && sig != nil && c.sameAs(sig, i) >= 0 {
			later = append(later, i)
			continue
		}
		typeArg(i, nil)
	}
	for _, i := range later {
		var ew types.Type
		if j := c.sameAs(sig, i); j >= 0 && argT[j] != nil {
			switch t := argT[j].(type) {
			case types.List:
				ew = t
			default:
				if sig.Rels[c.relIndex(sig, i)].Kind == stdlib.RelElemAccepts && !isInvalid(t) {
					ew = types.List{Elem: t}
				}
			}
		}
		typeArg(i, ew)
	}
	for _, b := range e.Blocks {
		c.block(b)
	}
	if sig != nil {
		return c.nativeResult(e, sig, argT)
	}
	return c.resultType(ph)
}

func (c *checker) nativeResult(e *resolve.Call, sig *stdlib.Sig, argT []types.Type) types.Type {
	arg := func(i int) types.Type {
		if i >= 0 && i < len(argT) && argT[i] != nil {
			return argT[i]
		}
		return types.TInvalid
	}
	expr := func(i int) resolve.Expr {
		if i >= 0 && i < len(e.Args) {
			return e.Args[i]
		}
		return e
	}
	for _, r := range sig.Rels {
		switch r.Kind {
		case stdlib.RelElemAccepts:
			if l, ok := arg(r.B).(types.List); ok && !isEmpty(l.Elem) {
				at := arg(r.A)
				if !c.accepts(at, l.Elem) && !(sig.ID == "lists.without" && c.accepts(l.Elem, at)) {
					c.errf(expr(r.A).Span(), source.TypeMismatch,
						`"`+expr(r.A).Src()+`" has type `+c.fmtType(at)+`, expected `+c.fmtType(l.Elem), "", c.hasType(at))
				}
			}
		case stdlib.RelKeyAccepts:
			if m, ok := arg(r.B).(types.Map); ok && !isEmpty(m.Key) {
				at := arg(r.A)
				if !c.accepts(at, m.Key) && !c.accepts(m.Key, at) {
					c.errf(expr(r.A).Span(), source.TypeMismatch,
						`"`+expr(r.A).Src()+`" has type `+c.fmtType(at)+`, expected `+c.fmtType(m.Key), "", c.hasType(at))
				}
			}
		case stdlib.RelSameElem:
			la, oka := arg(r.A).(types.List)
			lb, okb := arg(r.B).(types.List)
			if oka && okb && !isEmpty(la.Elem) && !c.accepts(lb.Elem, la.Elem) {
				c.errf(expr(r.B).Span(), source.TypeMismatch,
					`"`+expr(r.B).Src()+`" has type `+c.fmtType(arg(r.B))+`, expected `+c.fmtType(arg(r.A)), "", c.hasType(arg(r.B)))
			}
		case stdlib.RelSortable:
			if l, ok := arg(r.A).(types.List); ok {
				el := l.Elem
				if !isInvalid(el) && !isEmpty(el) && !isT(el, types.Number) && !isT(el, types.Time) && !isT(el, types.Text) {
					c.errf(expr(r.A).Span(), source.TypeMismatch,
						`"`+expr(r.A).Src()+`" has type `+c.fmtType(arg(r.A))+`, expected a list of numbers, times or text`, "", c.hasType(arg(r.A)))
				}
			}
		}
	}
	rule := sig.Result
	switch rule.Kind {
	case stdlib.RuleNone:
		return nil
	case stdlib.RuleFixed:
		return rule.T
	case stdlib.RuleSameAs:
		t := arg(rule.Arg)
		for _, r := range sig.Rels {
			switch r.Kind {
			case stdlib.RelElemAccepts:
				if r.B == rule.Arg && sig.ID != "lists.without" && isUnknownColl(t) && !isInvalid(arg(r.A)) {
					t = types.List{Elem: arg(r.A)}
				}
			case stdlib.RelSameElem:
				if r.A == rule.Arg && isUnknownColl(t) {
					t = arg(r.B)
				}
			}
		}
		return t
	case stdlib.RuleElemOf:
		if l, ok := arg(rule.Arg).(types.List); ok {
			return l.Elem
		}
		return types.TInvalid
	case stdlib.RuleKeysOf:
		if m, ok := arg(rule.Arg).(types.Map); ok {
			return types.List{Elem: m.Key}
		}
		return types.TInvalid
	case stdlib.RuleNodeOfPkg, stdlib.RuleListOfNodeOfPkg:
		var n types.Type = types.TNode
		if i := rule.Arg; i < len(e.Args) {
			if pr, ok := e.Args[i].(*resolve.PkgRef); ok && pr.Pkg != nil && !pr.Pkg.IsMain {
				n = types.NodeOf{Pkg: pr.Pkg.Path}
			}
		}
		if rule.Kind == stdlib.RuleListOfNodeOfPkg {
			return types.List{Elem: n}
		}
		return n
	}
	return types.TInvalid
}
