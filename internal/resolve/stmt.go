package resolve

import (
	"strconv"

	"github.com/synodlang/compiler/internal/parse"
	"github.com/synodlang/compiler/internal/source"
	"github.com/synodlang/compiler/internal/stdlib"
	"github.com/synodlang/compiler/internal/types"
)

// ---- bodies of declarations ----

func (r *resolver) bodyDefine(f *File, l *parse.Line) {
	ph := r.byLine[l]
	if ph == nil {
		return
	}
	kind := bkPhrase
	if ph.Kind != PhraseNormal {
		kind = bkMain
	}
	b := r.newBody(f, kind, ph)
	if ph.Receiver != nil {
		b.declParam(ph.Receiver)
	}
	for _, it := range ph.Items {
		if it.Param != nil {
			b.declParam(it.Param)
		}
	}
	for _, bs := range ph.Blocks {
		b.declParam(bs.Param)
	}
	if l.Block != nil && ph.Native == "" {
		ph.Body = &Block{At: At{l.Block.Lines[0].Pos}, Stmts: b.stmts(l.Block)}
	}
	ph.Locals = b.locals
	b.pop()
}

func (r *resolver) bodyHas(f *File, l *parse.Line) {
	for _, h := range r.hasLines {
		if h.line != l {
			continue
		}
		b := r.newBody(f, bkConst, nil)
		for _, bl := range l.Block.Lines {
			g := bl.Sentence.Groups[0]
			if len(g) != 4 || !parse.IsWord(g[0], "set") || !parse.IsWord(g[2], "to") || bl.Block != nil {
				continue
			}
			w, ok := g[1].(*parse.Word)
			if !ok {
				continue
			}
			for _, fl := range h.rec.Fields {
				if fl.Pos == w.P {
					fl.Init = b.arg(g[3], nil)
				}
			}
		}
		b.pop()
	}
}

func (r *resolver) bodyAlways(f *File, l *parse.Line) {
	g := l.Sentence.Groups[0]
	p := r.pkg
	p.Observers = true
	b := r.newBody(f, bkAlways, nil)
	defer b.pop()
	if len(g) != 2 || l.Block != nil {
		text := printSentence(l.Sentence)
		hint := ""
		if len(g) > 2 {
			hint = `did you mean "always (` + printItems(g[1:]) + `)"?`
		}
		r.err(f, g[0].Pos(), source.UnknownPhrase, `no phrase matches "`+text+`"`, hint)
		b.touch(g)
		return
	}
	inv := &Invariant{Pkg: p, File: f, Pos: l.Pos, Cond: b.arg(g[1], nil),
		Rule: f.Path + ":" + itoa(l.Pos.Line) + ": always " + printItem(g[1])}
	p.Invariants = append(p.Invariants, inv)
}

// ---- statements ----

func (b *body) stmts(blk *parse.Block) []Stmt {
	var out []Stmt
	for _, ln := range blk.Lines {
		if s := b.stmt(ln); s != nil {
			out = append(out, s)
		}
	}
	return out
}

// block resolves a block in a new scope. boundary is "after" or "a block
// argument" for blocks that run later on a copy of the locals.
func (b *body) block(blk *parse.Block, boundary string) *Block {
	b.push(boundary)
	if boundary != "" {
		b.noResult++
	}
	out := &Block{At: At{blk.Lines[0].Pos}, Stmts: b.stmts(blk)}
	if boundary != "" {
		b.noResult--
	}
	b.pop()
	return out
}

