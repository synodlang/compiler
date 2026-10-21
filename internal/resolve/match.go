package resolve

import (
	"strings"

	"github.com/synodlang/compiler/internal/parse"
)

// A form is a pattern that a sentence can match by shape: a defined phrase
// or a language form written as a pseudo-pattern, so that one matcher and
// one hint generator serve both (spec/03 section 3.3).

type fkind uint8

const (
	fkPhrase fkind = iota
	fkSet
	fkSetItem
	fkIf
	fkAfter
	fkDeliver
	fkEmit
	fkResult
	fkDraw
	fkItem
	fkNot
	fkBinary
	fkAttach
)

// pitem is an item of a form: a literal word, the head of a phrase (already
// matched by the candidate search), or a hole.
type pitem struct {
	lit  string
	head bool
	name string // hole name, for printing
}

type form struct {
	kind  fkind
	ph    *Phrase
	items []pitem
	// nblocks is the number of block holes; cont holds the continuation
	// words of blocks 1…. A draw takes zero or one block.
	nblocks int
	cont    []string
	op      string
	// text is the pattern as printed in hints and ambiguity messages.
	text string
}

func lit(s string) pitem  { return pitem{lit: s} }
func hole(n string) pitem { return pitem{name: n} }

// langStart holds the forms that begin with a language word.
var langStart = map[string][]*form{
	"set": {
		{kind: fkSet, items: []pitem{lit("set"), hole("place"), lit("to"), hole("value")}, text: "set [place] to [value]"},
		{kind: fkSetItem, items: []pitem{lit("set"), lit("item"), hole("key"), lit("of"), hole("place"), lit("to"), hole("value")},
			text: "set item [key] of [place] to [value]"},
	},
	"if": {
		{kind: fkIf, items: []pitem{lit("if"), hole("cond")}, nblocks: 1, text: "if [cond]"},
		{kind: fkIf, items: []pitem{lit("if"), hole("cond")}, nblocks: 2, cont: []string{"else"}, text: "if [cond] … else"},
	},
	"after":   {{kind: fkAfter, items: []pitem{lit("after"), hole("delay")}, nblocks: 1, text: "after [delay]"}},
	"deliver": {{kind: fkDeliver, items: []pitem{lit("deliver"), hole("msg"), lit("to"), hole("node")}, text: "deliver [msg] to [node]"}},
	"emit":    {{kind: fkEmit, items: []pitem{lit("emit"), hole("msg"), lit("by"), hole("node")}, text: "emit [msg] by [node]"}},
	"result":  {{kind: fkResult, items: []pitem{lit("result"), hole("value")}, text: "result [value]"}},
	"draw": {
		{kind: fkDraw, items: []pitem{lit("draw"), hole("shape"), lit("as"), hole("name")}, text: "draw [shape] as [name]"},
		{kind: fkDraw, items: []pitem{lit("draw"), hole("shape"), lit("as"), hole("name"), lit("for"), hole("key")}, text: "draw [shape] as [name] for [key]"},
	},
	"item": {{kind: fkItem, items: []pitem{lit("item"), hole("key"), lit("of"), hole("collection")}, text: "item [key] of [collection]"}},
	"not":  {{kind: fkNot, items: []pitem{lit("not"), hole("x")}, text: "not [x]"}},
}

// langWords are the words that begin a language form. Words that begin the
// dedicated forms each, message, define, use, when, always and has are
// included.
var langWords = map[string]bool{
	"set": true, "if": true, "each": true, "after": true, "deliver": true, "emit": true,
	"result": true, "always": true, "draw": true, "message": true, "item": true, "not": true,
	"define": true, "when": true, "use": true,
}

// infixForms are the operator forms.
var infixForms = func() []*form {
	var out []*form
	for _, op := range []string{"is", "is not", "is in", "is not in", "less than", "more than", "at least", "at most",
		"and", "or", "plus", "minus", "times", "divided by"} {
		items := []pitem{hole("a")}
		for _, w := range strings.Fields(op) {
			items = append(items, lit(w))
		}
		items = append(items, hole("b"))
		out = append(out, &form{kind: fkBinary, op: op, items: items, text: "[a] " + op + " [b]"})
	}
	return out
}()

// infixSecond are the words that can stand second in an operator form.
var infixSecond = map[string]bool{
	"is": true, "less": true, "more": true, "at": true, "and": true, "or": true,
	"plus": true, "minus": true, "times": true, "divided": true,
}

var attachForm = &form{kind: fkAttach, items: []pitem{hole("name"), lit("of"), hole("key")}, text: "[name] of [key]"}

// phraseForm returns the form of a defined phrase.
func phraseForm(ph *Phrase) *form {
	if ph.form != nil {
		return ph.form
	}
	f := &form{kind: fkPhrase, ph: ph, nblocks: len(ph.Blocks)}
	for i, it := range ph.Items {
		switch {
		case i == 0:
			f.items = append(f.items, pitem{head: true, lit: ph.Head})
		case it.Param != nil:
			f.items = append(f.items, hole(it.Param.Name))
		default:
			f.items = append(f.items, lit(it.Word))
		}
	}
	for i, bs := range ph.Blocks {
		if i > 0 {
			f.cont = append(f.cont, bs.Word)
		}
	}
	ph.form = f
	return f
}

