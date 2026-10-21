package resolve

import (
	"io/fs"
	"sort"
	"strings"

	"github.com/synodlang/compiler/internal/format"
	"github.com/synodlang/compiler/internal/parse"
	"github.com/synodlang/compiler/internal/source"
)

func formatDrop(pos source.Pos, part int) format.Fix {
	return format.Fix{Pos: pos, DropPrefix: true, Part: part}
}

// touchBody marks the qualifiers and names in a whole block as used.
func (b *body) touchBody(l *parse.Line) {
	var walk func(ln *parse.Line)
	walkBlock := func(bl *parse.Block) {
		if bl == nil {
			return
		}
		for _, x := range bl.Lines {
			walk(x)
		}
	}
	walk = func(ln *parse.Line) {
		for _, g := range ln.Sentence.Groups {
			b.touch(g)
		}
		walkBlock(ln.Block)
		for _, c := range ln.Conts {
			walkBlock(c.Block)
		}
	}
	walkBlock(l.Block)
}

// ---- edit distance ----

func charLev(a, b string) int {
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
			c := 1
			if ra[i-1] == rb[j-1] {
				c = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+c)
		}
		prev = cur
	}
	return prev[len(rb)]
}

func tokDist(a, b string) float64 {
	m := max(len([]rune(a)), len([]rune(b)))
	if m == 0 {
		return 0
	}
	return float64(charLev(a, b)) / float64(m)
}

// seqDist is the token Levenshtein distance: insert and delete cost 1,
// substitution the relative character distance.
func seqDist(a, b []string) float64 {
	prev := make([]float64, len(b)+1)
	for j := range prev {
		prev[j] = float64(j)
	}
	for i := 1; i <= len(a); i++ {
		cur := make([]float64, len(b)+1)
		cur[0] = float64(i)
		for j := 1; j <= len(b); j++ {
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+tokDist(a[i-1], b[j-1]))
		}
		prev = cur
	}
	return prev[len(b)]
}

// closest returns the candidate nearest to name, or "" if none is near.
func closest(name string, cands []string) string {
	best, bd := "", 0.0
	for _, c := range cands {
		if c == name {
			continue
		}
		d := tokDist(name, c)
		if d <= 0.4 && (best == "" || d < bd || d == bd && c < best) {
			best, bd = c, d
		}
	}
	return best
}

// ---- unknown names ----

func (b *body) visibleNames() []string {
	seen := map[string]bool{}
	var out []string
	add := func(n string) {
		if !seen[n] {
			seen[n] = true
			out = append(out, n)
		}
	}
	for s := b.sc; s != nil; s = s.parent {
		for n := range s.syms {
			add(n)
		}
	}
	for h, phs := range b.r.pkg.heads {
		for _, ph := range phs {
			if usable(ph) && ph.Receiver == nil && ph.Zero {
				add(h)
			}
		}
	}
	for q := range b.f.byQual {
		add(q)
	}
	sort.Strings(out)
	return out
}

// unknownName reports a name that means nothing.
func (b *body) unknownName(pos source.Pos, text, first string) {
	if first != text || strings.Contains(text, ".") {
		// a member whose first part is unknown
		if b.r.isStd(first) && b.f.byQual[first] == nil {
			b.r.err(b.f, pos, source.UnknownPhrase, `"`+first+`" is not imported`, `add "use std/`+first+`"`)
			return
		}
	}
	hint := ""
	if c := closest(first, b.visibleNames()); c != "" {
		hint = `did you mean "` + strings.Replace(text, first, c, 1) + `"?`
	}
	b.r.err(b.f, pos, source.UnknownPhrase, `no phrase matches "`+text+`"`, hint)
}

// closestMember suggests the member of a package nearest to head.
func (b *body) closestMember(imp *Import, own bool, head string) string {
	pk := b.qualPkg(imp, own)
	if pk == nil {
		return ""
	}
	q := pk.Name
	if imp != nil {
		q = imp.Qual
	}
	var names []string
	for h, phs := range pk.heads {
		for _, ph := range phs {
			if usable(ph) && ph.Receiver == nil && (ph.Exported || own) {
				names = append(names, h)
			}
		}
	}
	sort.Strings(names)
	if c := closest(head, names); c != "" {
		return `did you mean "` + q + "." + c + `"?`
	}
	return ""
}

