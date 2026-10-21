package resolve

import (
	"io/fs"
	"sort"
	"strconv"
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

// hint is the hint of an unknown-phrase diagnostic with its structured
// parts: text is the short-form hint, verbatim; the rest feeds the rich
// form.
type hint struct {
	text string
	// help is the text of the help that carries edits; text when empty.
	help  string
	edits []source.Edit
	// labels are secondary labels, notes are statements of fact.
	labels []label
	notes  []string
}

// label is a secondary label in a file.
type label struct {
	f    *File
	sp   source.Span
	text string
}

// extras turns the hint into the extras of a diagnostic.
func (h hint) extras() []source.Extra {
	var out []source.Extra
	for _, l := range h.labels {
		out = append(out, l.f.also(l.sp, l.text))
	}
	switch {
	case len(h.edits) > 0:
		text := h.help
		if text == "" {
			text = h.text
		}
		out = append(out, source.HelpEdit(text, h.edits...))
	case h.help != "":
		out = append(out, source.HelpText(h.help))
	case h.text != "" && len(h.notes) > 0:
		out = append(out, source.HelpText(h.text))
	}
	for _, n := range h.notes {
		out = append(out, source.Note(n))
	}
	return out
}

// unknownName reports a name that means nothing. it is the word or member.
func (b *body) unknownName(it parse.Item, text, first string) {
	whole := sp(it)
	firstSp := whole
	if m, ok := it.(*parse.Member); ok {
		firstSp = parse.PartSpan(m, 0)
	}
	if first != text || strings.Contains(text, ".") {
		// a member whose first part is unknown
		if b.r.isStd(first) && b.f.byQual[first] == nil {
			b.r.err(b.f, firstSp, source.UnknownPhrase, `"`+first+`" is not imported`, `add "use std/`+first+`"`,
				b.f.notImported(first))
			return
		}
	}
	h := hint{}
	if c := closest(first, b.visibleNames()); c != "" {
		h.text = `did you mean "` + strings.Replace(text, first, c, 1) + `"?`
		h.edits = []source.Edit{source.EditAt(firstSp, c)}
	}
	b.r.err(b.f, whole, source.UnknownPhrase, `no phrase matches "`+text+`"`, h.text, h.extras()...)
}

// closestMember suggests the member of a package nearest to head: the hint,
// and the name to write.
func (b *body) closestMember(imp *Import, own bool, head string) (string, string) {
	pk := b.qualPkg(imp, own)
	if pk == nil {
		return "", ""
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
		return `did you mean "` + q + "." + c + `"?`, c
	}
	return "", ""
}

// ---- unknown phrases ----

func (b *body) noMatch(s *parse.Sentence, items []parse.Item, ln *parse.Line) {
	text := printSentence(s)
	if m, ok := items[0].(*parse.Member); ok && !b.isValue(m.Parts[0]) && b.f.byQual[m.Parts[0]] == nil &&
		m.Parts[0] != b.r.pkg.Name && b.r.isStd(m.Parts[0]) {
		b.r.err(b.f, parse.PartSpan(m, 0), source.UnknownPhrase, `"`+m.Parts[0]+`" is not imported`, `add "use std/`+m.Parts[0]+`"`,
			b.f.notImported(m.Parts[0]))
		for _, g := range s.Groups {
			b.touch(g)
		}
		return
	}
	h := b.sentenceHint(items, ln)
	b.r.err(b.f, parse.SentenceSpan(s), source.UnknownPhrase, `no phrase matches "`+text+`"`, h.text,
		append([]source.Extra{source.WithLabel("no phrase takes these items")}, h.extras()...)...)
	for _, g := range s.Groups {
		b.touch(g)
	}
	if ln != nil {
		b.touchBody(ln)
	}
}

func (b *body) eventHint(ev []parse.Item) hint {
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

func (b *body) sentenceHint(items []parse.Item, ln *parse.Line) hint {
	if p, ok := items[0].(*parse.Paren); ok && ln != nil {
		open := source.Span{Start: p.P, End: source.Pos{Line: p.P.Line, Col: p.P.Col + 1}}
		cl := source.Span{Start: source.Pos{Line: p.E.Line, Col: p.E.Col - 1}, End: p.E}
		return hint{text: "remove the parentheses",
			edits: []source.Edit{source.EditAt(open, ""), source.EditAt(cl, "")}}
	}
	if parse.IsWord(items[0], "else") && len(items) > 1 && parse.IsWord(items[1], "if") {
		return hint{text: "nest the if inside else"}
	}
	if h, ok := b.parenHint(items); ok {
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

// sentenceSkeleton returns the tokens a sentence is compared by: its first
// item and the bare words that are not names, with the span of each.
func (b *body) sentenceSkeleton(items []parse.Item) (toks []string, head string, spans []source.Span) {
	head = printItem(items[0])
	toks = append(toks, head)
	spans = append(spans, sp(items[0]))
	for _, it := range items[1:] {
		if w, ok := it.(*parse.Word); ok && !b.isValue(w.Name) && b.f.byQual[w.Name] == nil {
			toks = append(toks, w.Name)
			spans = append(spans, sp(w))
		}
	}
	return toks, head, spans
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

// nearest finds the pattern closest to the sentence and builds the hint. It
// proposes an edit only when exactly one token of the sentence differs from
// the pattern.
func (b *body) nearest(items []parse.Item, cands []*form, quals []string) hint {
	sk, head, spans := b.sentenceSkeleton(items)
	headTail := head
	if i := strings.LastIndexByte(head, '.'); i >= 0 {
		headTail = head[i+1:]
	}
	qualified := strings.Contains(head, ".")
	bestText, bestD := "", 0.0
	best := -1
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
		if best < 0 || d < bestD || d == bestD && text < bestText {
			best, bestD, bestText = i, d, text
		}
	}
	if best < 0 {
		return hint{}
	}
	h := hint{text: `did you mean "` + bestText + `"?`}
	f, q := cands[best], quals[best]
	ps := formSkeleton(f, q)
	if len(ps) == len(sk) && !(f.kind == fkPhrase && f.ph.Receiver != nil) {
		diff, n := 0, 0
		for i := range ps {
			if ps[i] != sk[i] {
				diff, n = i, n+1
			}
		}
		ok := n == 1
		if ok && diff == 0 {
			if m, isMember := items[0].(*parse.Member); isMember {
				ok = strings.HasPrefix(ps[0], m.Parts[0]+".")
			}
		}
		if ok {
			h.edits = []source.Edit{source.EditAt(spans[diff], ps[diff])}
		}
	}
	if f.kind == fkPhrase {
		ph := f.ph
		switch {
		case ph.Pkg == b.r.pkg:
			h.labels = append(h.labels, label{ph.File, ph.Sp, "defined here"})
		case ph.Pkg.IsStd:
			h.notes = append(h.notes, `"`+q+"."+ph.Head+`" is in `+ph.Pkg.Path)
		}
	}
	return h
}

// parenHint looks for a way to read the sentence as a form whose holes take
// parenthesized runs of items. The hint carries one label and one pair of
// insertions per run.
func (b *body) parenHint(items []parse.Item) (hint, bool) {
	var forms []*form
	for _, c := range b.candidates(items) {
		if len(c.f.items) < len(items) {
			forms = append(forms, c.f)
		}
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
		return hint{}, false
	}
	var parts []string
	var h hint
	noun := ""
	si := 0
	for _, n := range best {
		run := items[si : si+n]
		if n >= 2 {
			parts = append(parts, "("+printItems(run)+")")
			rsp := parse.SpanOfItems(run)
			kind := "value"
			text := strconv.Itoa(n) + " items here; wrap them in parentheses"
			if si > 0 {
				prev := printItem(items[si-1])
				if parse.IsWord(items[si-1], "if") {
					kind = "condition"
				}
				text = strconv.Itoa(n) + ` items here; "` + prev + `" takes one ` + kind
			}
			if noun == "" || noun == kind {
				noun = kind
			} else {
				noun = "value"
			}
			h.labels = append(h.labels, label{b.f, rsp, text})
			h.edits = append(h.edits, parenEdits(rsp)...)
		} else {
			parts = append(parts, printItem(run[0]))
		}
		si += n
	}
	h.text = `did you mean "` + strings.Join(parts, " ") + `"?`
	h.help = "parenthesize the " + noun
	if len(h.labels) > 1 {
		h.help += "s"
	}
	return h, true
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
