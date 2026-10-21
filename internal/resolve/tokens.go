package resolve

import (
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/synodlang/compiler/internal/parse"
	"github.com/synodlang/compiler/internal/source"
	"github.com/synodlang/compiler/internal/stdlib"
)

// Role classifies a token for --tokens.
type Role string

// The roles of tokens.
const (
	RoleLanguage Role = "language" // reserved words
	RolePhrase   Role = "phrase"   // heads and literal words of calls and patterns
	RoleName     Role = "name"     // locals, holes, bindings, packages, import paths
	RoleField    Role = "field"    // node fields, record fields, setting words
	RoleType     Role = "type"     // record types, Node, hole types
	RoleValue    Role = "value"    // literals, true and false
	RoleComment  Role = "comment"
)

// Def points to the declaration a token refers to.
type Def struct {
	File string
	Pos  source.Pos
}

// Token is one classified token of a file. Len counts code points.
type Token struct {
	Line, Col, Len int
	Role           Role
	// Def is nil for std declarations and language words.
	Def *Def
}

type mark struct {
	role Role
	def  *Def
}

// mark records the role of the token at pos.
func (r *resolver) mark(f *File, pos source.Pos, role Role, def *Def) {
	if def != nil && strings.HasPrefix(def.File, "std/") {
		def = nil
	}
	m := r.marks[f.Load]
	if m == nil {
		m = map[source.Pos]mark{}
		r.marks[f.Load] = m
	}
	m[pos] = mark{role, def}
}

// stdPackages lists the std packages that hold library files directly.
func stdPackages() []string {
	root := stdlib.Root()
	ents, err := fsReadDir(root)
	if err != nil {
		return nil
	}
	return ents
}

func (r *resolver) tokens() {
	for _, f := range r.pkg.Files {
		t := &tokenizer{r: r, f: f, marks: r.marks[f.Load]}
		tree := f.Load.Tree
		if tree.Header != nil {
			t.comment(tree.Header)
		}
		for _, n := range tree.Body {
			switch v := n.(type) {
			case *parse.Line:
				t.line(v)
			case *parse.CommentGroup:
				for _, c := range v.Comments {
					t.comment(c)
				}
			}
		}
		for _, c := range tree.Trailing {
			t.comment(c)
		}
		sort.SliceStable(t.out, func(i, j int) bool {
			a, b := t.out[i], t.out[j]
			if a.Line != b.Line {
				return a.Line < b.Line
			}
			return a.Col < b.Col
		})
		r.pkg.Tokens[f.Load] = t.out
	}
}

type tokenizer struct {
	r     *resolver
	f     *File
	marks map[source.Pos]mark
	out   []Token
}

func (t *tokenizer) add(pos source.Pos, n int, role Role) {
	var def *Def
	if m, ok := t.marks[pos]; ok {
		role, def = m.role, m.def
	}
	t.out = append(t.out, Token{Line: pos.Line, Col: pos.Col, Len: n, Role: role, Def: def})
}

func (t *tokenizer) comment(c *parse.Comment) {
	if c == nil {
		return
	}
	text := "#"
	if c.Text != "" {
		text = "# " + c.Text
	}
	t.add(c.Pos, utf8.RuneCountInString(text), RoleComment)
}

func (t *tokenizer) line(l *parse.Line) {
	for _, c := range l.Leading {
		t.comment(c)
	}
	t.sentence(l.Sentence)
	t.comment(l.Comment)
	t.block(l.Block)
	for _, c := range l.Conts {
		for _, x := range c.Leading {
			t.comment(x)
		}
		t.add(c.Word.P, utf8.RuneCountInString(c.Word.Name), RoleLanguage)
		t.comment(c.Comment)
		t.block(c.Block)
	}
}

func (t *tokenizer) block(b *parse.Block) {
	if b == nil {
		return
	}
	for _, l := range b.Lines {
		t.line(l)
	}
	for _, c := range b.Trailing {
		t.comment(c)
	}
}

func (t *tokenizer) sentence(s *parse.Sentence) {
	for _, g := range s.Groups {
		for _, it := range g {
			t.item(it)
		}
	}
}

func (t *tokenizer) item(it parse.Item) {
	switch v := it.(type) {
	case *parse.Word:
		role := RoleName
		switch {
		case v.Name == "true" || v.Name == "false":
			role = RoleValue
		case reserved[v.Name]:
			role = RoleLanguage
		}
		t.add(v.P, utf8.RuneCountInString(v.Name), role)
	case *parse.Member:
		for i, p := range v.Parts {
			t.add(partPos(v, i), utf8.RuneCountInString(p), RoleName)
		}
	case *parse.Number:
		t.add(v.P, len(v.Text), RoleValue)
	case *parse.Duration:
		t.add(v.P, len(v.Text), RoleValue)
	case *parse.Text:
		t.add(v.P, utf8.RuneCountInString(v.Raw), RoleValue)
	case *parse.Color:
		t.add(v.P, len(v.Text), RoleValue)
	case *parse.Path:
		t.add(v.P, utf8.RuneCountInString(v.Text), RoleName)
	case *parse.Paren:
		t.sentence(v.S)
	case *parse.List:
		for _, e := range v.Items {
			t.item(e)
		}
	case *parse.Hole:
		t.add(v.Name.P, utf8.RuneCountInString(v.Name.Name), RoleName)
		t.typ(v.Type)
	}
}

func (t *tokenizer) typ(ty parse.Type) {
	switch v := ty.(type) {
	case *parse.NamedType:
		role := RoleType
		def := (*Def)(nil)
		if i := strings.IndexByte(v.Name, '.'); i >= 0 {
			t.add(v.P, i, RoleName)
			pos := source.Pos{Line: v.P.Line, Col: v.P.Col + i + 1}
			t.out = append(t.out, Token{Line: pos.Line, Col: pos.Col, Len: len(v.Name) - i - 1, Role: role, Def: def})
			return
		}
		t.add(v.P, len(v.Name), role)
	case *parse.ListType:
		t.add(v.P, 4, RoleLanguage)
		t.typ(v.Elem)
	case *parse.MapType:
		t.add(v.P, 3, RoleLanguage)
		t.typ(v.Key)
		t.typ(v.Value)
	}
}
