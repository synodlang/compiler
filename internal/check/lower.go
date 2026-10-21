package check

import (
	"sort"

	"github.com/synodlang/compiler/internal/ir"
	"github.com/synodlang/compiler/internal/resolve"
	"github.com/synodlang/compiler/internal/source"
	"github.com/synodlang/compiler/internal/stdlib"
	"github.com/synodlang/compiler/internal/types"
)

// Lower turns packages that have no diagnostics into IR. all lists the
// packages in dependency order, std packages included, and infos holds the
// result of Package for each. It returns one Program per package that is not
// std, keyed by the package's import path (a main package has "#main"
// appended): a library gets a program without Main, which the simulator can
// use to run its observers.
func Lower(all []*resolve.Package, infos map[string]*Info) map[string]*ir.Program {
	l := &lowerer{
		infos: infos,
		pk:    map[*resolve.Package]*ir.Package{},
		proc:  map[*resolve.Phrase]*ir.Proc{},
		fn:    map[*resolve.Phrase]*fnc{},
		field: map[*resolve.Field]*ir.Field{},
		rec:   map[*resolve.Record]*ir.Record{},
	}
	for _, rp := range all {
		l.declarePackage(rp)
	}
	for _, rp := range all {
		l.declareTypes(rp)
	}
	for _, rp := range all {
		l.declareProcs(rp)
	}
	for _, rp := range all {
		l.lowerPackage(rp)
	}
	out := map[string]*ir.Program{}
	for _, rp := range all {
		if rp.IsStd {
			continue
		}
		out[rp.Path] = l.program(rp)
	}
	return out
}

type lowerer struct {
	infos map[string]*Info
	pk    map[*resolve.Package]*ir.Package
	proc  map[*resolve.Phrase]*ir.Proc
	fn    map[*resolve.Phrase]*fnc
	field map[*resolve.Field]*ir.Field
	rec   map[*resolve.Record]*ir.Record
}

// fnc is the frame being built for one body.
type fnc struct {
	l    *lowerer
	info *Info
	rp   *resolve.Package
	file *resolve.File
	vars []*ir.Var
	by   map[any]*ir.Var
}

func (l *lowerer) newFn(rp *resolve.Package, f *resolve.File) *fnc {
	return &fnc{l: l, info: l.infos[rp.Path], rp: rp, file: f, by: map[any]*ir.Var{}}
}

func (f *fnc) newVar(key any, name string, kind ir.VarKind, t types.Type) *ir.Var {
	v := &ir.Var{Name: name, Kind: kind, Type: t, Slot: len(f.vars)}
	f.vars = append(f.vars, v)
	if key != nil {
		f.by[key] = v
	}
	return v
}

func (f *fnc) pos(p source.Pos) ir.Pos {
	path := ""
	if f.file != nil {
		path = f.file.Path
	}
	return ir.Pos{File: path, Line: p.Line, Col: p.Col}
}

// span is the position of a node with its extent.
func (f *fnc) span(sp source.Span) ir.Pos {
	p := f.pos(sp.Start)
	if sp.End.Line == sp.Start.Line && sp.End.Col > sp.Start.Col {
		p.EndLine, p.EndCol = sp.End.Line, sp.End.Col
	}
	return p
}

func (l *lowerer) declarePackage(rp *resolve.Package) {
	l.pk[rp] = &ir.Package{Path: rp.Path, Name: rp.Name, IsStd: rp.IsStd, IsMain: rp.IsMain}
}