func (b *body) stmt(ln *parse.Line) Stmt {
	s := ln.Sentence
	items := s.Groups[0]
	pos := ln.Pos
	if w, ok := items[0].(*parse.Word); ok {
		switch {
		case w.Name == "use" || w.Name == "define" || w.Name == "when" || w.Name == "always",
			len(items) == 2 && parse.IsWord(items[1], "has") && ln.Block != nil:
			name := w.Name
			if name != "use" && name != "define" && name != "when" && name != "always" {
				name = "has"
			}
			b.r.err(b.f, w.P, source.Context, `"`+name+`" stands only at file root`, "")
			return &BadStmt{At{pos}}
		case w.Name == "each":
			return b.eachStmt(ln)
		case len(items) == 1 && ln.Block == nil:
			if sym, _ := b.lookup(w.Name); sym != nil {
				if prm, ok := sym.(*Param); ok && prm.Kind == ParamBlock {
					useSym(prm, w.P)
					return &RunBlock{At: At{pos}, Param: prm}
				}
			}
		}
	}
	r := b.sentence(s, ln)
	switch {
	case r.S != nil:
		return r.S
	case r.E != nil:
		if c, ok := r.E.(*Call); ok {
			return &CallStmt{At: At{pos}, Call: c}
		}
		return &ExprStmt{At: At{pos}, X: r.E}
	}
	return &BadStmt{At{pos}}
}

func (b *body) eachStmt(ln *parse.Line) Stmt {
	s := ln.Sentence
	pos := ln.Pos
	var names []*parse.Word
	var over parse.Item
	ok := false
	switch g := s.Groups; {
	case len(g) == 1 && len(g[0]) == 4 && parse.IsWord(g[0][2], "in"):
		if w, isW := g[0][1].(*parse.Word); isW {
			names, over, ok = []*parse.Word{w}, g[0][3], true
		}
	case len(g) == 2 && len(g[0]) == 2 && len(g[1]) == 3 && parse.IsWord(g[1][1], "in"):
		w1, ok1 := g[0][1].(*parse.Word)
		w2, ok2 := g[1][0].(*parse.Word)
		if ok1 && ok2 {
			names, over, ok = []*parse.Word{w1, w2}, g[1][2], true
		}
	}
	if !ok || ln.Block == nil || len(ln.Conts) > 0 {
		b.r.err(b.f, pos, source.UnknownPhrase, `no phrase matches "`+printSentence(s)+`"`,
			`write "each <name> in <collection>" or "each <index>, <item> in <collection>" with an indented block`)
		b.touch(s.Groups[0])
		return &BadStmt{At{pos}}
	}
	st := &Each{At: At{pos}, Over: b.arg(over, nil)}
	b.push("")
	st.A = b.declLocal(names[0].Name, names[0].P, LocalEach)
	if len(names) == 2 {
		st.B = b.declLocal(names[1].Name, names[1].P, LocalEach)
		st.A.eachIdx, st.A.eachOther = 0, names[1].Name
		st.B.eachIdx, st.B.eachOther = 1, names[0].Name
	}
	for _, n := range names {
		b.r.mark(b.f, n.P, RoleName, &Def{File: b.f.Path, Pos: n.P})
	}
	body := &Block{At: At{ln.Block.Lines[0].Pos}, Stmts: b.stmts(ln.Block)}
	st.Body = body
	b.pop()
	return st
}

// bindSet resolves "set place to value" and "set item key of place to value".
func (b *body) bindSet(text string, items []parse.Item, itemForm []parse.Item) res {
	pos := items[0].Pos()
	if itemForm != nil {
		val := b.arg(items[6], nil)
		key := b.arg(items[2], nil)
		base := b.itemBase(items[4])
		if base == nil {
			return res{S: &BadStmt{At{pos}}}
		}
		return res{S: &Set{At: At{pos}, Place: &ItemPlace{At: At{items[1].Pos()}, Base: base, Key: key}, Value: val}}
	}
	val := b.arg(items[3], nil)
	pl, nl := b.place(items[1])
	return res{S: &Set{At: At{pos}, Place: pl, Value: val, NewLocal: nl}}
}

func (b *body) badPlace(pos source.Pos, name, what string) {
	b.r.err(b.f, pos, source.BadPlace, `cannot set "`+name+`": it is `+what, "")
}

func article(kind string) string {
	switch kind {
	case "each name", "":
		return "an " + kind
	}
	return "a " + kind
}

