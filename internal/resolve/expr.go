package resolve

import (
	"math"
	"strconv"
	"strings"

	"github.com/synodlang/compiler/internal/format"
	"github.com/synodlang/compiler/internal/parse"
	"github.com/synodlang/compiler/internal/source"
	"github.com/synodlang/compiler/internal/types"
)

func (b *body) eb(span source.Span, text string) ExprBase {
	return ExprBase{At: atS(span), Text: text}
}

func (b *body) bad(span source.Span, text string) *Bad {
	return &Bad{ExprBase: b.eb(span, text)}
}

// unexportedDef is the secondary label at the definition of a name that is
// not visible outside its package (none for std).
func unexportedDef(pk *Package, file *File, span source.Span, q string) []source.Extra {
	if pk == nil || pk.IsStd || file == nil {
		return nil
	}
	return []source.Extra{file.also(span, "lower-case: stays inside "+q)}
}

// res is the result of resolving a sentence: a statement or an expression.
type res struct {
	S Stmt
	E Expr
}

func isFieldType(t types.Type) bool { return types.Equal(t, types.TField) }

// constErr reports a non-constant in an initial value.
func (b *body) constErr(span source.Span) {
	b.r.err(b.f, span, source.Context, "initial values are constants",
		"use a literal, {}, a list, a message or sim.Nobody")
}

// arg resolves an item in argument position. hole is the hole it fills, or
// nil.
func (b *body) arg(it parse.Item, hole *Param) Expr {
	switch v := it.(type) {
	case *parse.Word:
		return b.wordExpr(v, hole)
	case *parse.Member:
		return b.memberExpr(v, hole)
	case *parse.Number:
		n, _ := strconv.ParseFloat(v.Text, 64)
		return &Lit{ExprBase: b.eb(sp(v), v.Text), Kind: LitNumber, Num: n}
	case *parse.Duration:
		return &Lit{ExprBase: b.eb(sp(v), v.Text), Kind: LitTime, Micros: micros(v.Text)}
	case *parse.Text:
		return &Lit{ExprBase: b.eb(sp(v), v.Raw), Kind: LitText, Str: unquote(v.Raw)}
	case *parse.Color:
		b.r.err(b.f, sp(v), source.ColorLiteral, "a color literal stands only as the result of a phrase with no holes",
			"name the color, as theme.Accent does")
		return &ColorLit{ExprBase: b.eb(sp(v), v.Text), Hex: strings.ToUpper(v.Text)}
	case *parse.Paren:
		return b.paren(v, hole)
	case *parse.List:
		return b.list(v)
	}
	return b.bad(sp(it), printItem(it))
}

func micros(text string) int64 {
	num, mult := strings.TrimSuffix(text, "ms"), 1000.0
	if num == text {
		num, mult = strings.TrimSuffix(text, "s"), 1e6
	}
	f, _ := strconv.ParseFloat(num, 64)
	return int64(math.Round(f * mult))
}

func (b *body) list(l *parse.List) Expr {
	text := printItem(l)
	if len(l.Items) == 0 {
		return &EmptyLit{ExprBase: b.eb(sp(l), text)}
	}
	out := &ListLit{ExprBase: b.eb(sp(l), text)}
	for _, e := range l.Items {
		out.Items = append(out.Items, b.arg(e, nil))
	}
	return out
}

func (b *body) paren(p *parse.Paren, hole *Param) Expr {
	s := p.S
	if len(s.Groups) == 1 && len(s.Groups[0]) == 1 {
		return b.arg(s.Groups[0][0], hole)
	}
	r := b.sentence(s, nil)
	switch {
	case r.E != nil:
		return r.E
	case r.S != nil:
		return &ActionExpr{ExprBase: b.eb(sp(p), printSentence(s)), Stmt: r.S}
	}
	return b.bad(sp(p), printItem(p))
}

