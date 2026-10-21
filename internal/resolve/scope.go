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
	// boundSp is the span of the line that opens the after block or block
	// argument of such a scope.
	boundSp source.Span
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
	sp   source.Span
}

// bound is an enclosing after block or block argument.
type bound struct {
	kind string // "after" or "a block argument"
	sp   source.Span
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
	// bounds are the enclosing after blocks and block arguments, innermost
	// last; noResult is len(bounds).
	bounds []bound
	locals []*Local
	// bindings of a when.
	bindings []*Binding
}

func (r *resolver) newBody(f *File, kind bodyKind, ph *Phrase) *body {
	b := &body{r: r, f: f, kind: kind, ph: ph}
	b.push("", source.Span{})
	return b
}

func (b *body) push(boundary string, sp source.Span) {
	b.sc = &scope{parent: b.sc, syms: map[string]Symbol{}, below: map[string]bool{}, boundary: boundary, boundSp: sp}
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
		b.r.err(b.f, l.Sp, source.UnusedName, `"`+l.Name+`" is never used`, hint)
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
	sym, crossed, _ = b.lookupB(name)
	return sym, crossed
}

// lookupB is lookup that also returns the span of the line that opens the
// crossed after block or block argument.
func (b *body) lookupB(name string) (sym Symbol, crossed string, at source.Span) {
	for s := b.sc; s != nil; s = s.parent {
		if x := s.syms[name]; x != nil {
			return x, crossed, at
		}
		if s.boundary != "" && crossed == "" {
			crossed, at = s.boundary, s.boundSp
		}
	}
	return nil, "", source.Span{}
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

// symSpan is the span of the name where s is declared.
func symSpan(s Symbol) source.Span {
	switch x := s.(type) {
	case *Local:
		return x.Sp
	case *Param:
		return x.Sp
	case *Binding:
		return x.Sp
	}
	return source.Span{}
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

// hidden is the earlier meaning of a name that a new declaration hides.
type hidden struct {
	kind  string
	found bool
	// file and sp tell where it is declared; file is nil for the package's
	// own name.
	file *File
	sp   source.Span
}

// where prints " at <file>:<line>", or "".
func (h hidden) where() string {
	if h.file == nil {
		return ""
	}
	return " at " + at(h.file, h.sp.Start)
}

// hides finds the earlier meaning of name that a new declaration would
// hide: a name in scope, a zero-hole phrase, a record type, an import
// qualifier or the package's own name.
func (b *body) hides(name string) hidden {
	for s := b.sc; s != nil; s = s.parent {
		if x := s.syms[name]; x != nil {
			return hidden{symKind(x), true, b.f, symSpan(x)}
		}
	}
	p := b.r.pkg
	if ph := b.zeroPhrase(name); ph != nil {
		return hidden{"phrase", true, ph.File, ph.Sp}
	}
	if rec := p.records[name]; rec != nil {
		return hidden{"record type", true, rec.File, rec.Sp}
	}
	if imp := b.f.byQual[name]; imp != nil {
		return hidden{"package", true, b.f, imp.Imp.LineSpan}
	}
	if name == p.Name && !p.IsMain {
		return hidden{kind: "package", found: true}
	}
	return hidden{}
}

// checkDecl runs the reserved-word and shadowing checks of a declaration.
// what is the kind of the new name.
func (b *body) checkDecl(name string, sp source.Span, what string) {
	if reserved[name] {
		b.r.err(b.f, sp, source.ReservedWord, `"`+name+`" is a reserved word and cannot be a name`, "")
		return
	}
	if h := b.hides(name); h.found {
		var more []source.Extra
		if h.file != nil {
			more = append(more, h.file.also(h.sp, `the `+h.kind+` "`+name+`" is declared here`))
		} else {
			more = append(more, source.Note(`"`+name+`" is the name of this package`))
		}
		b.r.err(b.f, sp, source.Shadowing, `"`+name+`" hides the `+h.kind+` "`+name+`"`+h.where(), "rename it", more...)
	}
}

func (b *body) markBelow(name string) {
	for s := b.sc; s != nil; s = s.parent {
		s.below[name] = true
	}
}

// declLocal declares a local, each name or draw name in the current scope.
func (b *body) declLocal(name string, span source.Span, kind LocalKind) *Local {
	what := "local"
	if kind == LocalEach {
		what = "each name"
	}
	pos := span.Start
	b.checkDecl(name, span, what)
	l := &Local{Name: name, Kind: kind, Pos: pos, Sp: span, eachIdx: -1, Used: reserved[name]}
	l.refs = append(l.refs, pos)
	b.sc.syms[name] = l
	b.sc.locals = append(b.sc.locals, l)
	b.locals = append(b.locals, l)
	b.markBelow(name)
	if Exported(name) {
		b.sc.pend = append(b.sc.pend, pendRename{l, name, pos, span})
	}
	return l
}

// declParam declares a hole or receiver of the phrase in the root scope.
func (b *body) declParam(prm *Param) {
	if _, dup := b.sc.syms[prm.Name].(*Param); !dup {
		b.checkDecl(prm.Name, prm.Sp, "hole")
	}
	b.sc.syms[prm.Name] = prm
	b.markBelow(prm.Name)
	prm.refs = append(prm.refs, prm.Pos)
	if Exported(prm.Name) {
		b.sc.pend = append(b.sc.pend, pendRename{prm, prm.Name, prm.Pos, prm.Sp})
	}
}

// declBinding declares a name bound by a when.
func (b *body) declBinding(bd *Binding) {
	if bd.Kind != BindRecord {
		b.checkDecl(bd.Name, bd.Sp, "binding")
	}
	b.sc.syms[bd.Name] = bd
	b.markBelow(bd.Name)
	bd.refs = append(bd.refs, bd.Pos)
	b.bindings = append(b.bindings, bd)
	if bd.Kind != BindRecord && Exported(bd.Name) {
		b.sc.pend = append(b.sc.pend, pendRename{bd, bd.Name, bd.Pos, bd.Sp})
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
		if b.hidesFileLevel(lower).found {
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
		b.r.err(b.f, pr.sp, source.NonCanonical,
			`"`+pr.name+`" must start lower-case: only exported names start upper-case`, "rename it",
			source.HelpEdit("rename it", lowerEdit(pr.pos, pr.name)),
			source.Note(`rename every use of "`+pr.name+`" the same way`))
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
func (b *body) hidesFileLevel(name string) hidden {
	save := b.sc
	b.sc = nil
	h := b.hides(name)
	b.sc = save
	return h
}
