package resolve

import (
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/synodlang/compiler/internal/format"
	"github.com/synodlang/compiler/internal/load"
	"github.com/synodlang/compiler/internal/parse"
	"github.com/synodlang/compiler/internal/source"
	"github.com/synodlang/compiler/internal/stdlib"
	"github.com/synodlang/compiler/internal/types"
)

// decl is what a name in the package namespace stands for.
type decl struct {
	what string // "phrase", "record type", "node field", "node type"
	file *File
	pos  source.Pos
	sp   source.Span
}

// claim is a name claimed in the package namespace; claims are checked in
// source order, so that the second definition is the one reported.
type claim struct {
	key  string // phrase skeleton, record name, field name
	disp string // how to print it
	what string
	fi   int
	file *File
	pos  source.Pos
	sp   source.Span
	// reject is called on a duplicate.
	reject func()
	// builtin marks a record type named like a built-in type.
	special string
}

type resolver struct {
	pkg   *Package
	lp    *load.Package
	deps  map[string]*Package
	diags []source.Diagnostic
	seen  map[string]bool

	claims []claim
	// fixes by file.
	fixes map[*load.File][]format.Fix
	// marks classify tokens for --tokens.
	marks map[*load.File]map[source.Pos]mark

	std      map[string]bool // std package names, for "add use std/x" hints
	hasLines []hasInfo
	byLine   map[*parse.Line]*Phrase
	nodeHas  *decl
	startAt  *decl
	mainAt   *decl
}

// Resolve resolves lp. deps holds the resolved packages that lp imports,
// by import path; a package that is broken is absent, and references to it
// resolve silently. lp must not be Broken. The diagnostics are not sorted.
func Resolve(lp *load.Package, deps map[string]*Package) (*Package, []source.Diagnostic) {
	r := &resolver{
		lp:     lp,
		deps:   deps,
		seen:   map[string]bool{},
		fixes:  map[*load.File][]format.Fix{},
		marks:  map[*load.File]map[source.Pos]mark{},
		byLine: map[*parse.Line]*Phrase{},
	}
	p := &Package{
		Load: lp, Path: lp.Path, Name: lp.Name, IsMain: lp.IsMain, IsStd: lp.IsStd,
		Fixes:   r.fixes,
		Tokens:  map[*load.File][]Token{},
		heads:   map[string][]*Phrase{},
		records: map[string]*Record{},
		fields:  map[string]*Field{},
		names:   map[string]*decl{},
	}
	r.pkg = p
	if !p.IsMain {
		p.Node = &Record{Pkg: p, Name: "Node", Exported: true, IsNode: true}
	}
	for _, lf := range lp.Files {
		if lf.Tree == nil {
			return p, nil
		}
		p.Files = append(p.Files, &File{Load: lf, Path: lf.Src.Path, byQual: map[string]*Import{}})
	}
	r.imports()
	// Pass 1: record types and node fields; pass 2: phrases; claims; pass 3:
	// bodies.
	for _, f := range p.Files {
		r.declRoot(f, 1)
	}
	for _, f := range p.Files {
		r.declRoot(f, 2)
	}
	r.checkClaims()
	r.checkQualifierClashes()
	for _, f := range p.Files {
		r.declRoot(f, 3)
	}
	r.mains()
	r.finishImports()
	r.tokens()
	for lf, fx := range r.fixes {
		r.fixes[lf] = dedupFixes(fx)
	}
	return p, r.diags
}

func dedupFixes(fx []format.Fix) []format.Fix {
	sort.SliceStable(fx, func(i, j int) bool {
		a, b := fx[i].Pos, fx[j].Pos
		if a.Line != b.Line {
			return a.Line < b.Line
		}
		return a.Col < b.Col
	})
	out := fx[:0]
	for i, f := range fx {
		if i > 0 && f.Pos == out[len(out)-1].Pos {
			continue
		}
		out = append(out, f)
	}
	return out
}

// err records a diagnostic, once per position and code. The primary span is
// sp; more adds the label, secondary labels, help and notes.
func (r *resolver) err(f *File, sp source.Span, code source.Code, msg, hint string, more ...source.Extra) {
	pos := sp.Start
	key := f.Path + ":" + strconv.Itoa(pos.Line) + ":" + strconv.Itoa(pos.Col) + ":" + string(code)
	if r.seen[key] {
		return
	}
	r.seen[key] = true
	r.diags = append(r.diags, source.NewSpan(f.Load.Src, sp, code, msg, hint, more...))
}

func (r *resolver) addFix(f *File, fx format.Fix) {
	r.fixes[f.Load] = append(r.fixes[f.Load], fx)
}

// at prints "<file>:<line>" for a declaration.
func at(f *File, pos source.Pos) string {
	return f.Path + ":" + strconv.Itoa(pos.Line)
}

// ---- imports ----

func lastSegment(p string) string {
	if i := strings.LastIndexByte(p, '/'); i >= 0 {
		return p[i+1:]
	}
	return p
}