// wordExpr resolves a bare word in argument position.
func (b *body) wordExpr(w *parse.Word, hole *Param) Expr {
	text := w.Name
	switch w.Name {
	case "true", "false":
		return &Lit{ExprBase: b.eb(sp(w), text), Kind: LitBool, Bool: w.Name == "true"}
	}
	if hole != nil && isFieldType(hole.Type) {
		if fl := b.r.pkg.fields[w.Name]; fl != nil {
			b.r.mark(b.f, w.P, RoleField, &Def{File: fl.File.Path, Pos: fl.Pos})
			return &FieldRef{ExprBase: b.eb(sp(w), text), Field: fl}
		}
	}
	if sym, _ := b.lookup(w.Name); sym != nil {
		useSym(sym, w.P)
		b.markSym(w.P, sym)
		return &Ref{ExprBase: b.eb(sp(w), text), Sym: sym}
	}
	if ph := b.zeroPhrase(w.Name); ph != nil {
		return b.zeroCall(ph, sp(w), text, nil)
	}
	if imp, own := b.qualifier(w.Name); imp != nil || own {
		if pk := b.qualPkg(imp, own); pk != nil {
			if imp != nil {
				imp.refs = append(imp.refs, w.P)
			}
			b.r.mark(b.f, w.P, RoleName, nil)
			return &PkgRef{ExprBase: b.eb(sp(w), text), Pkg: pk}
		}
		return b.bad(sp(w), text)
	}
	b.unknownName(w, text, w.Name)
	return b.bad(sp(w), text)
}

func (b *body) markSym(pos source.Pos, s Symbol) {
	b.r.mark(b.f, pos, RoleName, &Def{File: b.f.Path, Pos: symPos(s)})
}

// zeroCall is a call of a zero-hole phrase.
func (b *body) zeroCall(ph *Phrase, span source.Span, text string, imp *Import) Expr {
	return b.zeroCallAt(ph, span, span.Start, text, imp)
}

// zeroCallAt is zeroCall with the position of the head word, which differs
// from the start of span when a package qualifier stands before it.
func (b *body) zeroCallAt(ph *Phrase, span source.Span, head source.Pos, text string, imp *Import) Expr {
	if b.kind == bkConst && ph.Native != "sim.nobody" {
		b.constErr(span)
	}
	b.r.mark(b.f, head, RolePhrase, &Def{File: ph.File.Path, Pos: ph.Pos})
	return &Call{ExprBase: b.eb(span, text), Phrase: ph, Qual: imp}
}

// memberExpr resolves a member in argument position.
func (b *body) memberExpr(m *parse.Member, hole *Param) Expr {
	parts := m.Parts
	text := strings.Join(parts, ".")
	// A binding under a qualified spelling: api.Get, api.Get.Key.
	if len(parts) >= 2 {
		if sym, _ := b.lookup(parts[0] + "." + parts[1]); sym != nil {
			if bd, ok := sym.(*Binding); ok {
				useSym(bd, m.P)
				b.markSym(partPos(m, 1), bd)
				base := &Ref{ExprBase: b.eb(spanTo(parse.PartSpan(m, 0), parse.PartSpan(m, 1)), parts[0]+"."+parts[1]), Sym: bd}
				if len(parts) == 2 {
					return base
				}
				return b.selectOf(m, base, 2)
			}
		}
	}
	if sym, _ := b.lookup(parts[0]); sym != nil {
		useSym(sym, m.P)
		b.markSym(m.P, sym)
		base := &Ref{ExprBase: b.eb(parse.PartSpan(m, 0), parts[0]), Sym: sym}
		return b.selectOf(m, base, 1)
	}
	if ph := b.zeroPhrase(parts[0]); ph != nil {
		base := b.zeroCall(ph, parse.PartSpan(m, 0), parts[0], nil)
		return b.selectOf(m, base, 1)
	}
	if imp, own := b.qualifier(parts[0]); imp != nil || own {
		if imp != nil {
			imp.refs = append(imp.refs, m.P)
		}
		pk := b.qualPkg(imp, own)
		if pk == nil {
			return b.bad(sp(m), text)
		}
		if len(parts) != 2 {
			b.r.err(b.f, source.Span{Start: partPos(m, 2), End: m.End()}, source.UnknownPhrase, `no phrase matches "`+text+`"`, "")
			return b.bad(sp(m), text)
		}
		if own {
			b.r.addFix(b.f, format.Fix{Pos: m.P, DropPrefix: true, Part: 0})
		}
		pos1 := partPos(m, 1)
		if hole != nil && isFieldType(hole.Type) {
			if fl := pk.fields[parts[1]]; fl != nil {
				if !fl.Exported && !own {
					b.r.err(b.f, parse.PartSpan(m, 1), source.Unexported, `"`+text+`" is not exported by `+parts[0],
						"only names starting upper-case are visible outside "+parts[0],
						unexportedDef(pk, fl.File, fl.Sp, parts[0])...)
				}
				b.r.mark(b.f, pos1, RoleField, &Def{File: fl.File.Path, Pos: fl.Pos})
				return &FieldRef{ExprBase: b.eb(sp(m), text), Field: fl}
			}
		}
		for _, ph := range pk.heads[parts[1]] {
			if usable(ph) && ph.Receiver == nil && ph.Zero {
				if !ph.Exported && !own {
					b.r.err(b.f, parse.PartSpan(m, 1), source.Unexported, `"`+text+`" is not exported by `+parts[0],
						"only names starting upper-case are visible outside "+parts[0],
						unexportedDef(pk, ph.File, ph.Sp, parts[0])...)
				}
				if imp != nil || own {
					b.r.mark(b.f, m.P, RoleName, nil)
				}
				return b.zeroCallAt(ph, sp(m), pos1, text, imp)
			}
		}
		h := hint{}
		if t, c := b.closestMember(imp, own, parts[1]); c != "" {
			h.text = t
			h.edits = []source.Edit{source.EditAt(parse.PartSpan(m, 1), c)}
		}
		b.r.err(b.f, sp(m), source.UnknownPhrase, `no phrase matches "`+text+`"`, h.text, h.extras()...)
		return b.bad(sp(m), text)
	}
	b.unknownName(m, text, parts[0])
	return b.bad(sp(m), text)
}

