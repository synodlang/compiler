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

func (b *body) eb(pos source.Pos, text string) ExprBase {
	return ExprBase{At: At{pos}, Text: text}
}

func (b *body) bad(pos source.Pos, text string) *Bad {
	return &Bad{ExprBase: b.eb(pos, text)}
}

// res is the result of resolving a sentence: a statement or an expression.
type res struct {
	S Stmt
	E Expr
}

func isFieldType(t types.Type) bool { return types.Equal(t, types.TField) }

// constErr reports a non-constant in an initial value.
func (b *body) constErr(pos source.Pos) {
	b.r.err(b.f, pos, source.Context, "initial values are constants",
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
		return &Lit{ExprBase: b.eb(v.P, v.Text), Kind: LitNumber, Num: n}
	case *parse.Duration:
		return &Lit{ExprBase: b.eb(v.P, v.Text), Kind: LitTime, Micros: micros(v.Text)}
	case *parse.Text:
		return &Lit{ExprBase: b.eb(v.P, v.Raw), Kind: LitText, Str: unquote(v.Raw)}
	case *parse.Color:
		b.r.err(b.f, v.P, source.ColorLiteral, "a color literal stands only as the result of a phrase with no holes",
			"name the color, as theme.Accent does")
		return &ColorLit{ExprBase: b.eb(v.P, v.Text), Hex: strings.ToUpper(v.Text)}
	case *parse.Paren:
		return b.paren(v, hole)
	case *parse.List:
		return b.list(v)
	}
	return b.bad(it.Pos(), printItem(it))
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
		return &EmptyLit{ExprBase: b.eb(l.P, text)}
	}
	out := &ListLit{ExprBase: b.eb(l.P, text)}
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
		return &ActionExpr{ExprBase: b.eb(p.P, printSentence(s)), Stmt: r.S}
	}
	return b.bad(p.P, printItem(p))
}

// wordExpr resolves a bare word in argument position.
func (b *body) wordExpr(w *parse.Word, hole *Param) Expr {
	text := w.Name
	switch w.Name {
	case "true", "false":
		return &Lit{ExprBase: b.eb(w.P, text), Kind: LitBool, Bool: w.Name == "true"}
	}
	if hole != nil && isFieldType(hole.Type) {
		if fl := b.r.pkg.fields[w.Name]; fl != nil {
			b.r.mark(b.f, w.P, RoleField, &Def{File: fl.File.Path, Pos: fl.Pos})
			return &FieldRef{ExprBase: b.eb(w.P, text), Field: fl}
		}
	}
	if sym, _ := b.lookup(w.Name); sym != nil {
		useSym(sym, w.P)
		b.markSym(w.P, sym)
		return &Ref{ExprBase: b.eb(w.P, text), Sym: sym}
	}
	if ph := b.zeroPhrase(w.Name); ph != nil {
		return b.zeroCall(ph, w.P, text, nil)
	}
	if imp, own := b.qualifier(w.Name); imp != nil || own {
		if pk := b.qualPkg(imp, own); pk != nil {
			if imp != nil {
				imp.refs = append(imp.refs, w.P)
			}
			b.r.mark(b.f, w.P, RoleName, nil)
			return &PkgRef{ExprBase: b.eb(w.P, text), Pkg: pk}
		}
		return b.bad(w.P, text)
	}
	b.unknownName(w.P, text, w.Name)
	return b.bad(w.P, text)
}

func (b *body) markSym(pos source.Pos, s Symbol) {
	b.r.mark(b.f, pos, RoleName, &Def{File: b.f.Path, Pos: symPos(s)})
}

