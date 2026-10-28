package check

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/synodlang/compiler/internal/ir"
	"github.com/synodlang/compiler/internal/parse"
	"github.com/synodlang/compiler/internal/resolve"
	"github.com/synodlang/compiler/internal/source"
	"github.com/synodlang/compiler/internal/stdlib"
	"github.com/synodlang/compiler/internal/types"
)

// This file checks a theme package (spec/06 section 6.7): its files hold only
// definitions `define <Name>` whose block is one `result <literal>`, exactly
// one for each name of the theme table (spec/09 section 9.5), each literal of
// the name's type and inside its domain. Everything else is bad-theme. The
// resolver declares nothing for a theme, so the definitions are read from the
// syntax tree.

var hexColor = regexp.MustCompile(`^#[0-9A-Fa-f]{6}$`)

// themeCheck holds the state of the check of one theme package.
type themeCheck struct {
	rp    *resolve.Package
	diags []source.Diagnostic
	vals  map[string]ir.ThemeValue
	first map[string]*themeDef
}

// themeDef remembers where a name was first defined.
type themeDef struct {
	file *resolve.File
	sp   source.Span
}

// checkTheme validates the theme package rp. It returns the values in table
// order, which are complete only when there is no diagnostic.
func checkTheme(rp *resolve.Package) ([]ir.ThemeValue, []source.Diagnostic) {
	t := &themeCheck{rp: rp, vals: map[string]ir.ThemeValue{}, first: map[string]*themeDef{}}
	for _, f := range rp.Files {
		for _, n := range f.Load.Tree.Body {
			if l, ok := n.(*parse.Line); ok {
				t.line(f, l)
			}
		}
	}
	t.missing()
	var out []ir.ThemeValue
	for _, n := range stdlib.ThemeNames {
		if v, ok := t.vals[n.Name]; ok {
			out = append(out, v)
		}
	}
	return out, t.diags
}

func (t *themeCheck) err(f *resolve.File, sp source.Span, msg, hint string, more ...source.Extra) {
	t.diags = append(t.diags, source.NewSpan(f.Load.Src, sp, source.BadTheme, msg, hint, more...))
}

// line checks one top-level line of a theme file.
func (t *themeCheck) line(f *resolve.File, l *parse.Line) {
	s := l.Sentence
	g := s.Groups[0]
	lineSp := parse.LineSpan(l)
	switch {
	case parse.IsWord(g[0], "use"):
		t.err(f, lineSp, "a theme imports nothing", "",
			source.HelpEdit("remove the line", source.Edit{Line: l.Pos.Line, Col: 1, End: source.Pos{Line: l.Pos.Line + 1, Col: 1}}))
		return
	case !parse.IsWord(g[0], "define"):
		t.err(f, lineSp, "a theme file holds only definitions of theme names", "",
			source.HelpText("write `define <Name>` with one `result` line, as std/theme/default does"))
		return
	}
	// define <Name>
	if len(s.Groups) != 1 || len(g) != 2 {
		t.err(f, lineSp, "a theme definition is `define <Name>`, with no holes and no native", "",
			source.HelpText("write `define <Name>` with one `result` line, as std/theme/default does"))
		return
	}
	w, ok := g[1].(*parse.Word)
	if !ok {
		t.err(f, lineSp, "a theme definition is `define <Name>`, with no holes and no native", "")
		return
	}
	name := stdlib.ThemeNameOf(w.Name)
	if name == nil {
		t.err(f, parse.SpanOf(w), `"`+w.Name+`" is not a name of the theme table`, "",
			source.Note("a theme defines exactly the "+strconv.Itoa(len(stdlib.ThemeNames))+" names of std/theme/default"))
		return
	}
	if prev := t.first[w.Name]; prev != nil {
		t.err(f, parse.SpanOf(w), `"`+w.Name+`" is defined twice`, "",
			source.Also(prev.file.Path, prev.sp, "first defined here"))
		return
	}
	t.first[w.Name] = &themeDef{file: f, sp: parse.SpanOf(w)}
	if l.Block == nil || len(l.Block.Lines) != 1 || len(l.Conts) > 0 {
		t.err(f, lineSp, `"`+w.Name+`" is defined by one "result" line holding a literal`, "")
		return
	}
	r := l.Block.Lines[0]
	rg := r.Sentence.Groups[0]
	if len(r.Sentence.Groups) != 1 || len(rg) != 2 || !parse.IsWord(rg[0], "result") || r.Block != nil {
		t.err(f, parse.LineSpan(r), `"`+w.Name+`" is defined by one "result" line holding a literal`, "")
		return
	}
	t.value(f, name, rg[1])
}

// value checks the literal of a name against its type and domain.
func (t *themeCheck) value(f *resolve.File, n *stdlib.ThemeName, it parse.Item) {
	bad := func(extra string) {
		t.err(f, parse.SpanOf(it), `"`+n.Name+`" must be `+n.Domain()+extra, "")
	}
	switch v := it.(type) {
	case *parse.Color:
		if n.Type != types.TColor || !hexColor.MatchString(v.Text) {
			bad("")
			return
		}
		t.vals[n.Name] = ir.ThemeValue{Name: n.Name, Type: types.TColor, V: strings.ToUpper(v.Text)}
	case *parse.Number:
		x, err := strconv.ParseFloat(v.Text, 64)
		if n.Type != types.TNumber || err != nil || !n.AcceptsNumber(x) {
			bad("")
			return
		}
		t.vals[n.Name] = ir.ThemeValue{Name: n.Name, Type: types.TNumber, V: x}
	case *parse.Text:
		s := unquote(v.Raw)
		if n.Type != types.TText || !n.AcceptsText(s) {
			bad("")
			return
		}
		t.vals[n.Name] = ir.ThemeValue{Name: n.Name, Type: types.TText, V: s}
	default:
		bad("")
	}
}

// missing reports the first name of the table that the theme does not define.
func (t *themeCheck) missing() {
	var miss []string
	for _, n := range stdlib.ThemeNames {
		if t.first[n.Name] == nil {
			miss = append(miss, n.Name)
		}
	}
	if len(miss) == 0 || len(t.rp.Files) == 0 {
		return
	}
	f := t.rp.Files[0]
	sp := source.At(source.Pos{Line: 1, Col: 1})
	if h := f.Load.Tree.Header; h != nil {
		sp = parse.CommentSpan(h)
	}
	more := []source.Extra{source.Note("copy std/theme/default and change its values")}
	if len(miss) > 1 {
		rest := miss[1:]
		list := strings.Join(rest, ", ")
		if len(rest) > 4 {
			list = strings.Join(rest[:4], ", ") + ", ..."
		}
		more = append([]source.Extra{source.Note(strconv.Itoa(len(miss)) + " names are missing: " + list)}, more...)
	}
	t.err(f, sp, `theme "`+t.rp.Name+`" does not define "`+miss[0]+`"`, "", more...)
}

// unquote is the text a text literal denotes (the same rule as the
// resolver's).
func unquote(raw string) string {
	s := raw
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		s = s[1 : len(s)-1]
	}
	if !strings.Contains(s, `\`) {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) {
			i++
			if s[i] == 'n' {
				b.WriteByte('\n')
			} else {
				b.WriteByte(s[i])
			}
			continue
		}
		b.WriteByte(s[i])
	}
	return b.String()
}