// selectOf builds the Select of parts[from:] on base. A package qualifier
// right after the base is marked on the part; the own package's qualifier is
// dropped, with a fix.
func (b *body) selectOf(m *parse.Member, base Expr, from int) Expr {
	text := strings.Join(m.Parts, ".")
	sel := &Select{ExprBase: b.eb(sp(m), text), Base: base}
	for i := from; i < len(m.Parts); i++ {
		pt := Part{Name: m.Parts[i], Pos: partPos(m, i), Sp: parse.PartSpan(m, i)}
		if i == from && len(m.Parts) > from+1 {
			imp, own := b.qualifier(m.Parts[i])
			switch {
			case own:
				b.r.addFix(b.f, format.Fix{Pos: m.P, DropPrefix: true, Part: i})
				continue
			case imp != nil:
				imp.typeRefs = true // a qualifier past the first part cannot be renamed by a fix
				if imp.Pkg == nil {
					return b.bad(sp(m), text)
				}
				pt.Pkg = imp.Pkg
			}
		}
		sel.Path = append(sel.Path, pt)
	}
	return sel
}

// ---- messages ----

func (b *body) recordOf(it parse.Item) (*Record, bool) {
	switch v := it.(type) {
	case *parse.Word:
		if rec := b.r.pkg.records[v.Name]; rec != nil {
			b.r.mark(b.f, v.P, RoleType, &Def{File: rec.File.Path, Pos: rec.Pos})
			return rec, true
		}
		h := hint{}
		if c := closest(v.Name, sortedKeys(b.r.pkg.records)); c != "" {
			h.text = `did you mean "` + c + `"?`
			h.edits = []source.Edit{source.EditAt(sp(v), c)}
		}
		b.r.err(b.f, sp(v), source.UnknownPhrase, `no record type "`+v.Name+`"`, h.text, h.extras()...)
	case *parse.Member:
		if len(v.Parts) == 2 {
			imp, own := b.qualifier(v.Parts[0])
			if pk := b.qualPkg(imp, own); pk != nil {
				if imp != nil {
					imp.refs = append(imp.refs, v.P)
				}
				if rec := pk.records[v.Parts[1]]; rec != nil {
					if own {
						b.r.addFix(b.f, format.Fix{Pos: v.P, DropPrefix: true, Part: 0})
					} else if !rec.Exported {
						b.r.err(b.f, parse.PartSpan(v, 1), source.Unexported, `"`+printItem(v)+`" is not exported by `+v.Parts[0],
							"only names starting upper-case are visible outside "+v.Parts[0],
							unexportedDef(pk, rec.File, rec.Sp, v.Parts[0])...)
					}
					b.r.mark(b.f, partPos(v, 1), RoleType, &Def{File: rec.File.Path, Pos: rec.Pos})
					return rec, true
				}
				b.r.err(b.f, sp(v), source.UnknownPhrase, `no record type "`+printItem(v)+`"`, "")
				return nil, false
			}
			if imp != nil {
				return nil, false
			}
			if b.r.isStd(v.Parts[0]) {
				b.r.err(b.f, parse.PartSpan(v, 0), source.UnknownPhrase, `"`+v.Parts[0]+`" is not imported`, `add "use std/`+v.Parts[0]+`"`,
					b.f.notImported(v.Parts[0]))
				return nil, false
			}
		}
		b.r.err(b.f, sp(v), source.UnknownPhrase, `no record type "`+printItem(v)+`"`, "")
	default:
		b.r.err(b.f, sp(it), source.UnknownPhrase, `no record type "`+printItem(it)+`"`, "")
	}
	return nil, false
}