// zeroCall is a call of a zero-hole phrase.
func (b *body) zeroCall(ph *Phrase, pos source.Pos, text string, imp *Import) Expr {
	if b.kind == bkConst && ph.Native != "sim.nobody" {
		b.constErr(pos)
	}
	b.r.mark(b.f, pos, RolePhrase, &Def{File: ph.File.Path, Pos: ph.Pos})
	return &Call{ExprBase: b.eb(pos, text), Phrase: ph, Qual: imp}
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
				base := &Ref{ExprBase: b.eb(m.P, parts[0]+"."+parts[1]), Sym: bd}
				if len(parts) == 2 {
					return base
				}
				return b.selectOf(m, base, 2)
			}
		}
	}
	if sym, _ := b.lookup(parts[0]); sym != nil {
		useSym(sym, m.P)
		base := &Ref{ExprBase: b.eb(m.P, parts[0]), Sym: sym}
		return b.selectOf(m, base, 1)
	}
	if ph := b.zeroPhrase(parts[0]); ph != nil {
		base := b.zeroCall(ph, m.P, parts[0], nil)
		return b.selectOf(m, base, 1)
	}
	if imp, own := b.qualifier(parts[0]); imp != nil || own {
		if imp != nil {
			imp.refs = append(imp.refs, m.P)
		}
		pk := b.qualPkg(imp, own)
		if pk == nil {
			return b.bad(m.P, text)
		}
		if len(parts) != 2 {
			b.r.err(b.f, partPos(m, 2), source.UnknownPhrase, `no phrase matches "`+text+`"`, "")
			return b.bad(m.P, text)
		}
		if own {
			b.r.addFix(b.f, format.Fix{Pos: m.P, DropPrefix: true, Part: 0})
		}
		pos1 := partPos(m, 1)
		if hole != nil && isFieldType(hole.Type) {
			if fl := pk.fields[parts[1]]; fl != nil {
				if !fl.Exported && !own {
					b.r.err(b.f, pos1, source.Unexported, `"`+text+`" is not exported by `+parts[0],
						"only names starting upper-case are visible outside "+parts[0])
				}
				b.r.mark(b.f, pos1, RoleField, &Def{File: fl.File.Path, Pos: fl.Pos})
				return &FieldRef{ExprBase: b.eb(m.P, text), Field: fl}
			}
		}
		for _, ph := range pk.heads[parts[1]] {
			if usable(ph) && ph.Receiver == nil && ph.Zero {
				if !ph.Exported && !own {
					b.r.err(b.f, pos1, source.Unexported, `"`+text+`" is not exported by `+parts[0],
						"only names starting upper-case are visible outside "+parts[0])
				}
				return b.zeroCall(ph, m.P, text, imp)
			}
		}
		b.r.err(b.f, m.P, source.UnknownPhrase, `no phrase matches "`+text+`"`, b.closestMember(imp, own, parts[1]))
		return b.bad(m.P, text)
	}
	b.unknownName(m.P, text, parts[0])
	return b.bad(m.P, text)
}

// selectOf builds the Select of parts[from:] on base. A package qualifier
// right after the base is marked on the part; the own package's qualifier is
// dropped, with a fix.
func (b *body) selectOf(m *parse.Member, base Expr, from int) Expr {
	text := strings.Join(m.Parts, ".")
	sel := &Select{ExprBase: b.eb(m.P, text), Base: base}
	for i := from; i < len(m.Parts); i++ {
		pt := Part{Name: m.Parts[i], Pos: partPos(m, i)}
		if i == from && len(m.Parts) > from+1 {
			imp, own := b.qualifier(m.Parts[i])
			switch {
			case own:
				b.r.addFix(b.f, format.Fix{Pos: m.P, DropPrefix: true, Part: i})
				continue
			case imp != nil:
				imp.typeRefs = true // a qualifier past the first part cannot be renamed by a fix
				if imp.Pkg == nil {
					return b.bad(m.P, text)
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
			return rec, true
		}
		hint := ""
		if c := closest(v.Name, sortedKeys(b.r.pkg.records)); c != "" {
			hint = `did you mean "` + c + `"?`
		}
		b.r.err(b.f, v.P, source.UnknownPhrase, `no record type "`+v.Name+`"`, hint)
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
						b.r.err(b.f, partPos(v, 1), source.Unexported, `"`+printItem(v)+`" is not exported by `+v.Parts[0],
							"only names starting upper-case are visible outside "+v.Parts[0])
					}
					return rec, true
				}
				b.r.err(b.f, v.P, source.UnknownPhrase, `no record type "`+printItem(v)+`"`, "")
				return nil, false
			}
			if imp != nil {
				return nil, false
			}
			if b.r.isStd(v.Parts[0]) {
				b.r.err(b.f, v.P, source.UnknownPhrase, `"`+v.Parts[0]+`" is not imported`, `add "use std/`+v.Parts[0]+`"`)
				return nil, false
			}
		}
		b.r.err(b.f, v.P, source.UnknownPhrase, `no record type "`+printItem(v)+`"`, "")
	default:
		b.r.err(b.f, it.Pos(), source.UnknownPhrase, `no record type "`+printItem(it)+`"`, "")
	}
	return nil, false
}

