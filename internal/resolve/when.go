package resolve

import (
	"github.com/synodlang/compiler/internal/parse"
	"github.com/synodlang/compiler/internal/source"
	"github.com/synodlang/compiler/internal/stdlib"
	"github.com/synodlang/compiler/internal/types"
)

// eventCandidates finds the event phrases an event sentence can name.
func (b *body) eventCandidates(items []parse.Item) []candidate {
	var out []candidate
	pkg := b.r.pkg
	switch it := items[0].(type) {
	case *parse.Word:
		for _, ph := range pkg.heads[it.Name] {
			if ph.IsEvent && !ph.dup {
				out = append(out, candidate{f: phraseForm(ph), qualPart: -1})
			}
		}
	case *parse.Member:
		if len(it.Parts) == 2 {
			imp, own := b.qualifier(it.Parts[0])
			if pk := b.qualPkg(imp, own); pk != nil {
				if imp != nil {
					imp.refs = append(imp.refs, it.P)
				}
				for _, ph := range pk.heads[it.Parts[1]] {
					if ph.IsEvent && !ph.dup {
						out = append(out, candidate{f: phraseForm(ph), qual: imp, own: own, qualPart: 0})
					}
				}
			}
		}
	}
	return out
}

func (r *resolver) bodyWhen(f *File, l *parse.Line) {
	g := l.Sentence.Groups[0]
	p := r.pkg
	b := r.newBody(f, bkWhen, nil)
	defer b.pop()
	items := g[1:]
	var recvHole *parse.Hole
	if len(items) > 0 {
		if h, ok := items[0].(*parse.Hole); ok {
			recvHole = h
			items = items[1:]
		}
	}
	fail := func(pos source.Pos, text, hint string) {
		r.err(f, pos, source.UnknownPhrase, `no event phrase matches "`+text+`"`, hint)
		for _, it := range items {
			b.touch([]parse.Item{it})
		}
		b.touchBody(l)
	}
	if len(items) == 0 {
		fail(g[0].Pos(), "when", `write "when (<event phrase>)"`)
		return
	}
	var ev []parse.Item
	switch {
	case len(items) == 1:
		if pr, ok := items[0].(*parse.Paren); ok {
			ev = pr.S.Groups[0]
		} else {
			ev = items
		}
	default:
		fail(items[0].Pos(), printItems(items), `did you mean "when (`+printItems(items)+`)"?`)
		return
	}
	text := printItems(ev)
	var m []candidate
	for _, c := range b.eventCandidates(ev) {
		if c.f.shape(ev, nil) {
			m = append(m, c)
		}
	}
	if len(m) == 0 {
		fail(ev[0].Pos(), text, b.eventHint(ev))
		return
	}
	if len(m) > 1 {
		r.err(f, ev[0].Pos(), source.AmbiguousPhrase, `"`+text+`" matches more than one phrase`, b.ambiguousHint(m))
		b.touchBody(l)
		return
	}
	c := m[0]
	ph := c.f.ph
	h := &Handler{Pkg: p, File: f, Pos: l.Pos, Event: ph, Kind: ph.Event}
	evName := printItem(ev[0])
	if mem, ok := ev[0].(*parse.Member); ok {
		if c.own {
			r.addFix(f, formatDrop(mem.P, 0))
		}
		if !ph.Exported && !c.own {
			r.err(f, partPos(mem, 1), source.Unexported, `"`+evName+`" is not exported by `+mem.Parts[0], "")
		}
		r.mark(f, partPos(mem, 1), RolePhrase, &Def{File: ph.File.Path, Pos: ph.Pos})
	}
	// Receiver rules.
	switch {
	case p.IsMain && recvHole != nil:
		r.err(f, recvHole.P, source.BadReceiver, "a main file has no receivers", "")
	case p.IsMain && ph.Event == stdlib.NodeEvent:
		r.err(f, g[0].Pos(), source.BadReceiver, "a main file has no node-event when", "")
	case ph.Event == stdlib.NodeEvent && recvHole == nil:
		r.err(f, g[0].Pos(), source.BadReceiver, `"`+evName+`" is a node event: write "when [n: Node] (…)"`, "")
	case ph.Event == stdlib.TraceEvent && recvHole != nil:
		r.err(f, recvHole.P, source.BadReceiver, `"`+evName+`" is a trace event: remove the receiver`, "")
	}
	if ph.Event == stdlib.TraceEvent {
		p.Observers = true
	}
	if recvHole != nil {
		nt, ok := recvHole.Type.(*parse.NamedType)
		bd := &Binding{Name: recvHole.Name.Name, Kind: BindReceiver, Pos: recvHole.Name.P, Hole: -1, Type: types.TInvalid}
		if !ok || nt.Name != "Node" {
			if !p.IsMain {
				r.err(f, recvHole.P, source.BadReceiver, "the receiver of a when is [n: Node]", "")
			}
		} else if p.Node != nil {
			bd.Type = p.Node.Type()
		}
		b.declBinding(bd)
		h.Receiver = bd
	}
	// Holes.
	idx := 1
	for _, it := range ph.Items[1:] {
		if it.Param != nil {
			h.Matchers = append(h.Matchers, b.matchHole(it.Param, ev[idx]))
		} else if w, ok := ev[idx].(*parse.Word); ok {
			r.mark(f, w.P, RolePhrase, nil)
		}
		idx++
	}
	var fieldM *Field
	for _, mt := range h.Matchers {
		if mt.Kind == MatchField {
			fieldM = mt.Field
		}
	}
	if fieldM != nil {
		for _, mt := range h.Matchers {
			if mt.Kind == MatchBind && mt.Binding != nil && types.Equal(mt.Binding.Type, types.TValue) {
				mt.Binding.OfField = fieldM
			}
		}
	}
	if l.Block == nil {
		r.err(f, l.Pos, source.BadPattern, "a when needs an indented block", "")
	} else {
		h.Body = &Block{At: At{l.Block.Lines[0].Pos}, Stmts: b.stmts(l.Block)}
	}
	h.Bindings = b.bindings
	h.Locals = b.locals
	p.Handlers = append(p.Handlers, h)
}