// messageExpr resolves "message T with a x, b y".
func (b *body) messageExpr(s *parse.Sentence) Expr {
	g := s.Groups[0]
	text := printSentence(s)
	ssp := parse.SentenceSpan(s)
	pos := ssp.Start
	if len(g) < 3 || !parse.IsWord(g[2], "with") || len(g) == 3 {
		b.r.err(b.f, ssp, source.UnknownPhrase, `no phrase matches "`+text+`"`, "write \"message <Type> with <field> <value>\"")
		b.touch(g)
		return b.bad(ssp, text)
	}
	rec, ok := b.recordOf(g[1])
	// settings: group 0 holds the first after "with".
	var groups [][]parse.Item
	groups = append(groups, g[3:])
	groups = append(groups, s.Groups[1:]...)
	lit := &RecordLit{ExprBase: b.eb(ssp, text), Record: rec}
	valid := ok
	seen := map[string]source.Span{}
	var names []string
	for _, grp := range groups {
		if len(grp) != 2 {
			var hint string
			var more []source.Extra
			if len(grp) > 2 {
				hint = `did you mean "` + printItems(grp[:1]) + " (" + printItems(grp[1:]) + `)"?`
				more = append(more, source.HelpEdit("parenthesize the value", parenEdits(parse.SpanOfItems(grp[1:]))...))
			}
			b.r.err(b.f, parse.SpanOfItems(grp), source.UnknownPhrase, `no phrase matches "`+printItems(grp)+`"`, hint, more...)
			b.touch(grp)
			valid = false
			continue
		}
		w, isWord := grp[0].(*parse.Word)
		val := b.arg(grp[1], nil)
		if !isWord {
			b.r.err(b.f, parse.SpanOfItems(grp), source.UnknownPhrase, `no phrase matches "`+printItems(grp)+`"`, "")
			valid = false
			continue
		}
		if rec == nil {
			continue
		}
		setSp := spanTo(sp(w), sp(grp[1]))
		fl := rec.Lookup(w.Name)
		if fl == nil {
			h := hint{}
			var fn []string
			for _, f := range rec.Fields {
				fn = append(fn, f.Name)
			}
			if c := closest(w.Name, fn); c != "" {
				h.text = `did you mean "` + c + `"?`
				h.edits = []source.Edit{source.EditAt(sp(w), c)}
			}
			h.labels = append(h.labels, label{rec.File, rec.Sp, `record "` + rec.Name + `" is defined here`})
			b.r.err(b.f, sp(w), source.UnknownPhrase, `record "`+rec.Name+`" has no field "`+w.Name+`"`, h.text, h.extras()...)
			valid = false
			continue
		}
		b.r.mark(b.f, w.P, RoleField, &Def{File: fl.File.Path, Pos: fl.Pos})
		if first, dup := seen[w.Name]; dup {
			b.r.err(b.f, setSp, source.DuplicateName, `field "`+w.Name+`" is set twice`, "",
				b.f.also(first, "first set here"))
			valid = false
			continue
		}
		seen[w.Name] = setSp
		names = append(names, w.Name)
		lit.Settings = append(lit.Settings, Setting{Field: fl, Value: val, Pos: w.P, Sp: setSp})
	}
	if valid && rec != nil && len(names) > 1 {
		order := func() []string {
			var out []string
			for _, f := range rec.Fields {
				if _, ok := seen[f.Name]; ok {
					out = append(out, f.Name)
				}
			}
			return out
		}()
		for i := range order {
			if order[i] != names[i] {
				b.r.addFix(b.f, format.Fix{Pos: pos, Order: order})
				break
			}
		}
	}
	if rec == nil {
		return b.bad(ssp, text)
	}
	return lit
}

