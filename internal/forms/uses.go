package forms

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/synodlang/compiler/internal/check"
	"github.com/synodlang/compiler/internal/driver"
	"github.com/synodlang/compiler/internal/parse"
	"github.com/synodlang/compiler/internal/resolve"
	"github.com/synodlang/compiler/internal/source"
	"github.com/synodlang/compiler/internal/stdlib"
	"github.com/synodlang/compiler/internal/types"
)

// Uses is what the packages of a program use.
type Uses struct {
	// Items holds the catalog keys ("form:main", "type:list") that occur.
	Items map[string]bool
	// Phrases holds the std phrases called, or matched in a when, keyed by
	// their `synod doc` pattern ("net.Send [m: msg] from [a: node] to [b: node]").
	Phrases map[string]bool
}

func newUses() *Uses { return &Uses{Items: map[string]bool{}, Phrases: map[string]bool{}} }

// Merge adds the uses of o.
func (u *Uses) Merge(o *Uses) {
	for k := range o.Items {
		u.Items[k] = true
	}
	for k := range o.Phrases {
		u.Phrases[k] = true
	}
}

// Keys returns the sorted catalog keys.
func (u *Uses) Keys() []string { return sortedKeys(u.Items) }

// PhraseList returns the sorted patterns.
func (u *Uses) PhraseList() []string { return sortedKeys(u.Phrases) }

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// AnalyzeDir checks the folder dir (a module root, or a folder of one main
// file) the way `synod check .` does and reports what its packages use. The
// diagnostics are those of the check; Uses is nil when there are any.
func AnalyzeDir(dir string) (*Uses, []source.Diagnostic, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, nil, err
	}
	var targets []string
	err = filepath.WalkDir(abs, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		name := d.Name()
		if d.IsDir() {
			if path != abs && (strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_") || name == "testdata" || name == "vendor" || name == "out") {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(name, ".synod") {
			t := filepath.ToSlash(filepath.Dir(path))
			if len(targets) == 0 || targets[len(targets)-1] != t {
				targets = append(targets, t)
			}
		}
		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	if len(targets) == 0 {
		return nil, nil, &os.PathError{Op: "analyze", Path: dir, Err: os.ErrNotExist}
	}
	res := driver.Check(driver.Options{}, targets)
	if len(res.Diags) > 0 {
		return nil, res.Diags, nil
	}
	return Analyze(res), nil, nil
}

// Analyze walks the target packages of res (not std, not the modules the
// targets require): the parse trees for comments, blocks, parentheses and
// continuation lines, the resolved bodies for every other form, the types
// the checker gave, and the contexts of the entries.
func Analyze(res driver.Result) *Uses {
	u := newUses()
	if res.Graph == nil {
		return u
	}
	for _, t := range res.Graph.Targets {
		rp := res.ByPath[t.Path]
		if rp == nil || rp.IsStd {
			continue
		}
		c := &collector{u: u, info: res.Infos[rp.Path]}
		c.pkg(rp)
	}
	return u
}

type collector struct {
	u    *Uses
	info *check.Info
}

func (c *collector) add(k Kind, id string) { c.u.Items[Key(k, id)] = true }
func (c *collector) form(id string)        { c.add(KindForm, id) }

func (c *collector) phrase(ph *resolve.Phrase) {
	if ph != nil && ph.Pkg != nil && ph.Pkg.IsStd && ph.Exported {
		c.u.Phrases[ph.DescribeAs(ph.Pkg.Name, "", nil)] = true
	}
}

// typ records the types a type term is made of.
func (c *collector) typ(t types.Type) {
	switch x := t.(type) {
	case types.Basic:
		if x.K != types.Invalid {
			c.add(KindType, x.K.String())
		}
	case types.NodeOf:
		c.add(KindType, "Node")
	case types.Record:
		c.add(KindType, "record")
	case types.List:
		c.add(KindType, "list")
		c.typ(x.Elem)
	case types.Map:
		c.add(KindType, "map")
		c.typ(x.Key)
		c.typ(x.Val)
	}
}

func (c *collector) pkg(rp *resolve.Package) {
	for _, f := range rp.Files {
		if f.Load.Tree != nil {
			c.tree(f.Load.Tree)
		}
	}
	for _, imp := range rp.Imports {
		if imp.Imp.Alias != "" {
			c.form("use-as")
		} else {
			c.form("use")
		}
		if strings.HasPrefix(imp.Imp.Path, "std/") {
			c.add(KindPackage, imp.Imp.Path)
		}
	}
	if n := rp.Node; n != nil && n.File != nil {
		c.form("has-node")
		c.fields(n)
	}
	for _, r := range rp.Records {
		if r.Name == "" {
			continue
		}
		c.form("has-record")
		c.add(KindType, "record")
		c.fields(r)
	}
	for _, ph := range rp.Phrases {
		c.definition(ph)
	}
	for _, h := range rp.Handlers {
		c.handler(h)
	}
	for _, inv := range rp.Invariants {
		c.form("always")
		c.add(KindContext, "check")
		c.expr(inv.Cond)
	}
	if c.info != nil {
		for _, t := range c.info.LocalTy {
			c.typ(t)
		}
	}
}

func (c *collector) fields(r *resolve.Record) {
	for _, f := range r.Fields {
		if c.info != nil {
			c.typ(c.info.FieldTy[f])
		}
		c.expr(f.Init)
	}
}

func (c *collector) definition(ph *resolve.Phrase) {
	if ph.IsEvent {
		return
	}
	switch ph.Kind {
	case resolve.PhraseMain:
		c.form("main")
		c.add(KindContext, "main")
	case resolve.PhraseStart:
		c.form("start")
		c.add(KindContext, "node")
	default:
		if ph.Native != "" {
			c.form("define-native")
		} else {
			c.form("define")
		}
		if ph.Receiver != nil {
			c.form("pattern-receiver")
			c.add(KindContext, "node")
		}
		if len(ph.Params) > 0 {
			c.form("pattern-hole")
		}
		if len(ph.Blocks) > 0 {
			c.form("pattern-block")
		}
	}
	if ph.Receiver != nil {
		c.typ(ph.Receiver.Type)
	}
	for _, p := range ph.Params {
		c.typ(p.Type)
	}
	for _, b := range ph.Blocks {
		c.typ(b.Param.Type)
	}
	if c.info != nil {
		c.typ(c.info.ResultTy[ph])
	}
	c.block(ph.Body)
}

func (c *collector) handler(h *resolve.Handler) {
	switch h.Kind {
	case stdlib.NodeEvent:
		c.form("when-node")
		c.add(KindContext, "node")
	case stdlib.TraceEvent:
		c.form("when-trace")
		c.add(KindContext, "look")
	}
	c.phrase(h.Event)
	for _, m := range h.Matchers {
		switch m.Kind {
		case resolve.MatchNodeType:
			c.form("when-typed")
			c.add(KindType, "Node")
		case resolve.MatchRecord:
			c.add(KindType, "record")
		case resolve.MatchField:
			c.add(KindType, "field")
		case resolve.MatchEqual:
			c.expr(m.Value)
		case resolve.MatchBind:
			if m.Binding != nil {
				if b, ok := m.Binding.Type.(types.Basic); ok && b.K == types.Field {
					c.typ(b)
				}
			}
		}
	}
	c.block(h.Body)
}

func (c *collector) block(b *resolve.Block) {
	if b == nil {
		return
	}
	for _, s := range b.Stmts {
		c.stmt(s)
	}
}

func (c *collector) stmt(s resolve.Stmt) {
	switch x := s.(type) {
	case *resolve.Set:
		c.place(x.Place)
		c.expr(x.Value)
	case *resolve.CallStmt:
		c.call(x.Call)
	case *resolve.RunBlock:
		c.form("run-block")
		c.add(KindType, "block")
	case *resolve.If:
		c.form("if")
		c.expr(x.Cond)
		c.block(x.Then)
		if x.Else != nil {
			c.form("else")
			c.block(x.Else)
		}
	case *resolve.Each:
		if x.B != nil {
			c.form("each-pair")
		} else {
			c.form("each")
		}
		c.expr(x.Over)
		c.block(x.Body)
	case *resolve.After:
		c.form("after")
		c.expr(x.Delay)
		c.block(x.Body)
	case *resolve.Deliver:
		c.form("deliver")
		c.expr(x.Rec)
		c.expr(x.To)
	case *resolve.Emit:
		c.form("emit")
		c.expr(x.Rec)
		c.expr(x.By)
	case *resolve.Result:
		c.form("result")
		c.expr(x.Value)
	case *resolve.Draw:
		c.form("draw")
		c.expr(x.Shape)
		if x.For != nil {
			c.form("draw-for")
			c.expr(x.For)
		}
		if x.Body != nil {
			c.form("draw-block")
			c.block(x.Body)
		}
	case *resolve.ExprStmt:
		c.expr(x.X)
	}
}

func (c *collector) place(p resolve.Place) {
	switch x := p.(type) {
	case *resolve.LocalPlace:
		c.form("set-local")
	case *resolve.SelectPlace:
		c.form("set-field")
		c.expr(x.Select)
	case *resolve.ItemPlace:
		c.form("set-item")
		c.place(x.Base)
		c.expr(x.Key)
	}
}

var binaryForms = map[string]string{
	"is": "is", "is not": "is-not", "is in": "is-in", "is not in": "is-not-in",
	"less than": "less-than", "more than": "more-than", "at least": "at-least", "at most": "at-most",
	"and": "and", "or": "or", "plus": "plus", "minus": "minus", "times": "times", "divided by": "divided-by",
}

func (c *collector) call(x *resolve.Call) {
	c.phrase(x.Phrase)
	if x.Receiver != nil {
		c.form("member")
		c.expr(x.Receiver)
	}
	for _, a := range x.Args {
		c.expr(a)
	}
	for _, b := range x.Blocks {
		c.block(b)
	}
	c.typ(x.Type())
}

func (c *collector) expr(e resolve.Expr) {
	if e == nil {
		return
	}
	c.typ(e.Type())
	switch x := e.(type) {
	case *resolve.Lit:
		switch x.Kind {
		case resolve.LitText:
			c.form("value-text")
		case resolve.LitNumber:
			c.form("value-number")
		case resolve.LitBool:
			c.form("value-bool")
		case resolve.LitTime:
			c.form("value-duration")
		}
	case *resolve.ColorLit:
		c.form("value-color")
	case *resolve.EmptyLit:
		c.form("empty")
	case *resolve.ListLit:
		c.form("list")
		for _, it := range x.Items {
			c.expr(it)
		}
	case *resolve.Select:
		c.form("member")
		c.expr(x.Base)
	case *resolve.Call:
		c.call(x)
	case *resolve.RecordLit:
		if f := strings.Fields(x.Text); len(f) > 1 && strings.Contains(f[1], ".") {
			c.form("message-qualified")
		} else {
			c.form("message")
		}
		for _, s := range x.Settings {
			c.expr(s.Value)
		}
	case *resolve.Item:
		c.form("item")
		c.expr(x.Coll)
		c.expr(x.Key)
	case *resolve.Binary:
		if id := binaryForms[x.Op]; id != "" {
			c.form(id)
		}
		c.expr(x.A)
		c.expr(x.B)
	case *resolve.Not:
		c.form("not")
		c.expr(x.X)
	case *resolve.Attachment:
		c.form("attachment")
		c.expr(x.Key)
	case *resolve.ActionExpr:
		c.stmt(x.Stmt)
	}
}

// ---- parse tree ----

func (c *collector) tree(t *parse.File) {
	if t.Header != nil {
		c.form("header")
	}
	if len(t.Trailing) > 0 {
		c.form("comment")
	}
	for _, n := range t.Body {
		switch x := n.(type) {
		case *parse.CommentGroup:
			c.form("comment")
		case *parse.Line:
			c.line(x)
		}
	}
}

func (c *collector) line(l *parse.Line) {
	if len(l.Leading) > 0 || l.Comment != nil {
		c.form("comment")
	}
	c.sentence(l.Sentence)
	if l.Block != nil {
		c.parseBlock(l.Block)
	}
	for _, k := range l.Conts {
		if len(k.Leading) > 0 || k.Comment != nil {
			c.form("comment")
		}
		if k.Word.Name != "else" {
			c.form("continuation")
		}
		c.parseBlock(k.Block)
	}
}

func (c *collector) parseBlock(b *parse.Block) {
	c.form("block")
	if len(b.Trailing) > 0 {
		c.form("comment")
	}
	for _, l := range b.Lines {
		c.line(l)
	}
}

func (c *collector) sentence(s *parse.Sentence) {
	for _, g := range s.Groups {
		for _, it := range g {
			c.item(it)
		}
	}
}

func (c *collector) item(it parse.Item) {
	switch x := it.(type) {
	case *parse.Paren:
		c.form("paren")
		c.sentence(x.S)
	case *parse.List:
		for _, e := range x.Items {
			c.item(e)
		}
	}
}