// uses returns the packages rp imports, sorted by path.
func uses(rp *resolve.Package) []*resolve.Package {
	seen := map[*resolve.Package]bool{}
	var out []*resolve.Package
	for _, imp := range rp.Imports {
		if imp.Pkg != nil && !seen[imp.Pkg] {
			seen[imp.Pkg] = true
			out = append(out, imp.Pkg)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}

func (l *lowerer) declareTypes(rp *resolve.Package) {
	p := l.pk[rp]
	info := l.infos[rp.Path]
	for _, u := range uses(rp) {
		p.Uses = append(p.Uses, l.pk[u])
	}
	// Runs: dependencies first, in a stable order, ending with the package.
	seen := map[*resolve.Package]bool{}
	var visit func(q *resolve.Package)
	visit = func(q *resolve.Package) {
		if seen[q] {
			return
		}
		seen[q] = true
		for _, u := range uses(q) {
			visit(u)
		}
		p.Runs = append(p.Runs, l.pk[q])
	}
	visit(rp)
	mk := func(rec *resolve.Record, owner *ir.Record) []*ir.Field {
		var out []*ir.Field
		for _, f := range rec.Fields {
			t := info.FieldTy[f]
			if t == nil {
				t = types.TInvalid
			}
			nf := &ir.Field{Pkg: p, Record: owner, Name: f.Name, Exported: f.Exported, Index: f.Index, Type: t}
			l.field[f] = nf
			out = append(out, nf)
		}
		return out
	}
	if rp.Node != nil {
		p.NodeFields = mk(rp.Node, nil)
	}
	for _, r := range rp.Records {
		nr := &ir.Record{Pkg: p, Name: r.Name, Exported: r.Exported}
		l.rec[r] = nr
		nr.Fields = mk(r, nr)
		p.Records = append(p.Records, nr)
	}
}

func (l *lowerer) declareProcs(rp *resolve.Package) {
	p := l.pk[rp]
	info := l.infos[rp.Path]
	for _, ph := range rp.Phrases {
		pr := &ir.Proc{Name: ph.Name, Pkg: p, Kind: ir.ProcPhrase, Native: ph.Native}
		f := l.newFn(rp, ph.File)
		switch {
		case ph.Kind == resolve.PhraseMain:
			pr.Kind = ir.ProcMain
		case ph.Kind == resolve.PhraseStart:
			pr.Kind = ir.ProcStart
		case ph.Native != "":
			pr.Kind = ir.ProcNative
		}
		if ph.Receiver != nil {
			pr.Receiver = f.newVar(ph.Receiver, ph.Receiver.Name, ir.VarReceiver, ph.Receiver.Type)
		}
		for _, prm := range ph.Params {
			pr.Params = append(pr.Params, f.newVar(prm, prm.Name, ir.VarParam, prm.Type))
		}
		for _, bs := range ph.Blocks {
			pr.BlockParams = append(pr.BlockParams, f.newVar(bs.Param, bs.Param.Name, ir.VarBlock, nil))
		}
		switch {
		case ph.Sig != nil:
			switch ph.Sig.Result.Kind {
			case stdlib.RuleNone:
			case stdlib.RuleFixed:
				pr.Result = ph.Sig.Result.T
			default:
				pr.Result = types.TValue // depends on the arguments: see Call.Type
			}
		case ph.IsValue:
			pr.Result = info.ResultTy[ph]
		}
		pr.Pos = f.pos(ph.Pos)
		l.proc[ph] = pr
		l.fn[ph] = f
		switch pr.Kind {
		case ir.ProcStart:
			p.Start = pr
		case ir.ProcMain:
		default:
			p.Phrases = append(p.Phrases, pr)
		}
	}
}

func (l *lowerer) lowerPackage(rp *resolve.Package) {
	p := l.pk[rp]
	for _, ph := range rp.Phrases {
		pr := l.proc[ph]
		f := l.fn[ph]
		if ph.Body != nil {
			pr.Body = f.block(ph.Body)
		}
		pr.Slots, pr.Vars = len(f.vars), f.vars
	}
	// Initial values are constants: they use no frame.
	fieldInit := func(rf *resolve.Field) {
		if rf.Init == nil {
			return
		}
		f := l.newFn(rp, rf.File)
		l.field[rf].Init = f.expr(rf.Init)
	}
	if rp.Node != nil {
		for _, rf := range rp.Node.Fields {
			fieldInit(rf)
		}
	}
	for _, r := range rp.Records {
		for _, rf := range r.Fields {
			fieldInit(rf)
		}
	}
	for _, h := range rp.Handlers {
		ih := l.handler(rp, h)
		if ih.NodeEvent {
			p.NodeHandlers = append(p.NodeHandlers, ih)
		} else {
			p.Observers = append(p.Observers, ih)
		}
	}
	for _, inv := range rp.Invariants {
		f := l.newFn(rp, inv.File)
		p.Invariants = append(p.Invariants, &ir.Invariant{
			Pkg: p, Cond: f.expr(inv.Cond), Slots: len(f.vars), Rule: inv.Rule, Pos: f.pos(inv.Pos),
		})
	}
}

func lowerKind(k resolve.MatchKind) ir.MatchKind {
	switch k {
	case resolve.MatchRecord:
		return ir.MatchRecord
	case resolve.MatchNodeType:
		return ir.MatchNodeType
	case resolve.MatchField:
		return ir.MatchField
	case resolve.MatchEqual:
		return ir.MatchEqual
	}
	return ir.MatchBind
}

func (l *lowerer) handler(rp *resolve.Package, h *resolve.Handler) *ir.Handler {
	f := l.newFn(rp, h.File)
	out := &ir.Handler{Pkg: l.pk[rp], Event: h.Event.Native, NodeEvent: h.Kind == stdlib.NodeEvent, Pos: f.pos(h.Pos)}
	bindTy := func(b *resolve.Binding) types.Type {
		if b.OfField != nil {
			return f.l.fieldTy(rp, b.OfField)
		}
		return b.Type
	}
	if h.Receiver != nil {
		out.Receiver = f.newVar(h.Receiver, h.Receiver.Name, ir.VarReceiver, bindTy(h.Receiver))
	}
	for _, m := range h.Matchers {
		im := ir.Matcher{Hole: m.Hole, Kind: lowerKind(m.Kind)}
		if m.Binding != nil {
			im.Var = f.newVar(m.Binding, m.Binding.Name, ir.VarBinding, bindTy(m.Binding))
		}
		if m.Record != nil {
			im.Record = l.rec[m.Record]
		}
		if m.NodePkg != nil {
			im.NodePkg = l.pk[m.NodePkg]
		}
		if m.Field != nil {
			im.Field = l.field[m.Field]
		}
		out.Match = append(out.Match, im)
	}
	// Equality values may read the names bound before them.
	for i, m := range h.Matchers {
		if m.Kind == resolve.MatchEqual && m.Value != nil {
			out.Match[i].Value = f.expr(m.Value)
		}
	}
	if h.Body != nil {
		out.Body = f.block(h.Body)
	}
	out.Slots, out.Vars = len(f.vars), f.vars
	return out
}

func (l *lowerer) fieldTy(rp *resolve.Package, rf *resolve.Field) types.Type {
	if in := l.infos[rf.Owner.Pkg.Path]; in != nil {
		if t := in.FieldTy[rf]; t != nil {
			return t
		}
	}
	return types.TInvalid
}

// program assembles the program whose entry is rp.
func (l *lowerer) program(rp *resolve.Package) *ir.Program {
	p := l.pk[rp]
	prog := &ir.Program{Name: rp.Path, Packages: p.Runs}
	if rp.Main != nil {
		prog.Main = l.proc[rp.Main]
	}
	for _, q := range prog.Packages {
		prog.Observers = append(prog.Observers, q.Observers...)
		prog.Invariants = append(prog.Invariants, q.Invariants...)
	}
	return prog
}