func (b *body) matchHole(prm *Param, it parse.Item) Matcher {
	r := b.r
	p := r.pkg
	mt := Matcher{Hole: prm.Index, Pos: it.Pos()}
	isMsg := types.Equal(prm.Type, types.TMsg)
	isField := types.Equal(prm.Type, types.TField)
	equal := func() Matcher {
		mt.Kind = MatchEqual
		mt.Value = b.arg(it, prm)
		return mt
	}
	switch v := it.(type) {
	case *parse.Hole:
		mt.Kind = MatchNodeType
		t := r.typ(b.f, v.Type, false, false)
		nt, isNode := t.(types.NodeOf)
		if !types.Equal(prm.Type, types.TNode) {
			r.err(b.f, v.P, source.TypeMismatch,
				`"`+holeText(v)+`" has a node type, but the hole "`+prm.Name+`" has type `+prm.Source, "")
		} else if !isNode && !isInvalid(t) {
			r.err(b.f, v.P, source.BadPattern, "a typed binding names a node type: p.Node", "")
		}
		bd := &Binding{Name: v.Name.Name, Kind: BindNodeType, Pos: v.Name.P, Type: t, Hole: prm.Index}
		if isNode {
			mt.NodePkg = b.pkgByPath(nt.Pkg)
		}
		b.declBinding(bd)
		mt.Binding = bd
		return mt
	case *parse.Word:
		if isMsg {
			if rec := p.records[v.Name]; rec != nil {
				mt.Kind, mt.Record = MatchRecord, rec
				bd := &Binding{Name: v.Name, Kind: BindRecord, Pos: v.P, Type: rec.Type(), Record: rec, Hole: prm.Index}
				b.declBinding(bd)
				mt.Binding = bd
				r.mark(b.f, v.P, RoleType, &Def{File: rec.File.Path, Pos: rec.Pos})
				return mt
			}
		}
		if isField {
			if fl := p.fields[v.Name]; fl != nil {
				mt.Kind, mt.Field = MatchField, fl
				r.mark(b.f, v.P, RoleField, &Def{File: fl.File.Path, Pos: fl.Pos})
				return mt
			}
		}
		if v.Name == "true" || v.Name == "false" {
			return equal()
		}
		if sym, _ := b.lookup(v.Name); sym != nil || b.zeroPhrase(v.Name) != nil {
			return equal()
		}
		if imp, own := b.qualifier(v.Name); imp != nil || own {
			return equal()
		}
		t := prm.Type
		if isMsg || isField && false {
			t = types.TMsg
		}
		mt.Kind = MatchBind
		bd := &Binding{Name: v.Name, Kind: BindHole, Pos: v.P, Type: t, Hole: prm.Index}
		b.declBinding(bd)
		mt.Binding = bd
		b.markSym(v.P, bd)
		return mt
	case *parse.Member:
		if (isMsg || isField) && len(v.Parts) == 2 {
			imp, own := b.qualifier(v.Parts[0])
			if pk := b.qualPkg(imp, own); pk != nil {
				if imp != nil {
					imp.refs = append(imp.refs, v.P)
				}
				pos1 := partPos(v, 1)
				if isMsg {
					if rec := pk.records[v.Parts[1]]; rec != nil {
						if own {
							r.addFix(b.f, formatDrop(v.P, 0))
						} else if !rec.Exported {
							r.err(b.f, pos1, source.Unexported, `"`+printItem(v)+`" is not exported by `+v.Parts[0],
								"only names starting upper-case are visible outside "+v.Parts[0])
						}
						mt.Kind, mt.Record = MatchRecord, rec
						name := v.Parts[0] + "." + v.Parts[1]
						if own {
							name = v.Parts[1]
						}
						bd := &Binding{Name: name, Kind: BindRecord, Pos: v.P, Type: rec.Type(), Record: rec, Hole: prm.Index}
						b.declBinding(bd)
						mt.Binding = bd
						r.mark(b.f, pos1, RoleType, &Def{File: rec.File.Path, Pos: rec.Pos})
						return mt
					}
				}
				if isField {
					if fl := pk.fields[v.Parts[1]]; fl != nil {
						if own {
							r.addFix(b.f, formatDrop(v.P, 0))
						} else if !fl.Exported {
							r.err(b.f, pos1, source.Unexported, `"`+printItem(v)+`" is not exported by `+v.Parts[0],
								"only names starting upper-case are visible outside "+v.Parts[0])
						}
						mt.Kind, mt.Field = MatchField, fl
						r.mark(b.f, pos1, RoleField, &Def{File: fl.File.Path, Pos: fl.Pos})
						return mt
					}
				}
			}
		}
	}
	return equal()
}

func (b *body) pkgByPath(path string) *Package {
	if b.r.pkg.Path == path {
		return b.r.pkg
	}
	for _, imp := range b.r.pkg.Imports {
		if imp.Pkg != nil && imp.Pkg.Path == path {
			return imp.Pkg
		}
	}
	return nil
}