// ---- sentences ----

// sentence resolves a sentence: a statement line when ln is not nil, an
// argument in parentheses otherwise.
func (b *body) sentence(s *parse.Sentence, ln *parse.Line) res {
	items := s.Groups[0]
	if w, ok := items[0].(*parse.Word); ok && w.Name == "message" {
		return res{E: b.messageExpr(s)}
	}
	return b.generic(s, items, ln)
}

func (b *body) generic(s *parse.Sentence, items []parse.Item, ln *parse.Line) res {
	cands := b.candidates(items)
	var m []candidate
	for _, c := range cands {
		if c.f.shape(items, ln) {
			m = append(m, c)
		}
	}
	switch len(m) {
	case 0:
		if len(items) == 1 {
			// A name, a member or a literal standing alone.
			if _, isParen := items[0].(*parse.Paren); !isParen {
				return res{E: b.arg(items[0], nil)}
			}
		}
		b.noMatch(s, items, ln)
		return res{}
	case 1:
		return b.bind(m[0], s, items, ln)
	}
	text, more := b.ambiguousHint(m)
	b.r.err(b.f, parse.SentenceSpan(s), source.AmbiguousPhrase, `"`+printSentence(s)+`" matches more than one phrase`, text, more...)
	b.touch(items)
	return res{}
}

// ambiguousHint returns the short-form hint that names the first two
// candidates, and the extras that describe all of them: a label on each user
// phrase, a note on each other form, and the help.
func (b *body) ambiguousHint(m []candidate) (string, []source.Extra) {
	desc := func(c candidate) string {
		if c.f.kind != fkPhrase {
			return `the language form "` + c.f.text + `"`
		}
		ph := c.f.ph
		qual := ""
		if c.qual != nil {
			qual = c.qual.Qual
		}
		return `"` + ph.Describe(qual) + `" (` + at(ph.File, ph.Pos) + `)`
	}
	var more []source.Extra
	for _, c := range m {
		if c.f.kind != fkPhrase {
			more = append(more, source.Note(`also matches the language form "`+c.f.text+`"`))
			continue
		}
		ph := c.f.ph
		if ph.Pkg.IsStd {
			qual := ""
			if c.qual != nil {
				qual = c.qual.Qual
			}
			more = append(more, source.Note(`also matches "`+ph.Describe(qual)+`" from `+ph.Pkg.Path))
			continue
		}
		more = append(more, ph.File.also(ph.Sp, "matches this phrase"))
	}
	more = append(more, source.HelpText("parenthesize to pick one"))
	return desc(m[0]) + " and " + desc(m[1]), more
}