func (r *resolver) imports() {
	p := r.pkg
	byFile := map[*load.File]*File{}
	for _, f := range p.Files {
		byFile[f.Load] = f
	}
	for _, li := range r.lp.Imports {
		f := byFile[li.File]
		if f == nil {
			continue
		}
		imp := &Import{File: f, Imp: li, Qual: li.Alias, Pos: li.PathPos, Sp: li.PathSpan}
		if imp.Qual == "" {
			imp.Qual = lastSegment(li.Path)
		}
		if li.Pkg != nil {
			imp.Pkg = r.deps[li.Pkg.Path]
		}
		f.Imports = append(f.Imports, imp)
		p.Imports = append(p.Imports, imp)
	}
	for _, f := range p.Files {
		paths := map[string]*Import{}
		for _, imp := range f.Imports {
			li := imp.Imp
			if prev := paths[li.Path]; prev != nil {
				r.err(f, imp.Sp, source.DuplicateName, `"`+li.Path+`" is already imported`, "",
					f.also(prev.Sp, "first imported here"),
					source.HelpEdit("remove this line", deleteLine(li.LineSpan.Start.Line)))
				imp.Used = true // one error is enough
			} else {
				paths[li.Path] = imp
			}
			if li.Alias != "" {
				switch {
				case reserved[li.Alias]:
					r.err(f, aliasWord(li), source.ReservedWord, `"`+li.Alias+`" is a reserved word and cannot be a package name`, "")
				}
			}
			if prev := f.byQual[imp.Qual]; prev != nil && prev.Imp.Path != li.Path {
				r.err(f, li.PathSpan, source.DuplicateName,
					`qualifier "`+imp.Qual+`" is already used by "`+prev.Imp.Path+`"`,
					`rename one with "use `+li.Path+` as <name>"`,
					f.also(prev.Sp, `"`+imp.Qual+`" is first used here`))
				imp.Used = true // one error is enough
				continue
			} else if prev != nil {
				continue
			}
			if imp.Qual == p.Name && !p.IsMain && li.Alias == "" {
				r.err(f, li.PathSpan, source.DuplicateName,
					`qualifier "`+imp.Qual+`" is already the name of this package`,
					`rename it with "use `+li.Path+` as <name>"`)
			}
			f.byQual[imp.Qual] = imp
		}
		// unneeded-alias: an alias is for a collision of last segments.
		for _, imp := range f.Imports {
			li := imp.Imp
			if li.Alias == "" {
				continue
			}
			seg := lastSegment(li.Path)
			clash := seg == p.Name && !p.IsMain
			for _, o := range f.Imports {
				if o != imp && lastSegment(o.Imp.Path) == seg && o.Imp.Path != li.Path {
					clash = true
				}
			}
			if !clash && !reserved[li.Alias] {
				r.err(f, aliasWord(li), source.UnneededAlias,
					`"as `+li.Alias+`" is only for imports that share a last segment`,
					`write "use `+li.Path+`"`,
					source.HelpEdit(`write "use `+li.Path+`"`,
						source.EditAt(source.Span{Start: li.PathSpan.End, End: li.AliasSpan.End}, "")))
			}
		}
	}
}

// aliasWord is the span of the alias itself, without the word "as".
func aliasWord(li *load.Import) source.Span {
	return source.Span{Start: li.AliasPos, End: li.AliasSpan.End}
}

// finishImports reports unused imports and upper-case aliases.
func (r *resolver) finishImports() {
	for _, imp := range r.pkg.Imports {
		if a := imp.Imp.Alias; a != "" && Exported(a) {
			r.renameAlias(imp)
		}
		if imp.Pkg == nil || imp.Used {
			continue
		}
		if imp.Pkg.Observers {
			continue
		}
		r.err(imp.File, imp.Sp, source.UnusedImport, `"`+imp.Imp.Path+`" is imported but not used`, "",
			source.HelpEdit("remove the line", deleteLine(imp.Imp.LineSpan.Start.Line)))
	}
}

// ---- file root ----

var builtinTypes = map[string]bool{
	"text": true, "number": true, "bool": true, "time": true, "node": true, "msg": true,
	"shape": true, "color": true, "field": true, "package": true, "block": true, "value": true,
}

// declRoot walks the top-level lines of f for one pass.
func (r *resolver) declRoot(f *File, pass int) {
	for _, n := range f.Load.Tree.Body {
		l, ok := n.(*parse.Line)
		if !ok {
			continue
		}
		g := l.Sentence.Groups[0]
		first, _ := g[0].(*parse.Word)
		switch {
		case first != nil && first.Name == "use":
		case first != nil && first.Name == "define":
			switch pass {
			case 2:
				r.declDefine(f, l)
			case 3:
				r.bodyDefine(f, l)
			}
		case first != nil && first.Name == "when":
			if pass == 3 {
				r.bodyWhen(f, l)
			}
		case first != nil && first.Name == "always":
			if pass == 3 {
				r.bodyAlways(f, l)
			}
		case isHas(g) && l.Block != nil:
			switch pass {
			case 1:
				r.declHas(f, l)
			case 3:
				r.bodyHas(f, l)
			}
		default:
			if pass == 1 {
				r.err(f, parse.LineSpan(l), source.Context, "only declarations stand at file root",
					"a file holds use, has, define, when and always")
			}
		}
	}
}