// shape reports whether the sentence has the form's shape: the same number
// of items, equal literal words, and the same block structure.
func (f *form) shape(items []parse.Item, ln *parse.Line) bool {
	if len(items) != len(f.items) {
		return false
	}
	for i, p := range f.items {
		if p.lit != "" && !p.head && !parse.IsWord(items[i], p.lit) {
			return false
		}
	}
	nb := 0
	if ln != nil && ln.Block != nil {
		nb = 1 + len(ln.Conts)
	}
	if f.kind == fkDraw {
		return nb <= 1
	}
	if nb != f.nblocks {
		return false
	}
	for k, w := range f.cont {
		if !parse.IsWord(ln.Conts[k].Word, w) {
			return false
		}
	}
	return true
}

// display prints the form for messages: a language form as the form text,
// a phrase as its pattern, qualified as written.
func (f *form) display(qual string) string {
	if f.kind == fkPhrase {
		return f.ph.Describe(qual)
	}
	return f.text
}

// usable reports whether ph is a candidate for an ordinary sentence.
func usable(ph *Phrase) bool {
	return ph.Kind == PhraseNormal && !ph.IsEvent && !ph.dup
}

// candidate is a form with the way the sentence reaches it.
type candidate struct {
	f *form
	// qual is the import (or nil) through which a head was written, own
	// reports the current package's own name as qualifier.
	qual *Import
	own  bool
	// recv is the receiver name when the form is a receiver phrase: the
	// first part of the member, and the position of the qualifier part.
	recv string
	// qualPart is the index of the qualifier part in a receiver member
	// ("n.gossip.Learn" is 1), or -1; for a plain "q.H" it is 0.
	qualPart int
}

// candidates finds the forms a sentence can call, from its first item.
func (b *body) candidates(items []parse.Item) []candidate {
	var out []candidate
	pkg := b.r.pkg
	switch it := items[0].(type) {
	case *parse.Word:
		if fs, ok := langStart[it.Name]; ok {
			for _, f := range fs {
				out = append(out, candidate{f: f, qualPart: -1})
			}
			return out
		}
		if langWords[it.Name] {
			return nil
		}
		heads := pkg.heads[it.Name]
		for _, ph := range heads {
			if usable(ph) && ph.Receiver == nil {
				out = append(out, candidate{f: phraseForm(ph), qualPart: -1})
			}
		}
		if len(items) == 3 && parse.IsWord(items[1], "of") && len(heads) == 0 {
			out = append(out, candidate{f: attachForm, qualPart: -1})
		}
	case *parse.Member:
		parts := it.Parts
		if imp, own := b.qualifier(parts[0]); (imp != nil || own) && len(parts) == 2 {
			if pk := b.qualPkg(imp, own); pk != nil {
				for _, ph := range pk.heads[parts[1]] {
					if usable(ph) && ph.Receiver == nil {
						out = append(out, candidate{f: phraseForm(ph), qual: imp, own: own, qualPart: 0})
					}
				}
			}
		}
		if b.isValue(parts[0]) {
			switch {
			case len(parts) == 2:
				for _, ph := range pkg.heads[parts[1]] {
					if usable(ph) && ph.Receiver != nil {
						out = append(out, candidate{f: phraseForm(ph), recv: parts[0], qualPart: -1})
					}
				}
			case len(parts) == 3:
				if imp, own := b.qualifier(parts[1]); imp != nil || own {
					if pk := b.qualPkg(imp, own); pk != nil {
						for _, ph := range pk.heads[parts[2]] {
							if usable(ph) && ph.Receiver != nil {
								out = append(out, candidate{f: phraseForm(ph), qual: imp, own: own, recv: parts[0], qualPart: 1})
							}
						}
					}
				}
			}
		}
	}
	if len(items) >= 3 {
		if w, ok := items[1].(*parse.Word); ok && infixSecond[w.Name] {
			for _, f := range infixForms {
				out = append(out, candidate{f: f, qualPart: -1})
			}
		}
	}
	return out
}

// qualifier reports whether name is an import qualifier of the file (the
// import, marked used) or the current package's own name (own).
func (b *body) qualifier(name string) (imp *Import, own bool) {
	if imp := b.f.byQual[name]; imp != nil {
		imp.Used = true
		return imp, false
	}
	if name == b.r.pkg.Name && !b.r.pkg.IsMain {
		return nil, true
	}
	return nil, false
}

// qualPkg returns the package a qualifier names; nil when the import is
// unresolved.
func (b *body) qualPkg(imp *Import, own bool) *Package {
	if own {
		return b.r.pkg
	}
	if imp == nil {
		return nil
	}
	return imp.Pkg
}

// isValue reports whether name is a value name: a local, parameter, binding
// or zero-hole phrase.
func (b *body) isValue(name string) bool {
	if s, _ := b.lookup(name); s != nil {
		return true
	}
	return b.zeroPhrase(name) != nil
}

// zeroPhrase returns the zero-hole phrase without receiver called name.
func (b *body) zeroPhrase(name string) *Phrase {
	for _, ph := range b.r.pkg.heads[name] {
		if usable(ph) && ph.Receiver == nil && ph.Zero {
			return ph
		}
	}
	return nil
}