// bind binds the one candidate that matched.
func (b *body) bind(c candidate, s *parse.Sentence, items []parse.Item, ln *parse.Line) res {
	f := c.f
	text := printSentence(s)
	ssp := parse.SpanOfItems(items)
	switch f.kind {
	case fkPhrase:
		return res{E: b.bindPhrase(c, text, items, ln)}
	case fkSet:
		return b.bindSet(text, items, nil)
	case fkSetItem:
		return b.bindSet(text, items, items)
	case fkIf:
		st := &If{At: atS(ssp), Cond: b.arg(items[1], nil)}
		st.Then = b.block(ln.Block, "", source.Span{})
		if len(ln.Conts) > 0 {
			st.Else = b.block(ln.Conts[0].Block, "", source.Span{})
		}
		return res{S: st}
	case fkAfter:
		st := &After{At: atS(ssp), Delay: b.arg(items[1], nil)}
		st.Body = b.block(ln.Block, "after", parse.LineSpan(ln))
		return res{S: st}
	case fkDeliver:
		return res{S: &Deliver{At: atS(ssp), Rec: b.arg(items[1], nil), To: b.arg(items[3], nil)}}
	case fkEmit:
		return res{S: &Emit{At: atS(ssp), Rec: b.arg(items[1], nil), By: b.arg(items[3], nil)}}
	case fkResult:
		return b.bindResult(ssp, items)
	case fkDraw:
		return b.bindDraw(ssp, items, ln)
	case fkItem:
		return res{E: &Item{ExprBase: b.eb(ssp, text), Key: b.arg(items[1], nil), Coll: b.arg(items[3], nil)}}
	case fkNot:
		return res{E: &Not{ExprBase: b.eb(ssp, text), X: b.arg(items[1], nil)}}
	case fkBinary:
		opSp := source.Span{Start: items[1].Pos(), End: items[len(f.items)-2].End()}
		return res{E: &Binary{ExprBase: b.eb(ssp, text), Op: f.op, A: b.arg(items[0], nil),
			B: b.arg(items[len(items)-1], nil), OpPos: items[1].Pos(), OpSpan: opSp}}
	case fkAttach:
		w := items[0].(*parse.Word)
		if Exported(w.Name) {
			b.r.err(b.f, sp(w), source.NonCanonical,
				`"`+w.Name+`" must start lower-case: only exported names start upper-case`, "rename it",
				source.HelpEdit("rename it", lowerEdit(w.P, w.Name)),
				source.Note(`an attachment name is global: synod fmt does not rename it`))
		}
		b.r.mark(b.f, w.P, RoleName, nil)
		return res{E: &Attachment{ExprBase: b.eb(ssp, text), Name: w.Name, Key: b.arg(items[2], nil)}}
	}
	return res{}
}

func (b *body) bindResult(ssp source.Span, items []parse.Item) res {
	switch {
	case b.kind != bkPhrase:
		b.r.err(b.f, ssp, source.Context, `"result" stands only in a phrase definition`, "")
	case b.noResult > 0:
		in := b.bounds[len(b.bounds)-1]
		b.r.err(b.f, ssp, source.Context, `"result" cannot stand in an after block or a block argument`, "",
			b.f.also(in.sp, "inside this "+strings.TrimPrefix(in.kind, "a ")))
	}
	var v Expr
	if c, ok := items[1].(*parse.Color); ok && b.ph != nil && b.ph.Zero && b.ph.Receiver == nil && b.kind == bkPhrase {
		v = &ColorLit{ExprBase: b.eb(sp(c), c.Text), Hex: strings.ToUpper(c.Text)}
	} else {
		v = b.arg(items[1], nil)
	}
	return res{S: &Result{At: atS(ssp), Value: v}}
}

func (b *body) bindDraw(ssp source.Span, items []parse.Item, ln *parse.Line) res {
	st := &Draw{At: atS(ssp), Shape: b.arg(items[1], nil)}
	if len(items) == 6 {
		st.For = b.arg(items[5], nil)
	}
	w, ok := items[3].(*parse.Word)
	if !ok {
		b.r.err(b.f, sp(items[3]), source.UnknownPhrase, `no phrase matches "`+printItems(items)+`"`, "a draw name is a word")
		return res{S: &BadStmt{atS(ssp)}}
	}
	if old, ok := b.sc.syms[w.Name].(*Local); ok && old.Kind == LocalDraw {
		st.Name, st.Rebind = old, true
		old.refs = append(old.refs, w.P)
	} else {
		st.Name = b.declLocal(w.Name, sp(w), LocalDraw)
	}
	if len(items) == 6 {
		st.Name.Attached = true
		if Exported(w.Name) {
			// The attachment name is global; no local fix can rename it.
			b.r.err(b.f, sp(w), source.NonCanonical,
				`"`+w.Name+`" must start lower-case: only exported names start upper-case`, "rename it",
				source.HelpEdit("rename it", lowerEdit(w.P, w.Name)),
				source.Note(`an attachment name is global: synod fmt does not rename it`))
		}
	}
	b.r.mark(b.f, w.P, RoleName, &Def{File: b.f.Path, Pos: st.Name.Pos})
	if ln != nil && ln.Block != nil {
		st.Body = b.block(ln.Block, "", source.Span{})
	}
	return res{S: st}
}

