// Package check type-checks resolved packages and lowers them to the IR.
//
// It owns the diagnostics that need types or contexts: type-mismatch,
// ambiguous-empty, missing-result, value-as-statement, action-as-value,
// context (the statement forms resolution cannot see), foreign-state, the
// type-dependent cases of bad-place and unexported, and the member lookups
// that only a type can settle ("n.Known" is a node field, a record field or
// a receiver phrase).
//
// # Type inference
//
// Package is flow-insensitive, as spec/04 section 4.6 requires: every local,
// node field, record field and value-phrase result is one place with one
// type, the join (types.Join) of its initial value and every assignment.
// Expression types are computed from the place types of the previous pass;
// the passes repeat until no place changes. "{}" has the type types.Empty
// until a join or an expected type decides it:
//
//   - the type expected where it appears (the place it is assigned to, the
//     hole it fills, the field of a record setting) decides it;
//   - otherwise how the place is used decides it ("set item k of x" with a
//     non-number key makes x a map; a hole of type "list of node" or "map of
//     value to value" makes the places passed to it lists or maps);
//   - a place that is still undecided is an ambiguous-empty error at each
//     "{}" that flows into it.
//
// A final pass over the settled types reports every diagnostic once.
//
// # Contexts
//
// Contexts walks the whole graph from the entry points (spec/05 section
// 5.4), memoizing (shared phrase, context) pairs, and reports context and
// foreign-state errors.
//
// # Lowering
//
// Lower turns packages without diagnostics into ir.Program values.
package check

import (
	"sort"
	"strconv"
	"strings"

	"github.com/synodlang/compiler/internal/resolve"
	"github.com/synodlang/compiler/internal/source"
	"github.com/synodlang/compiler/internal/types"
)

// StepKind tells what one part of a member read settled to.
type StepKind uint8

// The kinds of step.
const (
	StepPkg         StepKind = iota // a package qualifier: n.gossip
	StepNodeField                   // a field of a node: state
	StepRecordField                 // a field of a record value
	StepRecvPhrase                  // a receiver phrase with no holes: n.Version
)

// Step is one settled part of a Select.
type Step struct {
	Kind   StepKind
	Pkg    *resolve.Package // StepPkg
	Field  *resolve.Field   // StepNodeField, StepRecordField
	Phrase *resolve.Phrase  // StepRecvPhrase
	// Type is the type of the value after the step (nil for StepPkg).
	Type types.Type
	// Pos is the position of the part.
	Pos source.Pos
}

// SelInfo is how a Select settled.
type SelInfo struct {
	Steps []Step
	// Bad reports that the select did not settle (an error was reported).
	Bad bool
}

// Info is what the checker learned about one package. It is final for the
// packages that import it.
type Info struct {
	Pkg *resolve.Package
	// Uses holds the import paths this package runs: itself and everything
	// it imports, transitively.
	Uses map[string]bool
	// FieldTy holds the inferred type of every node field and record field.
	FieldTy map[*resolve.Field]types.Type
	// ResultTy holds the type of every value phrase written in Synod.
	ResultTy map[*resolve.Phrase]types.Type
	// LocalTy holds the type of every local, each name and draw name.
	LocalTy map[*resolve.Local]types.Type
	// Sel holds the settled member reads.
	Sel map[*resolve.Select]*SelInfo
}

// FieldType returns the type of f, a field of this package.
func (in *Info) FieldType(f *resolve.Field) types.Type { return in.FieldTy[f] }

type srcPos struct{ file, line, col int }

func (a srcPos) less(b srcPos) bool {
	if a.file != b.file {
		return a.file < b.file
	}
	if a.line != b.line {
		return a.line < b.line
	}
	return a.col < b.col
}

// src is one source of a place's type.
type src struct {
	pos  srcPos
	at   source.Pos
	file *resolve.File
	ty   types.Type
	text string
	// weak marks "set item k of p to v" with a number key: it is a list item
	// or a map entry, decided by the other sources and the uses of p.
	weak     bool
	key, val types.Type
	// item is set for "set item k of p to v": text is then "item k of p".
	item bool
}

