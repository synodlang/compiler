package resolve

import (
	"github.com/synodlang/compiler/internal/format"
	"github.com/synodlang/compiler/internal/source"
)

// scope is a block scope. Names are visible from their declaration to the
// end of the block, nested blocks included.
type scope struct {
	parent *scope
	syms   map[string]Symbol
	// boundary is "after" or "a block argument" for the scope of such a
	// block: a local from outside cannot be set inside it.
	boundary string
	// below holds every name declared in this scope or one nested in it, as
	// spelled, for deciding whether a rename is free.
	below  map[string]bool
	locals []*Local
	pend   []pendRename
}

type pendRename struct {
	sym  Symbol
	name string
	pos  source.Pos
}

type bodyKind uint8

const (
	bkPhrase bodyKind = iota // a define with a name, receiver or start
	bkMain
	bkWhen
	bkAlways
	bkConst // initial values of a has block
)

// body is the state of resolving one body: a phrase, handler, invariant or
// initial value.
type body struct {
	r    *resolver
	f    *File
	kind bodyKind
	sc   *scope
	ph   *Phrase
	// noResult is the depth of after blocks and block arguments.
	noResult int
	locals   []*Local
	// bindings of a when.
	bindings []*Binding
}

func (r *resolver) newBody(f *File, kind bodyKind, ph *Phrase) *body {
	b := &body{r: r, f: f, kind: kind, ph: ph}
	b.push("")
	return b
}

func (b *body) push(boundary string) {
	b.sc = &scope{parent: b.sc, syms: map[string]Symbol{}, below: map[string]bool{}, boundary: boundary}
}

// pop closes the scope: unused locals, and renames of upper-case names.
func (b *body) pop() {
	s := b.sc
	for _, l := range s.locals {
		if l.Used || l.Kind == LocalDraw {
			continue
		}
		hint := ""
		switch {
		case l.eachIdx == 0 && l.eachOther != "":
			hint = `write "each ` + l.eachOther + ` in …"`
		case l.eachIdx == 1 && l.eachOther != "":
			hint = `write "each ` + l.eachOther + ` in …"`
		}
		b.r.err(b.f, l.Pos, source.UnusedName, `"`+l.Name+`" is never used`, hint)
	}
	for _, pr := range s.pend {
		b.resolveRename(s, pr)
	}
	b.sc = s.parent
}

// lookup finds a name in the scopes. crossed is the boundary kind of the
// innermost after block or block argument between the use and the
// declaration, or "".
func (b *body) lookup(name string) (sym Symbol, crossed string) {
	for s := b.sc; s != nil; s = s.parent {
		if x := s.syms[name]; x != nil {
			return x, crossed
		}
		if s.boundary != "" && crossed == "" {
			crossed = s.boundary
		}
	}
	return nil, ""
}

// symKind names what a symbol is, for shadowing and bad-place messages.
func symKind(s Symbol) string {
	switch x := s.(type) {
	case *Local:
		switch x.Kind {
		case LocalEach:
			return "each name"
		}
		return "local"
	case *Param:
		if x.Kind == ParamReceiver {
			return "receiver"
		}
		return "hole"
	case *Binding:
		return "binding"
	}
	return "name"
}

func symPos(s Symbol) source.Pos {
	switch x := s.(type) {
	case *Local:
		return x.Pos
	case *Param:
		return x.Pos
	case *Binding:
		return x.Pos
	}
	return source.Pos{}
}

// useSym records a reference to a symbol at pos.
func useSym(s Symbol, pos source.Pos) {
	switch x := s.(type) {
	case *Local:
		x.Used = true
		x.refs = append(x.refs, pos)
	case *Param:
		x.refs = append(x.refs, pos)
	case *Binding:
		x.refs = append(x.refs, pos)
	}
}

// hides finds the earlier meaning of name that a new declaration would
// hide: a name in scope, a zero-hole phrase, a record type, an import
// qualifier or the package's own name.
func (b *body) hides(name string) (kind string, where string, found bool) {
	for s := b.sc; s != nil; s = s.parent {
		if x := s.syms[name]; x != nil {
			return symKind(x), " at " + at(b.f, symPos(x)), true
		}
	}
	p := b.r.pkg
	if ph := b.zeroPhrase(name); ph != nil {
		return "phrase", " at " + at(ph.File, ph.Pos), true
	}
	if rec := p.records[name]; rec != nil {
		return "record type", " at " + at(rec.File, rec.Pos), true
	}
	if imp := b.f.byQual[name]; imp != nil {
		return "package", " at " + at(b.f, imp.Pos), true
	}
	if name == p.Name && !p.IsMain {
		return "package", "", true
	}
	return "", "", false
}