func isHas(g []parse.Item) bool {
	if len(g) != 2 {
		return false
	}
	if _, ok := g[0].(*parse.Word); !ok {
		return false
	}
	return parse.IsWord(g[1], "has")
}

// ---- has ----

// hasInfo remembers the record of a has line between passes.
type hasInfo struct {
	rec  *Record
	line *parse.Line
}

func (r *resolver) declHas(f *File, l *parse.Line) {
	g := l.Sentence.Groups[0]
	w := g[0].(*parse.Word)
	hasSp := parse.SpanOfItems(g)
	var rec *Record
	switch {
	case reserved[w.Name]:
		r.err(f, sp(w), source.ReservedWord, `"`+w.Name+`" is a reserved word and cannot be a record type`, "")
		return
	case w.Name == "Node":
		if r.pkg.IsMain {
			r.err(f, hasSp, source.BadReceiver, "a main file has no Node has", "")
			return
		}
		if r.nodeHas != nil {
			r.err(f, hasSp, source.DuplicateName,
				`a second "Node has"; the first is at `+at(r.nodeHas.file, r.nodeHas.pos), "",
				r.nodeHas.file.also(r.nodeHas.sp, `first "Node has" here`))
			return
		}
		r.nodeHas = &decl{"node type", f, w.P, hasSp}
		rec = r.pkg.Node
		rec.File, rec.Pos, rec.Sp = f, w.P, hasSp
	default:
		rec = &Record{Pkg: r.pkg, File: f, Name: w.Name, Exported: Exported(w.Name), Pos: w.P, Sp: hasSp}
		if builtinTypes[w.Name] {
			r.err(f, sp(w), source.DuplicateName, `"`+w.Name+`" is a built-in type`, "",
				source.Note(builtinTypesNote))
			return
		}
		fi := r.fileIndex(f)
		r.claims = append(r.claims, claim{key: w.Name, disp: w.Name, what: "record type", fi: fi, file: f, pos: w.P, sp: hasSp,
			reject: func() { rec.Name = "" }})
		r.pkg.Records = append(r.pkg.Records, rec)
		r.pkg.records[w.Name] = rec
	}
	if rec.File != nil {
		r.mark(f, w.P, RoleType, &Def{File: rec.File.Path, Pos: rec.Pos})
	}
	// fields
	for _, bl := range l.Block.Lines {
		fg := bl.Sentence.Groups[0]
		if len(fg) != 4 || !parse.IsWord(fg[0], "set") || !parse.IsWord(fg[2], "to") || bl.Block != nil {
			r.err(f, parse.LineSpan(bl), source.Context, `a has block holds only "set <field> to <value>" lines`, "")
			continue
		}
		fw, ok := fg[1].(*parse.Word)
		if !ok {
			r.err(f, sp(fg[1]), source.BadPlace, `"`+printItem(fg[1])+`" is not a `+fieldWhat(rec)+` name`, "")
			continue
		}
		if reserved[fw.Name] {
			r.err(f, sp(fw), source.ReservedWord, `"`+fw.Name+`" is a reserved word and cannot be a field`, "")
			continue
		}
		if first := rec.Lookup(fw.Name); first != nil {
			r.err(f, sp(fw), source.DuplicateName, `"`+fw.Name+`" is already defined at `+at(first.File, first.Pos), "",
				first.File.also(first.Sp, "first defined here"))
			continue
		}
		fl := &Field{Owner: rec, Name: fw.Name, Exported: Exported(fw.Name), Index: len(rec.Fields), Pos: fw.P, Sp: sp(fw), File: f}
		rec.Fields = append(rec.Fields, fl)
		r.mark(f, fw.P, RoleField, &Def{File: f.Path, Pos: fw.P})
		if rec.IsNode {
			fi := r.fileIndex(f)
			r.claims = append(r.claims, claim{key: fw.Name, disp: fw.Name, what: "node field", fi: fi, file: f, pos: fw.P, sp: sp(fw),
				reject: func() { r.pkg.fields[fl.Name] = nil; delete(r.pkg.fields, fl.Name) }})
			r.pkg.fields[fw.Name] = fl
		}
	}
	r.hasLines = append(r.hasLines, hasInfo{rec, l})
}

func fieldWhat(rec *Record) string {
	if rec.IsNode {
		return "node field"
	}
	return "record field"
}

func (r *resolver) fileIndex(f *File) int {
	for i, x := range r.pkg.Files {
		if x == f {
			return i
		}
	}
	return 0
}