type checker struct {
	rp    *resolve.Package
	info  *Info
	deps  map[string]*Info
	all   map[string]*Info
	pkgs  map[string]*resolve.Package
	diags []source.Diagnostic
	seen  map[string]bool

	final  bool
	file   *resolve.File
	phrase *resolve.Phrase // phrase whose body is walked, for "result"

	ty      map[any]types.Type
	forced  map[any]types.Type
	srcs    map[any][]src
	hi      map[any][]types.Type
	order   []any
	known   map[any]bool
	fileIdx map[*resolve.File]int
	names   map[any]string

	// marks are the member parts the final pass settled, for the tokens.
	marks []tokMark
	// cyc holds the sources that made a place contain itself.
	cyc map[srcPos]bool
}

// tokMark is the role of the token of one settled member part.
type tokMark struct {
	file *resolve.File
	pos  source.Pos
	role resolve.Role
	def  *resolve.Def
}

// Package type-checks rp. deps holds the Info of every package rp imports,
// directly or not, by import path (the checker runs dependencies first).
// The diagnostics are not sorted.
func Package(rp *resolve.Package, deps map[string]*Info) (*Info, []source.Diagnostic) {
	info := &Info{
		Pkg:      rp,
		Uses:     map[string]bool{rp.Path: true},
		FieldTy:  map[*resolve.Field]types.Type{},
		ResultTy: map[*resolve.Phrase]types.Type{},
		LocalTy:  map[*resolve.Local]types.Type{},
		Sel:      map[*resolve.Select]*SelInfo{},
	}
	c := &checker{
		rp: rp, info: info, deps: deps,
		all: map[string]*Info{}, pkgs: map[string]*resolve.Package{},
		seen:   map[string]bool{},
		ty:     map[any]types.Type{},
		forced: map[any]types.Type{},
		hi:     map[any][]types.Type{},
		known:  map[any]bool{},
		names:  map[any]string{},
		cyc:    map[srcPos]bool{},
	}
	for p, d := range deps {
		c.all[p] = d
		c.pkgs[p] = d.Pkg
	}
	c.all[rp.Path] = info
	c.pkgs[rp.Path] = rp
	for _, imp := range rp.Imports {
		if imp.Pkg == nil {
			continue
		}
		if d := deps[imp.Pkg.Path]; d != nil {
			for u := range d.Uses {
				info.Uses[u] = true
			}
		}
		info.Uses[imp.Pkg.Path] = true
	}
	c.fileIdx = map[*resolve.File]int{}
	for i, f := range rp.Files {
		c.fileIdx[f] = i
	}
	c.solve()
	c.finish()
	c.applyMarks()
	return info, c.diags
}

// runs is the relation of types.Runs.
func (c *checker) runs(p, q string) bool {
	if p == q {
		return true
	}
	if in := c.all[p]; in != nil {
		return in.Uses[q]
	}
	return false
}

func (c *checker) accepts(s, t types.Type) bool { return types.Accepts(s, t, c.runs) }

func (c *checker) join(a, b types.Type) (types.Type, bool) { return types.Join(a, b, c.runs) }

// ---- diagnostics ----

func (c *checker) errf(pos source.Pos, code source.Code, msg, hint string) {
	if !c.final || c.file == nil {
		return
	}
	key := c.file.Path + ":" + strconv.Itoa(pos.Line) + ":" + strconv.Itoa(pos.Col) + ":" + string(code)
	if c.seen[key] {
		return
	}
	c.seen[key] = true
	c.diags = append(c.diags, source.New(c.file.Load.Src, pos, code, msg, hint))
}

// fmtType prints t as the current file writes it.
func (c *checker) fmtType(t types.Type) string {
	if t == nil {
		return "nothing"
	}
	return types.Format(t, c.rp.Path, c.qualOf)
}

