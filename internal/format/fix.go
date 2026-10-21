package format

import (
	"github.com/synodlang/compiler/internal/parse"
	"github.com/synodlang/compiler/internal/source"
)

// Fix is a change to the canonical form that only the resolver can decide,
// because it depends on what the words mean: the order of the settings of a
// record, the spelling of a name, or a redundant package qualifier. The
// resolver finds them, FormatWith applies them to the tree of the file, and
// CheckWith reports the first line they change as non-canonical.
//
// A fix names its place by position, in the tree FormatWith parses from the
// same file. A fix whose position holds no item of the kind it expects is
// ignored, so a stale fix does nothing.
type Fix struct {
	// Pos is the position of a Word or Member item (also the name of a
	// hole), or of the "message" word of a record sentence.
	Pos source.Pos
	// Rename is the new spelling of a Word, or of the first part of a
	// Member.
	Rename string
	// DropPrefix removes part number Part of a Member ("own.X" becomes
	// "X", "n.own.F" with Part 1 becomes "n.F"); a Member with one part
	// left is a Word. It takes precedence over Rename.
	DropPrefix bool
	// Part is the index of the part DropPrefix removes; 0 is the first.
	Part int
	// Order lists, for a record sentence "message T with a x, b y", the
	// setting words in the order of the record's fields. It applies only
	// when the sentence has exactly those words, each once, and each
	// setting is a word followed by a single item.
	Order []string
}

// FormatWith is Format after applying fixes to the freshly parsed tree,
// before canonicalization. With no fixes it is Format.
func FormatWith(f *source.File, fixes []Fix) ([]byte, *source.Diagnostic) {
	tree, d := parse.Parse(f)
	if d != nil {
		return nil, d
	}
	if len(fixes) > 0 {
		applyFixes(tree, fixes)
	}
	canonicalize(tree)
	return emit(tree), nil
}

type fixer struct{ byPos map[source.Pos]Fix }

// applyFixes applies fixes to t. When several fixes name one position, the
// first one wins.
func applyFixes(t *parse.File, fixes []Fix) {
	fx := &fixer{byPos: make(map[source.Pos]Fix, len(fixes))}
	for _, f := range fixes {
		if _, dup := fx.byPos[f.Pos]; !dup {
			fx.byPos[f.Pos] = f
		}
	}
	for _, n := range t.Body {
		if l, ok := n.(*parse.Line); ok {
			fx.line(l)
		}
	}
}

func (fx *fixer) line(l *parse.Line) {
	fx.sentence(l.Sentence)
	fx.block(l.Block)
	for _, c := range l.Conts {
		fx.block(c.Block)
	}
}

func (fx *fixer) block(b *parse.Block) {
	if b == nil {
		return
	}
	for _, l := range b.Lines {
		fx.line(l)
	}
}

func (fx *fixer) sentence(s *parse.Sentence) {
	fx.reorder(s)
	for _, g := range s.Groups {
		for i, it := range g {
			g[i] = fx.item(it)
		}
	}
}

// item applies the fix at it, and those inside it, and returns the item to
// put in its place.
func (fx *fixer) item(it parse.Item) parse.Item {
	switch v := it.(type) {
	case *parse.Word:
		if f, ok := fx.byPos[v.P]; ok && f.Rename != "" && !f.DropPrefix {
			v.Name = f.Rename
		}
	case *parse.Member:
		f, ok := fx.byPos[v.P]
		switch {
		case !ok:
		case f.DropPrefix:
			if f.Part < 0 || f.Part >= len(v.Parts) {
				break
			}
			v.Parts = append(v.Parts[:f.Part:f.Part], v.Parts[f.Part+1:]...)
			if len(v.Parts) == 1 {
				return &parse.Word{P: v.P, Name: v.Parts[0]}
			}
		case f.Rename != "":
			v.Parts = append([]string{f.Rename}, v.Parts[1:]...)
		}
	case *parse.Paren:
		fx.sentence(v.S)
	case *parse.List:
		for i, e := range v.Items {
			v.Items[i] = fx.item(e)
		}
	case *parse.Hole:
		fx.item(v.Name)
	}
	return it
}

// reorder applies an Order fix to the record sentence in s, if it has one.
func (fx *fixer) reorder(s *parse.Sentence) {
	g := s.Groups[0]
	for i, it := range g {
		w, ok := it.(*parse.Word)
		if !ok || w.Name != "message" {
			continue
		}
		f, ok := fx.byPos[w.P]
		if !ok || f.Order == nil {
			continue
		}
		// message T with a x [, b y ...]
		if i+5 != len(g) || !parse.IsWord(g[i+2], "with") {
			return
		}
		type setting struct {
			word string
			pair []parse.Item
		}
		var all []setting
		add := func(pair []parse.Item) bool {
			k, ok := pair[0].(*parse.Word)
			if ok {
				all = append(all, setting{k.Name, pair})
			}
			return ok
		}
		if !add(g[i+3 : i+5]) {
			return
		}
		for _, grp := range s.Groups[1:] {
			if len(grp) != 2 || !add(grp) {
				return
			}
		}
		if len(all) != len(f.Order) {
			return
		}
		var sorted []setting
		used := make([]bool, len(all))
		for _, name := range f.Order {
			found := false
			for j, st := range all {
				if !used[j] && st.word == name {
					used[j], found = true, true
					sorted = append(sorted, st)
					break
				}
			}
			if !found {
				return
			}
		}
		head := append(append([]parse.Item(nil), g[:i+3]...), sorted[0].pair...)
		groups := [][]parse.Item{head}
		for _, st := range sorted[1:] {
			groups = append(groups, st.pair)
		}
		s.Groups = groups
		return
	}
}