// checkClaims reports duplicate names in the package namespace, the second
// definition (files in name order, then line) being the one reported.
func (r *resolver) checkClaims() {
	cs := r.claims
	sort.SliceStable(cs, func(i, j int) bool {
		a, b := cs[i], cs[j]
		if a.fi != b.fi {
			return a.fi < b.fi
		}
		if a.pos.Line != b.pos.Line {
			return a.pos.Line < b.pos.Line
		}
		return a.pos.Col < b.pos.Col
	})
	first := map[string]claim{}
	for _, c := range cs {
		if c.key == "Node" && !r.pkg.IsMain {
			r.err(c.file, c.sp, source.DuplicateName, `"Node" is the name of the node type of every package`, "")
			c.reject()
			continue
		}
		if prev, ok := first[c.key]; ok {
			r.err(c.file, c.sp, source.DuplicateName, `"`+c.disp+`" is already defined at `+at(prev.file, prev.pos), "",
				prev.file.also(prev.sp, "first defined here"))
			c.reject()
			continue
		}
		first[c.key] = c
	}
	// Fill the lookup tables from the survivors, in order.
	p := r.pkg
	var recs []*Record
	for _, rec := range p.Records {
		if rec.Name != "" {
			recs = append(recs, rec)
		}
	}
	p.Records = recs
	var phs []*Phrase
	for _, ph := range p.Phrases {
		if !ph.dup {
			phs = append(phs, ph)
			if ph.Kind == PhraseNormal && ph.Head != "" {
				p.heads[ph.Head] = append(p.heads[ph.Head], ph)
			}
		}
	}
	p.Phrases = phs
}

// checkQualifierClashes reports a node field or phrase head spelled like an
// import qualifier of a file (decision: shadowing, spec/03 section 3.3).
func (r *resolver) checkQualifierClashes() {
	p := r.pkg
	for _, f := range p.Files {
		for _, q := range sortedKeys(f.byQual) {
			imp := f.byQual[q]
			if fl := p.fields[q]; fl != nil {
				r.err(fl.File, fl.Sp, source.Shadowing,
					`"`+q+`" hides the package "`+q+`" at `+at(f, imp.Pos), "rename it",
					f.also(imp.Imp.LineSpan, `the package "`+q+`" is declared here`))
			}
			for _, ph := range p.heads[q] {
				r.err(ph.File, ph.Items[0].Sp, source.Shadowing,
					`"`+q+`" hides the package "`+q+`" at `+at(f, imp.Pos), "rename it",
					f.also(imp.Imp.LineSpan, `the package "`+q+`" is declared here`))
			}
		}
	}
}

// mains reports missing and extra "define main".
func (r *resolver) mains() {
	p := r.pkg
	if p.IsMain && p.Main == nil && len(p.Files) > 0 {
		f := p.Files[0]
		span := source.Span{Start: source.Pos{Line: 1, Col: 1}}
		span.End = source.Pos{Line: 1, Col: 1 + utf8.RuneCountInString(f.Load.Src.Line(1))}
		if h := f.Load.Tree.Header; h != nil {
			span = parse.CommentSpan(h)
		}
		r.err(f, span, source.MissingMain, `main file has no "define main"`, "",
			source.HelpText(`add "define main" with an indented block`))
	}
}

// reserved is the set of reserved words (spec/01 section 1.10).
var reserved = func() map[string]bool {
	m := map[string]bool{}
	for _, w := range strings.Fields(`use as define native main start has set to when after deliver emit
if else each in result always draw for message with of item list map
is not at least most less more than and or plus minus times divided by
true false`) {
		m[w] = true
	}
	return m
}()

// ---- define ----