// ---- unknown phrases ----

func (b *body) noMatch(s *parse.Sentence, items []parse.Item, ln *parse.Line) {
	text := printSentence(s)
	if m, ok := items[0].(*parse.Member); ok && !b.isValue(m.Parts[0]) && b.f.byQual[m.Parts[0]] == nil &&
		m.Parts[0] != b.r.pkg.Name && b.r.isStd(m.Parts[0]) {
		b.r.err(b.f, m.P, source.UnknownPhrase, `"`+m.Parts[0]+`" is not imported`, `add "use std/`+m.Parts[0]+`"`)
		for _, g := range s.Groups {
			b.touch(g)
		}
		return
	}
	b.r.err(b.f, items[0].Pos(), source.UnknownPhrase, `no phrase matches "`+text+`"`, b.sentenceHint(items, ln))
	for _, g := range s.Groups {
		b.touch(g)
	}
	if ln != nil {
		b.touchBody(ln)
	}
}

func (b *body) eventHint(ev []parse.Item) string {
	var cands []*form
	var quals []string
	for _, imp := range b.f.Imports {
		if imp.Pkg == nil {
			continue
		}
		for _, h := range sortedKeys(imp.Pkg.heads) {
			for _, ph := range imp.Pkg.heads[h] {
				if ph.IsEvent && ph.Exported {
					cands = append(cands, phraseForm(ph))
					quals = append(quals, imp.Qual)
				}
			}
		}
	}
	return b.nearest(ev, cands, quals)
}

func (b *body) sentenceHint(items []parse.Item, ln *parse.Line) string {
	if _, ok := items[0].(*parse.Paren); ok && ln != nil {
		return "remove the parentheses"
	}
	if parse.IsWord(items[0], "else") && len(items) > 1 && parse.IsWord(items[1], "if") {
		return "nest the if inside else"
	}
	if h := b.parenHint(items); h != "" {
		return h
	}
	// Candidates: language forms, current package phrases, exported
	// phrases of the imports.
	var cands []*form
	var quals []string
	for _, w := range sortedKeys(langStart) {
		for _, f := range langStart[w] {
			cands = append(cands, f)
			quals = append(quals, "")
		}
	}
	pkg := b.r.pkg
	for _, h := range sortedKeys(pkg.heads) {
		for _, ph := range pkg.heads[h] {
			if usable(ph) {
				cands = append(cands, phraseForm(ph))
				quals = append(quals, "")
			}
		}
	}
	for _, imp := range b.f.Imports {
		if imp.Pkg == nil {
			continue
		}
		for _, h := range sortedKeys(imp.Pkg.heads) {
			for _, ph := range imp.Pkg.heads[h] {
				if usable(ph) && ph.Exported {
					cands = append(cands, phraseForm(ph))
					quals = append(quals, imp.Qual)
				}
			}
		}
	}
	return b.nearest(items, cands, quals)
}

// skeleton returns the tokens a sentence is compared by: its first item and
// the bare words that are not names.
func (b *body) sentenceSkeleton(items []parse.Item) (toks []string, head string) {
	head = printItem(items[0])
	toks = append(toks, head)
	for _, it := range items[1:] {
		if w, ok := it.(*parse.Word); ok && !b.isValue(w.Name) && b.f.byQual[w.Name] == nil {
			toks = append(toks, w.Name)
		}
	}
	return toks, head
}

func formSkeleton(f *form, qual string) []string {
	var toks []string
	for i, p := range f.items {
		switch {
		case p.head:
			h := p.lit
			if qual != "" {
				h = qual + "." + h
			}
			toks = append(toks, h)
		case p.lit != "":
			if i == 0 && qual != "" {
				toks = append(toks, qual+"."+p.lit)
			} else {
				toks = append(toks, p.lit)
			}
		}
	}
	return toks
}