// messageExpr resolves "message T with a x, b y".
func (b *body) messageExpr(s *parse.Sentence) Expr {
	g := s.Groups[0]
	text := printSentence(s)
	pos := g[0].Pos()
	if len(g) < 3 || !parse.IsWord(g[2], "with") || len(g) == 3 {
		b.r.err(b.f, pos, source.UnknownPhrase, `no phrase matches "`+text+`"`, "write \"message <Type> with <field> <value>\"")
		b.touch(g)
		return b.bad(pos, text)
	}
	rec, ok := b.recordOf(g[1])
	// settings: group 0 holds the first after "with".
	var groups [][]parse.Item
	groups = append(groups, g[3:])
	groups = append(groups, s.Groups[1:]...)
	lit := &RecordLit{ExprBase: b.eb(pos, text), Record: rec}
	valid := ok
	seen := map[string]bool{}
	var names []string
	for _, grp := range groups {
		if len(grp) != 2 {
			var hint string
			if len(grp) > 2 {
				hint = `did you mean "` + printItems(grp[:1]) + " (" + printItems(grp[1:]) + `)"?`
			}
			b.r.err(b.f, grp[0].Pos(), source.UnknownPhrase, `no phrase matches "`+printItems(grp)+`"`, hint)
			b.touch(grp)
			valid = false
			continue
		}
		w, isWord := grp[0].(*parse.Word)
		val := b.arg(grp[1], nil)
		if !isWord {
			b.r.err(b.f, grp[0].Pos(), source.UnknownPhrase, `no phrase matches "`+printItems(grp)+`"`, "")
			valid = false
			continue
		}
		if rec == nil {
			continue
		}
		fl := rec.Lookup(w.Name)
		if fl == nil {
			hint := ""
			var fn []string
			for _, f := range rec.Fields {
				fn = append(fn, f.Name)
			}
			if c := closest(w.Name, fn); c != "" {
				hint = `did you mean "` + c + `"?`
			}
			b.r.err(b.f, w.P, source.UnknownPhrase, `record "`+rec.Name+`" has no field "`+w.Name+`"`, hint)
			valid = false
			continue
		}
		b.r.mark(b.f, w.P, RoleField, &Def{File: fl.File.Path, Pos: fl.Pos})
		if seen[w.Name] {
			b.r.err(b.f, w.P, source.DuplicateName, `field "`+w.Name+`" is set twice`, "")
			valid = false
			continue
		}
		seen[w.Name] = true
		names = append(names, w.Name)
		lit.Settings = append(lit.Settings, Setting{Field: fl, Value: val, Pos: w.P})
	}
	if valid && rec != nil && len(names) > 1 {
		order := func() []string {
			var out []string
			for _, f := range rec.Fields {
				if seen[f.Name] {
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
		return b.bad(pos, text)
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
	b.r.err(b.f, items[0].Pos(), source.AmbiguousPhrase, `"`+printSentence(s)+`" matches more than one phrase`,
		b.ambiguousHint(m))
	b.touch(items)
	return res{}
}

func (b *body) ambiguousHint(m []candidate) string {
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
	return desc(m[0]) + " and " + desc(m[1])
}

// bind binds the one candidate that matched.
func (b *body) bind(c candidate, s *parse.Sentence, items []parse.Item, ln *parse.Line) res {
	f := c.f
	text := printSentence(s)
	pos := items[0].Pos()
	switch f.kind {
	case fkPhrase:
		return res{E: b.bindPhrase(c, text, items, ln)}
	case fkSet:
		return b.bindSet(text, items, nil)
	case fkSetItem:
		return b.bindSet(text, items, items)
	case fkIf:
		st := &If{At: At{pos}, Cond: b.arg(items[1], nil)}
		st.Then = b.block(ln.Block, "")
		if len(ln.Conts) > 0 {
			st.Else = b.block(ln.Conts[0].Block, "")
		}
		return res{S: st}
	case fkAfter:
		st := &After{At: At{pos}, Delay: b.arg(items[1], nil)}
		st.Body = b.block(ln.Block, "after")
		return res{S: st}
	case fkDeliver:
		return res{S: &Deliver{At: At{pos}, Rec: b.arg(items[1], nil), To: b.arg(items[3], nil)}}
	case fkEmit:
		return res{S: &Emit{At: At{pos}, Rec: b.arg(items[1], nil), By: b.arg(items[3], nil)}}
	case fkResult:
		return b.bindResult(pos, items)
	case fkDraw:
		return b.bindDraw(pos, items, ln)
	case fkItem:
		return res{E: &Item{ExprBase: b.eb(pos, text), Key: b.arg(items[1], nil), Coll: b.arg(items[3], nil)}}
	case fkNot:
		return res{E: &Not{ExprBase: b.eb(pos, text), X: b.arg(items[1], nil)}}
	case fkBinary:
		return res{E: &Binary{ExprBase: b.eb(pos, text), Op: f.op, A: b.arg(items[0], nil),
			B: b.arg(items[len(items)-1], nil), OpPos: items[1].Pos()}}
	case fkAttach:
		w := items[0].(*parse.Word)
		if Exported(w.Name) {
			b.r.err(b.f, w.P, source.NonCanonical,
				`"`+w.Name+`" must start lower-case: only exported names start upper-case`, "rename it")
		}
		b.r.mark(b.f, w.P, RoleName, nil)
		return res{E: &Attachment{ExprBase: b.eb(pos, text), Name: w.Name, Key: b.arg(items[2], nil)}}
	}
	return res{}
}

func (b *body) bindResult(pos source.Pos, items []parse.Item) res {
	switch {
	case b.kind != bkPhrase:
		b.r.err(b.f, pos, source.Context, `"result" stands only in a phrase definition`, "")
	case b.noResult > 0:
		b.r.err(b.f, pos, source.Context, `"result" cannot stand in an after block or a block argument`, "")
	}
	var v Expr
	if c, ok := items[1].(*parse.Color); ok && b.ph != nil && b.ph.Zero && b.ph.Receiver == nil && b.kind == bkPhrase {
		v = &ColorLit{ExprBase: b.eb(c.P, c.Text), Hex: strings.ToUpper(c.Text)}
	} else {
		v = b.arg(items[1], nil)
	}
	return res{S: &Result{At: At{pos}, Value: v}}
}

func (b *body) bindDraw(pos source.Pos, items []parse.Item, ln *parse.Line) res {
	st := &Draw{At: At{pos}, Shape: b.arg(items[1], nil)}
	if len(items) == 6 {
		st.For = b.arg(items[5], nil)
	}
	w, ok := items[3].(*parse.Word)
	if !ok {
		b.r.err(b.f, items[3].Pos(), source.UnknownPhrase, `no phrase matches "`+printItems(items)+`"`, "a draw name is a word")
		return res{S: &BadStmt{At{pos}}}
	}
	if old, ok := b.sc.syms[w.Name].(*Local); ok && old.Kind == LocalDraw {
		st.Name, st.Rebind = old, true
		old.refs = append(old.refs, w.P)
	} else {
		st.Name = b.declLocal(w.Name, w.P, LocalDraw)
	}
	if len(items) == 6 {
		st.Name.Attached = true
		if Exported(w.Name) {
			// The attachment name is global; no local fix can rename it.
			b.r.err(b.f, w.P, source.NonCanonical,
				`"`+w.Name+`" must start lower-case: only exported names start upper-case`, "rename it")
		}
	}
	b.r.mark(b.f, w.P, RoleName, &Def{File: b.f.Path, Pos: st.Name.Pos})
	if ln != nil && ln.Block != nil {
		st.Body = b.block(ln.Block, "")
	}
	return res{S: st}
}

// bindPhrase binds the arguments of a phrase call.
func (b *body) bindPhrase(c candidate, text string, items []parse.Item, ln *parse.Line) Expr {
	ph := c.f.ph
	call := &Call{ExprBase: b.eb(items[0].Pos(), text), Phrase: ph, Qual: c.qual}
	if b.kind == bkConst {
		b.constErr(items[0].Pos())
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
				call.Receiver = &Ref{ExprBase: b.eb(m.P, c.recv), Sym: sym}
			} else if zp := b.zeroPhrase(c.recv); zp != nil {
				call.Receiver = b.zeroCall(zp, m.P, c.recv, nil)
			}
		}
		if c.own {
			b.r.addFix(b.f, format.Fix{Pos: m.P, DropPrefix: true, Part: c.qualPart})
		}
		if !ph.Exported && ph.Pkg != b.r.pkg {
			q := m.Parts[c.qualPart]
			b.r.err(b.f, partPos(m, hp), source.Unexported, `"`+strings.Join(m.Parts, ".")+`" is not exported by `+q,
				"only names starting upper-case are visible outside "+q)
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
		call.Blocks = append(call.Blocks, b.block(ln.Block, "a block argument"))
		for _, ct := range ln.Conts {
			call.Blocks = append(call.Blocks, b.block(ct.Block, "a block argument"))
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
			}
			if imp := b.f.byQual[v.Name]; imp != nil {
				imp.Used = true
			}
		case *parse.Member:
			if s, _ := b.lookup(v.Parts[0]); s != nil {
				useSym(s, v.P)
			}
			for _, p := range v.Parts {
				if imp := b.f.byQual[p]; imp != nil {
					imp.Used = true
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