func (r *resolver) declDefine(f *File, l *parse.Line) {
	g := l.Sentence.Groups[0]
	items := g[1:]
	defPos := g[0].Pos()
	p := r.pkg
	if len(items) == 1 && parse.IsWord(items[0], "main") {
		r.declMain(f, l, g)
		return
	}
	var native *parse.Text
	var nativeWord *parse.Word
	if n := len(items); n >= 2 && parse.IsWord(items[n-2], "native") {
		if t, ok := items[n-1].(*parse.Text); ok {
			native, nativeWord = t, items[n-2].(*parse.Word)
			items = items[:n-2]
		}
	}
	if len(items) == 0 {
		r.err(f, sp(g[0]), source.BadPattern, "a pattern needs a word", "")
		return
	}
	defSp := source.Span{Start: defPos, End: items[len(items)-1].End()}
	ph := &Phrase{Pkg: p, File: f, Pos: defPos, Sp: defSp, Line: l}
	// The entry "define [n: Node] start".
	if isStart(items) {
		r.declStart(f, l, ph, items)
		return
	}
	idx := 0
	if h, ok := items[0].(*parse.Hole); ok {
		ph.Receiver = r.receiver(f, h)
		idx = 1
	}
	if idx >= len(items) {
		r.err(f, sp(items[0]), source.BadPattern, "a pattern needs a word", "")
		return
	}
	head, ok := items[idx].(*parse.Word)
	if !ok {
		if _, isHole := items[idx].(*parse.Hole); isHole {
			r.err(f, sp(items[idx]), source.BadPattern, "only the receiver stands before the head", "")
		} else {
			r.err(f, sp(items[idx]), source.BadPattern, "pattern items are words and holes", "")
		}
		return
	}
	if reserved[head.Name] {
		r.err(f, sp(head), source.ReservedWord, `"`+head.Name+`" is a reserved word and cannot be a phrase head`, "")
	}
	if !reserved[head.Name] {
		r.mark(f, head.P, RolePhrase, &Def{File: f.Path, Pos: head.P})
	}
	ph.Head = head.Name
	ph.Exported = Exported(head.Name)
	ph.Items = append(ph.Items, PatItem{Word: head.Name, Pos: head.P, Sp: sp(head)})
	// The head is never directly followed by a comparison operator.
	if idx+1 < len(items) {
		if w, ok := items[idx+1].(*parse.Word); ok {
			var op string
			opEnd := w.End()
			switch w.Name {
			case "is":
				op = "is"
			case "less", "more":
				if idx+2 < len(items) && parse.IsWord(items[idx+2], "than") {
					op = w.Name + " than"
					opEnd = items[idx+2].End()
				}
			case "at":
				if idx+2 < len(items) && (parse.IsWord(items[idx+2], "least") || parse.IsWord(items[idx+2], "most")) {
					op = "at " + items[idx+2].(*parse.Word).Name
					opEnd = items[idx+2].End()
				}
			}
			if op != "" {
				r.err(f, source.Span{Start: w.P, End: opEnd}, source.BadPattern,
					`"`+head.Name+" "+op+`" reads as a comparison; a head is never directly followed by "`+op+`"`, "",
					f.also(sp(head), "this is the head"))
			}
		}
	}
	// Words and holes after the head.
	prevHole := (*parse.Hole)(nil)
	sawBlock := false
	pendingWord := ""
	inline := 0
	for i := idx + 1; i < len(items); i++ {
		switch it := items[i].(type) {
		case *parse.Word:
			r.mark(f, it.P, RolePhrase, nil)
			if sawBlock {
				if pendingWord != "" {
					r.err(f, sp(it), source.BadPattern, `after a block hole only "<word> [b: block]" pairs follow`, "")
					return
				}
				pendingWord = it.Name
				if Exported(it.Name) {
					r.err(f, sp(it), source.BadPattern, "pattern words after the head start lower-case", "")
				}
				continue
			}
			if Exported(it.Name) {
				r.err(f, sp(it), source.BadPattern, "pattern words after the head start lower-case", "")
			}
			ph.Items = append(ph.Items, PatItem{Word: it.Name, Pos: it.P, Sp: sp(it)})
			prevHole = nil
		case *parse.Hole:
			param := r.param(f, it, ph.Pkg)
			if param.Kind == ParamBlock {
				if len(ph.Items) == 1 && !sawBlock && len(ph.Blocks) == 0 {
					r.err(f, sp(it), source.BadPattern, "a word or hole must stand between the head and the first block hole", "")
				}
				if sawBlock && pendingWord == "" {
					r.err(f, sp(it), source.BadPattern, `after a block hole only "<word> [b: block]" pairs follow`, "")
				}
				ph.Blocks = append(ph.Blocks, BlockSlot{Word: pendingWord, Param: param})
				pendingWord = ""
				sawBlock = true
				prevHole = nil
				continue
			}
			if sawBlock {
				var more []source.Extra
				if n := len(ph.Blocks); n > 0 {
					more = append(more, f.also(ph.Blocks[n-1].Param.HoleSp, "the last block hole is here"))
				}
				r.err(f, sp(it), source.BadPattern, "block holes come last", "", more...)
				return
			}
			if prevHole != nil {
				r.err(f, sp(it), source.BadPattern,
					`holes "`+holeText(prevHole)+`" and "`+holeText(it)+`" are adjacent`, "",
					f.also(sp(prevHole), "adjacent to this hole"),
					source.HelpText("put a word between them"))
			}
			param.Index = inline
			inline++
			ph.Params = append(ph.Params, param)
			ph.Items = append(ph.Items, PatItem{Param: param, Pos: it.P, Sp: sp(it)})
			prevHole = it
		default:
			r.err(f, sp(items[i]), source.BadPattern, "pattern items are words and holes", "")
			return
		}
	}
	if pendingWord != "" {
		r.err(f, sp(items[len(items)-1]), source.BadPattern, `after a block hole only "<word> [b: block]" pairs follow`, "")
	}
	// Names that appear twice, and the receiver.
	seen := map[string]*Param{}
	if ph.Receiver != nil {
		seen[ph.Receiver.Name] = ph.Receiver
	}
	for _, prm := range allParams(ph) {
		if prm == ph.Receiver {
			continue
		}
		if prev := seen[prm.Name]; prev != nil {
			r.err(f, prm.Sp, source.DuplicateName, `"`+prm.Name+`" is already defined at `+at(f, prev.Pos), "",
				f.also(prev.Sp, "first defined here"))
		} else {
			seen[prm.Name] = prm
		}
	}
	ph.Zero = len(ph.Items) == 1 && len(ph.Blocks) == 0
	r.finishPhrase(f, l, ph, native, nativeWord)
}