// bindPhrase binds the arguments of a phrase call.
func (b *body) bindPhrase(c candidate, text string, items []parse.Item, ln *parse.Line) Expr {
	ph := c.f.ph
	ssp := parse.SpanOfItems(items)
	call := &Call{ExprBase: b.eb(ssp, text), Phrase: ph, Qual: c.qual}
	if b.kind == bkConst {
		b.constErr(ssp)
	}
	if m, ok := items[0].(*parse.Member); ok {
		if c.qual != nil {
			if c.qualPart == 0 {
				c.qual.refs = append(c.qual.refs, m.P)
			} else {
				c.qual.typeRefs = true
			}
		}
		hp := len(m.Parts) - 1
		if c.recv != "" {
			sym, _ := b.lookup(c.recv)
			if sym != nil {
				useSym(sym, m.P)
				b.markSym(m.P, sym)
				call.Receiver = &Ref{ExprBase: b.eb(parse.PartSpan(m, 0), c.recv), Sym: sym}
			} else if zp := b.zeroPhrase(c.recv); zp != nil {
				call.Receiver = b.zeroCall(zp, parse.PartSpan(m, 0), c.recv, nil)
			}
		}
		if c.own {
			b.r.addFix(b.f, format.Fix{Pos: m.P, DropPrefix: true, Part: c.qualPart})
		}
		if !ph.Exported && ph.Pkg != b.r.pkg {
			q := m.Parts[c.qualPart]
			b.r.err(b.f, parse.PartSpan(m, hp), source.Unexported, `"`+strings.Join(m.Parts, ".")+`" is not exported by `+q,
				"only names starting upper-case are visible outside "+q,
				unexportedDef(ph.Pkg, ph.File, ph.Sp, q)...)
		}
		b.r.mark(b.f, partPos(m, hp), RolePhrase, &Def{File: ph.File.Path, Pos: ph.Pos})
	} else {
		b.r.mark(b.f, items[0].Pos(), RolePhrase, &Def{File: ph.File.Path, Pos: ph.Pos})
	}
	for i, it := range ph.Items {
		if i == 0 {
			continue
		}
		if it.Param != nil {
			call.Args = append(call.Args, b.arg(items[i], it.Param))
		} else if w, ok := items[i].(*parse.Word); ok {
			b.r.mark(b.f, w.P, RolePhrase, nil)
		}
	}
	if ln != nil && ln.Block != nil {
		call.Blocks = append(call.Blocks, b.block(ln.Block, "a block argument", parse.LineSpan(ln)))
		for _, ct := range ln.Conts {
			call.Blocks = append(call.Blocks, b.block(ct.Block, "a block argument", sp(ct.Word)))
		}
	}
	return call
}

// touch marks the names and qualifiers in items as used, after an error, so
// that one error does not breed unused-name and unused-import errors.
func (b *body) touch(items []parse.Item) {
	for _, it := range items {
		switch v := it.(type) {
		case *parse.Word:
			if s, _ := b.lookup(v.Name); s != nil {
				useSym(s, v.P)
				taintSym(s)
			}
			if imp := b.f.byQual[v.Name]; imp != nil {
				imp.Used = true
			}
			b.taintField(v.Name)
		case *parse.Member:
			if s, _ := b.lookup(v.Parts[0]); s != nil {
				useSym(s, v.P)
				taintSym(s)
			}
			for i, p := range v.Parts {
				if imp := b.f.byQual[p]; imp != nil {
					imp.Used = true
				}
				if i > 0 {
					b.taintField(p)
				}
			}
		case *parse.Paren:
			for _, g := range v.S.Groups {
				b.touch(g)
			}
		case *parse.List:
			b.touch(v.Items)
		}
	}
}

// taintSym marks a local named by a failed sentence.
func taintSym(s Symbol) {
	if l, ok := s.(*Local); ok {
		l.Tainted = true
	}
}

// taintField marks the node and record fields of this package called name.
func (b *body) taintField(name string) {
	if fl := b.r.pkg.fields[name]; fl != nil {
		fl.Tainted = true
	}
	for _, rec := range b.r.pkg.Records {
		if fl := rec.Lookup(name); fl != nil {
			fl.Tainted = true
		}
	}
}