// nearest finds the pattern closest to the sentence and renders the hint.
func (b *body) nearest(items []parse.Item, cands []*form, quals []string) string {
	sk, head := b.sentenceSkeleton(items)
	headTail := head
	if i := strings.LastIndexByte(head, '.'); i >= 0 {
		headTail = head[i+1:]
	}
	qualified := strings.Contains(head, ".")
	bestText, bestD := "", 0.0
	found := false
	for i, f := range cands {
		ps := formSkeleton(f, quals[i])
		if len(ps) == 0 {
			continue
		}
		// Compare the head by its last segment when the sentence did not
		// write a qualifier.
		a := append([]string(nil), sk...)
		c := append([]string(nil), ps...)
		hd := tokDist(head, c[0])
		if !qualified {
			cc := c[0]
			if j := strings.LastIndexByte(cc, '.'); j >= 0 {
				cc = cc[j+1:]
			}
			hd = tokDist(a[0], cc)
			a[0], c[0] = headTail, cc
		}
		d := seqDist(a, c)
		m := float64(max(len(a), len(c)))
		if d > 0.5*m || hd > 0.5 {
			continue
		}
		text := f.display(quals[i])
		if f.kind == fkPhrase && f.ph.Receiver != nil {
			text = f.ph.Describe(quals[i])
		}
		if !found || d < bestD || d == bestD && text < bestText {
			found, bestD, bestText = true, d, text
		}
	}
	if !found {
		return ""
	}
	return `did you mean "` + bestText + `"?`
}

// parenHint looks for a way to read the sentence as a form whose holes take
// parenthesized runs of items.
func (b *body) parenHint(items []parse.Item) string {
	var forms []*form
	for _, c := range b.candidates(items) {
		if len(c.f.items) < len(items) {
			forms = append(forms, c.f)
		}
	}
	// Operator forms at the front: a sentence like "a is b" inside a larger
	// one is handled through the enclosing form.
	type sol struct {
		runs []int
	}
	var best []int
	bestScore := [3]int{-1, 0, 0}
	for _, f := range forms {
		var sols [][]int
		var rec func(pi, si int, runs []int)
		rec = func(pi, si int, runs []int) {
			if len(sols) >= 32 {
				return
			}
			if pi == len(f.items) {
				if si == len(items) {
					sols = append(sols, append([]int(nil), runs...))
				}
				return
			}
			p := f.items[pi]
			if p.lit != "" {
				if p.head {
					if si == 0 {
						rec(pi+1, si+1, append(runs, 1))
					}
					return
				}
				if si < len(items) && parse.IsWord(items[si], p.lit) {
					rec(pi+1, si+1, append(runs, 1))
				}
				return
			}
			left := len(f.items) - pi - 1
			for n := 1; si+n+left <= len(items); n++ {
				rec(pi+1, si+n, append(runs, n))
			}
		}
		rec(0, 0, nil)
		for _, runs := range sols {
			plausible, wrapped := 1, 0
			si := 0
			for _, n := range runs {
				if n >= 2 {
					wrapped++
					if !b.plausible(items[si : si+n]) {
						plausible = 0
					}
				}
				si += n
			}
			if wrapped == 0 {
				continue
			}
			score := [3]int{plausible, -wrapped, 0}
			if score[0] > bestScore[0] || score[0] == bestScore[0] && score[1] > bestScore[1] {
				bestScore, best = score, runs
			}
		}
	}
	if best == nil || bestScore[0] != 1 {
		return ""
	}
	var parts []string
	si := 0
	for _, n := range best {
		run := items[si : si+n]
		if n >= 2 {
			parts = append(parts, "("+printItems(run)+")")
		} else {
			parts = append(parts, printItem(run[0]))
		}
		si += n
	}
	return `did you mean "` + strings.Join(parts, " ") + `"?`
}

// plausible reports whether a run of items is a sentence of some form.
func (b *body) plausible(run []parse.Item) bool {
	if len(run) >= 2 && parse.IsWord(run[0], "message") {
		return len(run) >= 4
	}
	for _, c := range b.candidates(run) {
		if c.f.kind == fkIf || c.f.kind == fkAfter {
			continue
		}
		if len(c.f.items) == len(run) && c.f.nblocks == 0 {
			if c.f.shape(run, nil) {
				return true
			}
		}
	}
	return false
}

func fsReadDir(root fs.FS) ([]string, error) {
	ents, err := fs.ReadDir(root, ".")
	if err != nil {
		return nil, err
	}
	var out []string
	for _, e := range ents {
		if e.IsDir() {
			out = append(out, e.Name())
		}
	}
	return out, nil
}