func (c *checker) qualOf(path string) string {
	if c.file != nil {
		for _, imp := range c.file.Imports {
			if imp.Pkg != nil && imp.Pkg.Path == path {
				return imp.Qual
			}
		}
	}
	if p := c.pkgs[path]; p != nil {
		return p.Name
	}
	return ""
}

// ---- the solver ----

// place keys: *resolve.Local, *resolve.Field, *resolve.Phrase (its result).

func (c *checker) note(p any, name string) {
	if !c.known[p] {
		c.known[p] = true
		c.order = append(c.order, p)
		c.names[p] = name
	}
}

func (c *checker) pos(f *resolve.File, p source.Pos) srcPos {
	return srcPos{c.fileIdx[f], p.Line, p.Col}
}

// addSrc adds a source to the type of place p.
func (c *checker) addSrc(p any, name string, at source.Pos, ty types.Type, text string) {
	c.note(p, name)
	c.srcs[p] = append(c.srcs[p], src{pos: c.pos(c.file, at), at: at, file: c.file, ty: ty, text: text})
}

func (c *checker) addWeak(p any, name string, at source.Pos, k, v types.Type, text string) {
	c.note(p, name)
	c.srcs[p] = append(c.srcs[p], src{pos: c.pos(c.file, at), at: at, file: c.file, weak: true, key: k, val: v, text: text, item: true})
}

// addItem adds the map that "set item k of p to v" makes of p.
func (c *checker) addItem(p any, name string, at source.Pos, t types.Type, text string) {
	c.note(p, name)
	c.srcs[p] = append(c.srcs[p], src{pos: c.pos(c.file, at), at: at, file: c.file, ty: t, text: text, item: true})
}

func (c *checker) addHi(p any, t types.Type) {
	for _, h := range c.hi[p] {
		if types.Equal(h, t) {
			return
		}
	}
	c.hi[p] = append(c.hi[p], t)
}

func isCollection(t types.Type) bool {
	switch t.(type) {
	case types.List, types.Map:
		return true
	}
	return false
}

// hasEmpty reports an undecided "{}" inside t.
func hasEmpty(t types.Type) bool {
	switch x := t.(type) {
	case types.Empty:
		return true
	case types.List:
		return hasEmpty(x.Elem)
	case types.Map:
		return hasEmpty(x.Key) || hasEmpty(x.Val)
	}
	return false
}

// maxDepth bounds how deeply lists and maps nest in an inferred type; a place
// that grows past it contains itself ("set l to {l}").
const maxDepth = 8

func depth(t types.Type) int {
	switch x := t.(type) {
	case types.List:
		return 1 + depth(x.Elem)
	case types.Map:
		return 1 + max(depth(x.Key), depth(x.Val))
	}
	return 0
}

// hasInvalid reports the type of an expression that failed, or has not been
// typed yet, anywhere in t. Such a source tells nothing about its place.
func hasInvalid(t types.Type) bool {
	switch x := t.(type) {
	case types.Basic:
		return x.K == types.Invalid
	case types.List:
		return hasInvalid(x.Elem)
	case types.Map:
		return hasInvalid(x.Key) || hasInvalid(x.Val)
	}
	return false
}

// hasValue reports the native type "value" inside t.
func hasValue(t types.Type) bool {
	switch x := t.(type) {
	case types.Basic:
		return x.K == types.Value
	case types.List:
		return hasValue(x.Elem)
	case types.Map:
		return hasValue(x.Key) || hasValue(x.Val)
	}
	return false
}

func isInvalid(t types.Type) bool {
	b, ok := t.(types.Basic)
	return t == nil || ok && b.K == types.Invalid
}

// shapeOf decides whether weak sources of p are list items or map entries.
func (c *checker) shapeOf(p any, ss []src, forced types.Type) (isMap bool) {
	look := func(t types.Type) (found, m bool) {
		switch t.(type) {
		case types.List:
			return true, false
		case types.Map:
			return true, true
		}
		return false, false
	}
	if forced != nil {
		if f, m := look(forced); f {
			return m
		}
	}
	for _, s := range ss {
		if s.weak {
			continue
		}
		if f, m := look(s.ty); f {
			return m
		}
	}
	for _, h := range c.hi[p] {
		if f, m := look(h); f {
			return m
		}
	}
	return false
}