func allParams(ph *Phrase) []*Param {
	var out []*Param
	if ph.Receiver != nil {
		out = append(out, ph.Receiver)
	}
	for _, it := range ph.Items {
		if it.Param != nil {
			out = append(out, it.Param)
		}
	}
	for _, b := range ph.Blocks {
		out = append(out, b.Param)
	}
	return out
}

func holeText(h *parse.Hole) string {
	return "[" + h.Name.Name + ": " + typeString(h.Type) + "]"
}

func typeString(t parse.Type) string {
	switch t := t.(type) {
	case *parse.NamedType:
		return t.Name
	case *parse.ListType:
		return "list of " + typeString(t.Elem)
	case *parse.MapType:
		return "map of " + typeString(t.Key) + " to " + typeString(t.Value)
	}
	return "?"
}

func isStart(items []parse.Item) bool {
	switch len(items) {
	case 1:
		return parse.IsWord(items[0], "start")
	case 2:
		_, ok := items[0].(*parse.Hole)
		return ok && parse.IsWord(items[1], "start")
	}
	return false
}

// receiver declares the receiver hole of a phrase.
func (r *resolver) receiver(f *File, h *parse.Hole) *Param {
	prm := &Param{Name: h.Name.Name, Kind: ParamReceiver, Index: -1, Pos: h.Name.P, Sp: sp(h.Name), HoleSp: sp(h), Source: typeString(h.Type)}
	r.nameCheckDecl(f, sp(h.Name), h.Name.Name, "hole name")
	r.mark(f, h.Name.P, RoleName, &Def{File: f.Path, Pos: h.Name.P})
	if r.pkg.IsMain {
		r.err(f, sp(h), source.BadReceiver, "a main file has no receivers", "")
		prm.Type = types.TInvalid
		return prm
	}
	t := r.typ(f, h.Type, false, false)
	prm.Type = t
	switch t.(type) {
	case types.NodeOf:
	default:
		if !isInvalid(t) {
			r.err(f, sp(h), source.BadReceiver, "a receiver has type Node or p.Node, not "+typeString(h.Type), "")
		}
		prm.Type = types.TInvalid
	}
	return prm
}

func isInvalid(t types.Type) bool { return types.Equal(t, types.TInvalid) }

// param declares an inline or block hole.
func (r *resolver) param(f *File, h *parse.Hole, pkg *Package) *Param {
	prm := &Param{Name: h.Name.Name, Index: -1, Pos: h.Name.P, Sp: sp(h.Name), HoleSp: sp(h), Source: typeString(h.Type)}
	r.nameCheckDecl(f, sp(h.Name), h.Name.Name, "hole name")
	r.mark(f, h.Name.P, RoleName, &Def{File: f.Path, Pos: h.Name.P})
	if nt, ok := h.Type.(*parse.NamedType); ok && nt.Name == "block" {
		prm.Kind = ParamBlock
		prm.Type = types.TBlock
		return prm
	}
	prm.Kind = ParamHole
	prm.Type = r.typ(f, h.Type, false, false)
	return prm
}

// nameCheckDecl reports a reserved word used as a name and, for names that
// must start lower-case, schedules nothing here (the body pass handles
// renames).
func (r *resolver) nameCheckDecl(f *File, span source.Span, name, what string) {
	if reserved[name] {
		r.err(f, span, source.ReservedWord, `"`+name+`" is a reserved word and cannot be a `+what, "")
	}
}