// checkSettable reports a symbol that cannot be set.
func (b *body) checkSettable(sym Symbol, crossed string, pos source.Pos, name string) bool {
	switch x := sym.(type) {
	case *Local:
		if x.Kind == LocalEach {
			b.badPlace(pos, name, "an each name")
			return false
		}
		if crossed != "" {
			x.Tainted = true // the set is lost, and with it what it says of the type
			b.r.err(b.f, pos, source.BadPlace, `"`+name+`" is captured by `+crossed+`; it cannot be set here`, "")
			return false
		}
		return true
	case *Param:
		if x.Kind == ParamReceiver {
			b.badPlace(pos, name, "a receiver")
		} else {
			b.badPlace(pos, name, "a hole parameter")
		}
	case *Binding:
		b.badPlace(pos, name, "a when binding")
	}
	return false
}

// itemBase resolves the collection of "set item k of base".
func (b *body) itemBase(it parse.Item) Place {
	switch v := it.(type) {
	case *parse.Word:
		sym, crossed := b.lookup(v.Name)
		if sym == nil {
			b.unknownName(v.P, v.Name, v.Name)
			return nil
		}
		useSym(sym, v.P)
		b.markSym(v.P, sym)
		if !b.checkSettable(sym, crossed, v.P, v.Name) {
			return nil
		}
		return &LocalPlace{At: At{v.P}, Local: sym.(*Local)}
	case *parse.Member:
		return b.memberPlace(v)
	}
	b.r.err(b.f, it.Pos(), source.BadPlace, `cannot set "`+printItem(it)+`": it is not a place`, "")
	return nil
}

func (b *body) memberPlace(m *parse.Member) Place {
	if sym, _ := b.lookup(m.Parts[0]); sym == nil && b.zeroPhrase(m.Parts[0]) == nil {
		if imp, own := b.qualifier(m.Parts[0]); imp != nil || own {
			if imp != nil {
				imp.refs = append(imp.refs, m.P)
			}
			b.badPlace(m.P, printItem(m), "a phrase")
			return nil
		}
	}
	e := b.memberExpr(m, nil)
	switch x := e.(type) {
	case *Select:
		return &SelectPlace{At: At{m.P}, Select: x}
	case *Bad:
		return nil
	case *Call:
		b.badPlace(m.P, x.Text, "a phrase")
	default:
		b.badPlace(m.P, printItem(m), "not a place")
	}
	return nil
}

// place resolves the place of a plain set. A new word creates a local, which
// is returned so that the caller records it.
func (b *body) place(it parse.Item) (Place, *Local) {
	switch v := it.(type) {
	case *parse.Word:
		if sym, crossed := b.lookup(v.Name); sym != nil {
			useSetRef(sym, v.P)
			b.markSym(v.P, sym)
			if !b.checkSettable(sym, crossed, v.P, v.Name) {
				return &BadPlace{At{v.P}}, nil
			}
			return &LocalPlace{At: At{v.P}, Local: sym.(*Local)}, nil
		}
		if b.zeroPhrase(v.Name) != nil {
			b.badPlace(v.P, v.Name, "a phrase")
			return &BadPlace{At{v.P}}, nil
		}
		if imp, own := b.qualifier(v.Name); imp != nil || own {
			b.badPlace(v.P, v.Name, "a package")
			return &BadPlace{At{v.P}}, nil
		}
		l := b.declLocal(v.Name, v.P, LocalSet)
		b.r.mark(b.f, v.P, RoleName, &Def{File: b.f.Path, Pos: v.P})
		return &LocalPlace{At: At{v.P}, Local: l}, l
	case *parse.Member:
		if p := b.memberPlace(v); p != nil {
			return p, nil
		}
		return &BadPlace{At{v.P}}, nil
	}
	b.r.err(b.f, it.Pos(), source.BadPlace, `cannot set "`+printItem(it)+`": it is not a place`, "")
	return &BadPlace{At{it.Pos()}}, nil
}

// useSetRef records a position of a symbol without counting a use.
func useSetRef(s Symbol, pos source.Pos) {
	switch x := s.(type) {
	case *Local:
		x.refs = append(x.refs, pos)
	case *Param:
		x.refs = append(x.refs, pos)
	case *Binding:
		x.refs = append(x.refs, pos)
	}
}

func itoa(n int) string { return strconv.Itoa(n) }

var _ = stdlib.Any
var _ = types.TText