// checkDecl runs the reserved-word and shadowing checks of a declaration.
// what is the kind of the new name.
func (b *body) checkDecl(name string, pos source.Pos, what string) {
	if reserved[name] {
		b.r.err(b.f, pos, source.ReservedWord, `"`+name+`" is a reserved word and cannot be a name`, "")
		return
	}
	if kind, where, found := b.hides(name); found {
		b.r.err(b.f, pos, source.Shadowing, `"`+name+`" hides the `+kind+` "`+name+`"`+where, "rename it")
	}
}

func (b *body) markBelow(name string) {
	for s := b.sc; s != nil; s = s.parent {
		s.below[name] = true
	}
}

// declLocal declares a local, each name or draw name in the current scope.
func (b *body) declLocal(name string, pos source.Pos, kind LocalKind) *Local {
	what := "local"
	if kind == LocalEach {
		what = "each name"
	}
	b.checkDecl(name, pos, what)
	l := &Local{Name: name, Kind: kind, Pos: pos, eachIdx: -1, Used: reserved[name]}
	l.refs = append(l.refs, pos)
	b.sc.syms[name] = l
	b.sc.locals = append(b.sc.locals, l)
	b.locals = append(b.locals, l)
	b.markBelow(name)
	if Exported(name) {
		b.sc.pend = append(b.sc.pend, pendRename{l, name, pos})
	}
	return l
}

// declParam declares a hole or receiver of the phrase in the root scope.
func (b *body) declParam(prm *Param) {
	if _, dup := b.sc.syms[prm.Name].(*Param); !dup {
		b.checkDecl(prm.Name, prm.Pos, "hole")
	}
	b.sc.syms[prm.Name] = prm
	b.markBelow(prm.Name)
	prm.refs = append(prm.refs, prm.Pos)
	if Exported(prm.Name) {
		b.sc.pend = append(b.sc.pend, pendRename{prm, prm.Name, prm.Pos})
	}
}

// declBinding declares a name bound by a when.
func (b *body) declBinding(bd *Binding) {
	if bd.Kind != BindRecord {
		b.checkDecl(bd.Name, bd.Pos, "binding")
	}
	b.sc.syms[bd.Name] = bd
	b.markBelow(bd.Name)
	bd.refs = append(bd.refs, bd.Pos)
	b.bindings = append(b.bindings, bd)
	if bd.Kind != BindRecord && Exported(bd.Name) {
		b.sc.pend = append(b.sc.pend, pendRename{bd, bd.Name, bd.Pos})
	}
}

// resolveRename turns an upper-case declaration into a fix when the
// lower-case name is free, and into a non-canonical error when it is not.
func (b *body) resolveRename(s *scope, pr pendRename) {
	lower := lowerFirst(pr.name)
	free := !s.below[lower]
	if free && s.parent != nil {
		if x, _ := b.lookupFrom(s.parent, lower); x != nil {
			free = false
		}
	}
	if free {
		if _, _, found := b.hidesFileLevel(lower); found {
			free = false
		}
	}
	var refs []source.Pos
	switch x := pr.sym.(type) {
	case *Local:
		refs = x.refs
		if x.Attached {
			free = false // an attachment name is global: no local fix can rename it
		}
	case *Param:
		refs = x.refs
	case *Binding:
		refs = x.refs
	}
	if !free {
		b.r.err(b.f, pr.pos, source.NonCanonical,
			`"`+pr.name+`" must start lower-case: only exported names start upper-case`, "rename it")
		return
	}
	for _, p := range refs {
		b.r.addFix(b.f, format.Fix{Pos: p, Rename: lower})
	}
}

func (b *body) lookupFrom(s *scope, name string) (Symbol, string) {
	for ; s != nil; s = s.parent {
		if x := s.syms[name]; x != nil {
			return x, ""
		}
	}
	return nil, ""
}

// hidesFileLevel is hides without the scope chain.
func (b *body) hidesFileLevel(name string) (string, string, bool) {
	save := b.sc
	b.sc = nil
	k, w, f := b.hides(name)
	b.sc = save
	return k, w, f
}