// typ resolves a hole type. inColl is set inside list and map; stdOK is
// unused. Errors are bad-pattern.
func (r *resolver) typ(f *File, t parse.Type, inColl, key bool) types.Type {
	tsp := parse.TypeSpan(t)
	bad := func(msg, hint string, more ...source.Extra) types.Type {
		r.err(f, tsp, source.BadPattern, msg, hint, more...)
		return types.TInvalid
	}
	switch t := t.(type) {
	case *parse.ListType:
		return types.List{Elem: r.typ(f, t.Elem, true, false)}
	case *parse.MapType:
		k := r.typ(f, t.Key, true, true)
		v := r.typ(f, t.Value, true, false)
		return types.Map{Key: k, Val: v}
	case *parse.NamedType:
		name := t.Name
		if i := strings.IndexByte(name, '.'); i >= 0 {
			q, n := name[:i], name[i+1:]
			if q == r.pkg.Name && !r.pkg.IsMain {
				if n == "Node" || r.pkg.records[n] != nil {
					r.err(f, tsp, source.NonCanonical, `"`+name+`" names a type of this package: write "`+n+`"`, "",
						source.HelpEdit(`write "`+n+`"`, source.EditAt(tsp, n)))
					if n == "Node" {
						return r.pkg.Node.Type()
					}
					return r.pkg.records[n].Type()
				}
			}
			imp := f.byQual[q]
			if imp == nil {
				hint := ""
				var more []source.Extra
				if r.isStd(q) {
					hint = `add "use std/` + q + `"`
					more = append(more, f.notImported(q))
				}
				return bad(`unknown type "`+name+`"`, hint, more...)
			}
			imp.Used = true
			imp.typeRefs = true
			if imp.Pkg == nil {
				return types.TInvalid
			}
			if n == "Node" {
				if imp.Pkg.Node == nil {
					return bad(`unknown type "`+name+`"`, "")
				}
				return imp.Pkg.Node.Type()
			}
			rec := imp.Pkg.records[n]
			if rec == nil {
				return bad(`unknown type "`+name+`"`, "")
			}
			if !rec.Exported {
				var more []source.Extra
				if !imp.Pkg.IsStd {
					more = append(more, rec.File.also(rec.Sp, "lower-case: stays inside "+imp.Qual))
				}
				r.err(f, tsp, source.Unexported, `"`+name+`" is not exported by `+imp.Qual,
					"only names starting upper-case are visible outside "+imp.Qual, more...)
				return types.TInvalid
			}
			return rec.Type()
		}
		switch name {
		case "text":
			return types.TText
		case "number":
			return types.TNumber
		case "bool":
			return types.TBool
		case "time":
			return types.TTime
		case "node":
			return types.TNode
		case "msg":
			return types.TMsg
		case "shape":
			return types.TShape
		case "color":
			return types.TColor
		case "field":
			return types.TField
		case "package":
			return types.TPackage
		case "block":
			if inColl {
				return bad(`"block" cannot be nested inside list or map`, "")
			}
			return types.TBlock
		case "value":
			if !r.pkg.IsStd {
				return bad(`"value" is a type for std natives only`, "")
			}
			return types.TValue
		case "Node":
			if r.pkg.IsMain {
				return bad(`unknown type "Node"`, "a main file has no node type")
			}
			return r.pkg.Node.Type()
		}
		if rec := r.pkg.records[name]; rec != nil {
			return rec.Type()
		}
		return bad(`unknown type "`+name+`"`, "")
	}
	return types.TInvalid
}

func (r *resolver) isStd(name string) bool {
	if r.std == nil {
		r.std = map[string]bool{}
		for _, n := range stdPackages() {
			r.std[n] = true
		}
	}
	return r.std[name]
}

// finishPhrase completes a phrase after its pattern: native, naming,
// registration.
func (r *resolver) finishPhrase(f *File, l *parse.Line, ph *Phrase, native *parse.Text, nativeWord *parse.Word) {
	p := r.pkg
	var sk []string
	disp := []string{}
	gap := false
	for i, it := range ph.Items {
		if it.Param != nil {
			gap = true
			continue
		}
		sk = append(sk, it.Word)
		if i > 0 && gap {
			disp = append(disp, "…")
		}
		disp = append(disp, it.Word)
		gap = false
	}
	for _, bs := range ph.Blocks {
		if bs.Word != "" {
			sk = append(sk, bs.Word)
			disp = append(disp, "…", bs.Word)
		}
	}
	ph.Skeleton = strings.Join(sk, " ")
	ph.Name = strings.Join(disp, " ")
	ph.Contexts = stdlib.Any
	switch {
	case native != nil:
		id := strings.Trim(native.Raw, `"`)
		if !p.IsStd {
			r.err(f, source.Span{Start: nativeWord.P, End: native.End()}, source.NativeOutsideStd, `"native" is only allowed in std packages`, "")
		} else if sig := stdlib.Natives[id]; sig == nil {
			r.err(f, sp(native), source.BadPattern, `unknown native id "`+id+`"`, "")
		} else {
			ph.Native, ph.Sig = id, sig
			ph.Contexts = sig.Contexts
			ph.IsValue = sig.Result.Kind != stdlib.RuleNone
			if sig.Kind != stdlib.Phrase {
				ph.IsEvent, ph.Event = true, sig.Kind
			}
		}
		if ph.Sig == nil {
			ph.Native = id
		}
		if l.Block != nil {
			r.err(f, parse.LineSpan(l.Block.Lines[0]), source.BadPattern, "a native definition has no block", "")
		}
	case l.Block == nil:
		r.err(f, parse.LineSpan(l), source.BadPattern, `a definition needs an indented block or native "<id>"`, "")
	default:
		ph.IsValue = hasResult(l.Block)
		if p.IsStd {
			if c, ok := stdlib.Contexts[p.Name+"."+ph.Skeleton]; ok {
				ph.Contexts = c
			}
		}
	}
	p.Phrases = append(p.Phrases, ph)
	r.byLine[l] = ph
	fi := r.fileIndex(f)
	r.claims = append(r.claims, claim{key: ph.Skeleton, disp: ph.Name, what: "phrase", fi: fi, file: f, pos: ph.Pos, sp: ph.Sp,
		reject: func() { ph.dup = true }})
}