// fold computes the type of place p from its sources, in source order.
func (c *checker) fold(p any) types.Type {
	ss := append([]src(nil), c.srcs[p]...)
	sort.SliceStable(ss, func(i, j int) bool { return ss[i].pos.less(ss[j].pos) })
	acc := c.forced[p]
	isMap := c.shapeOf(p, ss, acc)
	for _, s := range ss {
		t := s.ty
		if s.weak {
			if isMap {
				t = types.Map{Key: s.key, Val: s.val}
			} else {
				t = types.List{Elem: s.val}
			}
		}
		if t == nil || hasInvalid(t) {
			continue
		}
		if depth(t) > maxDepth {
			c.cyc[s.pos] = true
		}
		if c.cyc[s.pos] {
			if c.final {
				save := c.file
				c.file = s.file
				c.errf(s.at, source.TypeMismatch, `"`+s.text+`" cannot contain itself`, "")
				c.file = save
			}
			continue
		}
		if acc == nil {
			acc = t
			continue
		}
		j, ok := c.join(acc, t)
		if !ok {
			if c.final {
				c.joinErr(p, s, acc, t)
			}
			continue
		}
		acc = j
	}
	return acc
}

func (c *checker) joinErr(p any, s src, have, got types.Type) {
	save := c.file
	c.file = s.file
	defer func() { c.file = save }()
	if ph, ok := p.(*resolve.Phrase); ok {
		c.errf(s.at, source.TypeMismatch,
			`results of "`+ph.Describe("")+`" have types `+c.fmtType(have)+` and `+c.fmtType(got), "")
		return
	}
	text := s.text
	if s.weak {
		got = types.List{Elem: s.val}
		if isMapType(have) {
			got = types.Map{Key: s.key, Val: s.val}
		}
	}
	if s.item {
		c.errf(s.at, source.TypeMismatch,
			`"`+text+`" makes "`+c.names[p]+`" a `+c.fmtType(got)+`, but it has type `+c.fmtType(have), "")
		return
	}
	c.errf(s.at, source.TypeMismatch,
		`"`+text+`" has type `+c.fmtType(got)+`, but "`+c.names[p]+`" has type `+c.fmtType(have), "")
}

func isMapType(t types.Type) bool { _, ok := t.(types.Map); return ok }

// commit folds every place and reports whether any type changed.
func (c *checker) commit() bool {
	changed := false
	for _, p := range c.order {
		t := c.fold(p)
		if !types.Equal(c.ty[p], t) {
			c.ty[p] = t
			changed = true
		}
	}
	return changed
}

// applyHi decides the places that are still undecided from how they are
// used. It reports whether it decided any.
func (c *checker) applyHi() bool {
	changed := false
	for _, p := range c.order {
		t := c.ty[p]
		if t == nil || !hasEmpty(t) {
			continue
		}
		var cands []types.Type
		for _, h := range c.hi[p] {
			if !hasValue(h) && c.accepts(t, h) {
				cands = append(cands, h)
			}
		}
		if len(cands) == 0 && isEmpty(t) {
			// Only the shape is known from the uses: list or map, items unknown.
			for _, h := range c.hi[p] {
				var shaped types.Type
				switch h.(type) {
				case types.List:
					shaped = types.List{Elem: types.Empty{}}
				case types.Map:
					shaped = types.Map{Key: types.Empty{}, Val: types.Empty{}}
				}
				if shaped != nil {
					if !types.Equal(c.forced[p], shaped) {
						c.forced[p] = shaped
						changed = true
					}
					break
				}
			}
		}
		for _, a := range cands {
			best := true
			for _, b := range cands {
				if !c.accepts(a, b) {
					best = false
					break
				}
			}
			if best {
				if !types.Equal(c.forced[p], a) {
					c.forced[p] = a
					changed = true
				}
				break
			}
		}
	}
	return changed
}

func (c *checker) solve() {
	for iter := 0; iter < 60; iter++ {
		c.pass(false)
		if c.commit() {
			continue
		}
		if !c.applyHi() {
			break
		}
	}
	c.pass(true)
}

// finish records the settled types in Info.
func (c *checker) finish() {
	for _, p := range c.order {
		t := c.ty[p]
		if t == nil {
			t = types.TInvalid
		}
		switch x := p.(type) {
		case *resolve.Field:
			c.info.FieldTy[x] = t
		case *resolve.Phrase:
			c.info.ResultTy[x] = t
		case *resolve.Local:
			c.info.LocalTy[x] = t
		}
	}
}

// placeType is the settled type of place p, or Invalid before it has one.
func (c *checker) placeType(p any) types.Type {
	if t := c.ty[p]; t != nil {
		return t
	}
	return types.TInvalid
}

// fieldType is the type of a node field or record field of any package.
func (c *checker) fieldType(f *resolve.Field) types.Type {
	if f.Owner.Pkg == c.rp {
		return c.placeType(f)
	}
	if in := c.deps[f.Owner.Pkg.Path]; in != nil {
		if t := in.FieldTy[f]; t != nil {
			return t
		}
	}
	return types.TInvalid
}

// resultType is the type a value phrase gives, nil for an action.
func (c *checker) resultType(ph *resolve.Phrase) types.Type {
	if !ph.IsValue {
		return nil
	}
	if ph.Sig != nil {
		return nil // handled by the native rules
	}
	if ph.Pkg == c.rp {
		return c.placeType(ph)
	}
	if in := c.deps[ph.Pkg.Path]; in != nil {
		if t := in.ResultTy[ph]; t != nil {
			return t
		}
	}
	return types.TInvalid
}

// closeName returns the candidate nearest to name, or "".
func closeName(name string, cands []string) string {
	best, bd := "", 0.0
	for _, cand := range cands {
		if cand == name {
			continue
		}
		d := lev(name, cand)
		if d <= 0.4 && (best == "" || d < bd || d == bd && cand < best) {
			best, bd = cand, d
		}
	}
	return best
}

func lev(a, b string) float64 {
	a, b = strings.ToLower(a), strings.ToLower(b)
	ra, rb := []rune(a), []rune(b)
	prev := make([]int, len(rb)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(ra); i++ {
		cur := make([]int, len(rb)+1)
		cur[0] = i
		for j := 1; j <= len(rb); j++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}
		prev = cur
	}
	m := max(len(ra), len(rb))
	if m == 0 {
		return 0
	}
	return float64(prev[len(rb)]) / float64(m)
}

// mark records the role of the token at pos of the current file. Defs in std
// are left out, as the resolver does.
func (c *checker) mark(pos source.Pos, role resolve.Role, defFile *resolve.File, defPos source.Pos) {
	if !c.final || c.file == nil {
		return
	}
	var def *resolve.Def
	if defFile != nil && !strings.HasPrefix(defFile.Path, "std/") {
		def = &resolve.Def{File: defFile.Path, Pos: defPos}
	}
	c.marks = append(c.marks, tokMark{c.file, pos, role, def})
}

// applyMarks patches the resolver's tokens: a member part is a field, a
// record field or a receiver phrase only once its base type is known.
func (c *checker) applyMarks() {
	idx := map[*resolve.File]map[source.Pos]int{}
	for _, m := range c.marks {
		ix := idx[m.file]
		if ix == nil {
			ix = map[source.Pos]int{}
			for i, t := range c.rp.Tokens[m.file.Load] {
				ix[source.Pos{Line: t.Line, Col: t.Col}] = i
			}
			idx[m.file] = ix
		}
		if i, ok := ix[m.pos]; ok {
			t := &c.rp.Tokens[m.file.Load][i]
			t.Role, t.Def = m.role, m.def
		}
	}
}