// hasResult reports a "result" statement in b, outside after blocks and
// block arguments: directly, or inside the blocks of if and each.
func hasResult(b *parse.Block) bool {
	for _, l := range b.Lines {
		g := l.Sentence.Groups[0]
		if parse.IsWord(g[0], "result") {
			return true
		}
		if parse.IsWord(g[0], "if") || parse.IsWord(g[0], "each") {
			if l.Block != nil && hasResult(l.Block) {
				return true
			}
			for _, c := range l.Conts {
				if hasResult(c.Block) {
					return true
				}
			}
		}
	}
	return false
}

func (r *resolver) declMain(f *File, l *parse.Line, g []parse.Item) {
	p := r.pkg
	mainSp := parse.SpanOfItems(g)
	switch {
	case !p.IsMain:
		var more []source.Extra
		if h := f.Load.Tree.Header; h != nil {
			more = append(more, f.also(parse.CommentSpan(h), "this is a library file"))
		}
		r.err(f, mainSp, source.ExtraMain, `"define main" stands only in a main file`, "", more...)
		return
	case r.mainAt != nil:
		r.err(f, mainSp, source.ExtraMain, `a second "define main"`, "",
			r.mainAt.file.also(r.mainAt.sp, `first "define main" here`))
		return
	case l.Block == nil:
		r.err(f, parse.LineSpan(l), source.BadPattern, "define main needs an indented block", "")
		return
	}
	r.mainAt = &decl{"main", f, l.Pos, mainSp}
	ph := &Phrase{Pkg: p, File: f, Pos: l.Pos, Sp: mainSp, Kind: PhraseMain, Head: "main", Skeleton: "main", Name: "main", Line: l,
		Contexts: stdlib.Any, Zero: true}
	p.Main = ph
	p.Phrases = append(p.Phrases, ph)
	r.byLine[l] = ph
}

func (r *resolver) declStart(f *File, l *parse.Line, ph *Phrase, items []parse.Item) {
	p := r.pkg
	startW := items[len(items)-1].(*parse.Word)
	ph.Kind = PhraseStart
	ph.Head, ph.Skeleton, ph.Name = "start", "start", "start"
	ph.Contexts = stdlib.Any
	ph.Zero = true
	ph.Items = []PatItem{{Word: "start", Pos: startW.P, Sp: sp(startW)}}
	if p.IsMain {
		span := sp(startW)
		if len(items) == 2 {
			span = spanTo(sp(items[0]), sp(startW))
		}
		r.err(f, span, source.BadReceiver, "a main file has no start", "")
		return
	}
	if len(items) == 1 {
		r.err(f, sp(startW), source.BadReceiver, "the receiver of start is [n: Node]", "")
		return
	}
	h := items[0].(*parse.Hole)
	nt, ok := h.Type.(*parse.NamedType)
	prm := &Param{Name: h.Name.Name, Kind: ParamReceiver, Index: -1, Pos: h.Name.P, Sp: sp(h.Name), HoleSp: sp(h), Source: typeString(h.Type)}
	r.nameCheckDecl(f, sp(h.Name), h.Name.Name, "hole name")
	r.mark(f, h.Name.P, RoleName, &Def{File: f.Path, Pos: h.Name.P})
	if !ok || nt.Name != "Node" {
		r.err(f, sp(h), source.BadReceiver, "the receiver of start is [n: Node]", "")
		prm.Type = types.TInvalid
	} else {
		prm.Type = p.Node.Type()
	}
	ph.Receiver = prm
	if l.Block == nil {
		r.err(f, parse.LineSpan(l), source.BadPattern, "define start needs an indented block", "")
		return
	}
	if r.startAt != nil {
		r.err(f, sp(startW), source.DuplicateName,
			"a second start; the first is at "+at(r.startAt.file, r.startAt.pos), "",
			r.startAt.file.also(r.startAt.sp, "first start here"))
		return
	}
	r.startAt = &decl{"start", f, ph.Pos, ph.Sp}
	p.Start = ph
	p.Phrases = append(p.Phrases, ph)
	r.byLine[l] = ph
}

// renameAlias turns an upper-case alias into a fix when every reference is a
// word or the first part of a member and the lower-case name is free.
func (r *resolver) renameAlias(imp *Import) {
	a := imp.Imp.Alias
	lower := lowerFirst(a)
	p := r.pkg
	free := !imp.typeRefs && lower != p.Name && imp.File.byQual[lower] == nil &&
		p.records[lower] == nil && p.fields[lower] == nil
	if len(p.heads[lower]) > 0 {
		free = false
	}
	if !free {
		r.err(imp.File, aliasWord(imp.Imp), source.NonCanonical,
			`"`+a+`" must start lower-case: only exported names start upper-case`, "rename it",
			source.HelpEdit("rename it", lowerEdit(imp.Imp.AliasPos, a)),
			source.Note(`rename every use of "`+a+`" the same way`))
		return
	}
	r.addFix(imp.File, format.Fix{Pos: imp.Imp.AliasPos, Rename: lower})
	for _, pos := range imp.refs {
		r.addFix(imp.File, format.Fix{Pos: pos, Rename: lower})
	}
}
