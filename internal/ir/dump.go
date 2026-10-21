package ir

import (
	"strconv"
	"strings"

	"github.com/synodlang/compiler/internal/types"
)

// Dump prints p as a deterministic S-expression. It is the form of the IR
// goldens: one statement per line, expressions inline, variables as
// name#slot. Native phrases are not listed (a call names its native id);
// the phrases of std that are written in Synod are.
func Dump(p *Program) string {
	d := &dumper{}
	d.program(p)
	return d.b.String()
}

type dumper struct {
	b      strings.Builder
	indent int
}

func (d *dumper) nl() {
	d.b.WriteByte('\n')
	for i := 0; i < d.indent; i++ {
		d.b.WriteString("  ")
	}
}

func (d *dumper) w(s string) { d.b.WriteString(s) }

func q(s string) string { return strconv.Quote(s) }

func ty(t types.Type) string {
	if t == nil {
		return "none"
	}
	return types.Format(t, "", nil)
}

func vr(v *Var) string {
	if v == nil {
		return "_"
	}
	return v.Name + "#" + strconv.Itoa(v.Slot)
}

func (d *dumper) program(p *Program) {
	d.w("(program " + q(p.Name))
	d.indent++
	d.nl()
	d.w("(packages")
	for _, k := range p.Packages {
		d.w(" " + q(k.Path))
	}
	d.w(")")
	if p.Main != nil {
		d.nl()
		d.proc(p.Main, "main")
	}
	for _, k := range p.Packages {
		d.nl()
		d.pkg(k)
	}
	d.indent--
	d.w(")\n")
}

func (d *dumper) pkg(k *Package) {
	d.w("(package " + q(k.Path))
	if k.IsStd {
		d.w(" std")
	}
	if k.IsMain {
		d.w(" main")
	}
	d.indent++
	list := func(name string, ps []*Package) {
		if len(ps) == 0 {
			return
		}
		d.nl()
		d.w("(" + name)
		for _, p := range ps {
			d.w(" " + q(p.Path))
		}
		d.w(")")
	}
	list("uses", k.Uses)
	list("runs", k.Runs)
	if len(k.NodeFields) > 0 {
		d.nl()
		d.w("(node-fields")
		d.indent++
		for _, f := range k.NodeFields {
			d.nl()
			d.field(f)
		}
		d.indent--
		d.w(")")
	}
	for _, r := range k.Records {
		d.nl()
		d.w("(record " + r.Name)
		if r.Exported {
			d.w(" exported")
		}
		d.indent++
		for _, f := range r.Fields {
			d.nl()
			d.field(f)
		}
		d.indent--
		d.w(")")
	}
	if k.Start != nil {
		d.nl()
		d.proc(k.Start, "start")
	}
	for _, p := range k.Phrases {
		if p.Kind == ProcNative {
			continue
		}
		d.nl()
		d.proc(p, "phrase")
	}
	for _, h := range k.NodeHandlers {
		d.nl()
		d.handler(h, "when-node")
	}
	for _, h := range k.Observers {
		d.nl()
		d.handler(h, "when-trace")
	}
	for _, inv := range k.Invariants {
		d.nl()
		d.w("(always " + q(inv.Rule) + " ")
		d.expr(inv.Cond)
		d.w(" (slots " + strconv.Itoa(inv.Slots) + "))")
	}
	d.indent--
	d.w(")")
}

func (d *dumper) field(f *Field) {
	d.w("(field " + f.Name + " " + ty(f.Type))
	if f.Exported {
		d.w(" exported")
	}
	if f.Init != nil {
		d.w(" ")
		d.expr(f.Init)
	}
	d.w(")")
}

func (d *dumper) vars(vs []*Var) {
	d.nl()
	d.w("(vars")
	for _, v := range vs {
		kind := [...]string{"receiver", "param", "block", "local", "each", "draw", "binding"}[v.Kind]
		d.w(" (" + strconv.Itoa(v.Slot) + " " + v.Name + " " + kind)
		if v.Type != nil {
			d.w(" " + ty(v.Type))
		}
		d.w(")")
	}
	d.w(")")
}

func (d *dumper) proc(p *Proc, kind string) {
	d.w("(" + kind + " " + q(p.Name))
	if p.Result != nil {
		d.w(" (result " + ty(p.Result) + ")")
	}
	d.indent++
	d.vars(p.Vars)
	d.nl()
	d.w("(body")
	d.block(p.Body)
	d.w(")")
	d.indent--
	d.w(")")
}

func (d *dumper) handler(h *Handler, kind string) {
	d.w("(" + kind + " " + q(h.Event))
	d.indent++
	d.vars(h.Vars)
	for _, m := range h.Match {
		d.nl()
		d.w("(match " + strconv.Itoa(m.Hole) + " ")
		switch m.Kind {
		case MatchBind:
			d.w("bind " + vr(m.Var))
		case MatchRecord:
			d.w("record " + m.Record.Pkg.Name + "." + m.Record.Name + " " + vr(m.Var))
		case MatchNodeType:
			d.w("node " + q(m.NodePkg.Path) + " " + vr(m.Var))
		case MatchField:
			d.w("field " + m.Field.Pkg.Name + "." + m.Field.Name)
		case MatchEqual:
			d.w("equal ")
			d.expr(m.Value)
		}
		d.w(")")
	}
	d.nl()
	d.w("(body")
	d.block(h.Body)
	d.w(")")
	d.indent--
	d.w(")")
}

// block writes the statements of b, one per line, indented one level.
func (d *dumper) block(b *Block) {
	if b == nil {
		return
	}
	d.indent++
	for _, s := range b.Stmts {
		d.nl()
		d.stmt(s)
	}
	d.indent--
}

func (d *dumper) place(p Place) {
	switch x := p.(type) {
	case *VarPlace:
		d.w(vr(x.Var))
	case *FieldPlace:
		d.w("(nodefield ")
		d.expr(x.Node)
		d.w(" " + x.Field.Pkg.Name + "." + x.Field.Name + ")")
	}
}

func (d *dumper) stmt(s Stmt) {
	switch x := s.(type) {
	case *SetLocal:
		d.w("(set " + vr(x.Var) + " ")
		d.expr(x.X)
		d.w(")")
	case *SetField:
		d.w("(set-field ")
		d.expr(x.Node)
		d.w(" " + x.Field.Pkg.Name + "." + x.Field.Name + " ")
		d.expr(x.X)
		d.w(")")
	case *SetItem:
		d.w("(set-item ")
		d.place(x.Base)
		d.w(" ")
		d.expr(x.Key)
		d.w(" ")
		d.expr(x.X)
		if x.IsMap {
			d.w(" map")
		}
		d.w(")")
	case *CallStmt:
		d.expr(x.Call)
	case *RunBlock:
		d.w("(run-block " + vr(x.Var) + ")")
	case *If:
		d.w("(if ")
		d.expr(x.Cond)
		d.indent++
		d.nl()
		d.w("(then")
		d.block(x.Then)
		d.w(")")
		if x.Else != nil {
			d.nl()
			d.w("(else")
			d.block(x.Else)
			d.w(")")
		}
		d.indent--
		d.w(")")
	case *Each:
		d.w("(each " + vr(x.Key) + " " + vr(x.Val) + " ")
		d.expr(x.Over)
		if x.IsMap {
			d.w(" map")
		}
		d.block(x.Body)
		d.w(")")
	case *After:
		d.w("(after ")
		d.expr(x.Delay)
		d.block(x.Body)
		d.w(")")
	case *Deliver:
		d.w("(deliver ")
		d.expr(x.Rec)
		d.w(" ")
		d.expr(x.To)
		d.w(")")
	case *Emit:
		d.w("(emit ")
		d.expr(x.Rec)
		d.w(" ")
		d.expr(x.By)
		d.w(")")
	case *Result:
		d.w("(result ")
		d.expr(x.X)
		d.w(")")
	case *Draw:
		d.w("(draw ")
		d.expr(x.Shape)
		d.w(" " + vr(x.Var))
		if x.For != nil {
			d.w(" (for " + q(x.Name) + " ")
			d.expr(x.For)
			d.w(")")
		}
		if x.Rebind {
			d.w(" rebind")
		}
		if x.Reactive != nil {
			d.indent++
			d.nl()
			d.w("(reactive")
			d.block(x.Reactive)
			d.w(")")
			d.indent--
		}
		d.w(")")
	}
}

var arithNames = [...]string{"plus", "minus", "times", "divided-by"}
var cmpNames = [...]string{"is", "is-not", "less-than", "more-than", "at-most", "at-least"}

func (d *dumper) exprs(es []Expr) {
	for _, e := range es {
		d.w(" ")
		d.expr(e)
	}
}

func (d *dumper) expr(e Expr) {
	switch x := e.(type) {
	case nil:
		d.w("_")
	case *Const:
		switch v := x.V.(type) {
		case string:
			if _, isColor := x.Ty.(types.Basic); isColor && x.Ty == types.TColor {
				d.w(v)
			} else {
				d.w(q(v))
			}
		case float64:
			d.w(strconv.FormatFloat(v, 'f', -1, 64))
		case bool:
			d.w(strconv.FormatBool(v))
		case int64:
			d.w("(time " + strconv.FormatInt(v, 10) + ")")
		default:
			d.w("?")
		}
	case *Empty:
		d.w("(empty " + ty(x.Ty) + ")")
	case *ListLit:
		d.w("(list")
		d.exprs(x.Items)
		d.w(")")
	case *Load:
		d.w(vr(x.Var))
	case *NodeField:
		d.w("(nodefield ")
		d.expr(x.Node)
		d.w(" " + x.Field.Pkg.Name + "." + x.Field.Name + ")")
	case *RecordField:
		d.w("(recfield ")
		d.expr(x.Rec)
		d.w(" " + x.Field.Name + ")")
	case *MakeRecord:
		d.w("(message " + x.Record.Pkg.Name + "." + x.Record.Name)
		for i, v := range x.Values {
			if v == nil {
				continue
			}
			d.w(" (" + x.Record.Fields[i].Name + " ")
			d.expr(v)
			d.w(")")
		}
		d.w(")")
	case *Item:
		d.w("(item ")
		d.expr(x.Coll)
		d.w(" ")
		d.expr(x.Key)
		if x.IsMap {
			d.w(" map")
		}
		d.w(")")
	case *Arith:
		name := arithNames[x.Op]
		if !types.Equal(x.Ty, types.TNumber) {
			name += ":" + ty(x.Ty)
		}
		d.w("(" + name + " ")
		d.expr(x.A)
		d.w(" ")
		d.expr(x.B)
		d.w(")")
	case *Compare:
		d.w("(" + cmpNames[x.Op] + " ")
		d.expr(x.A)
		d.w(" ")
		d.expr(x.B)
		d.w(")")
	case *Contains:
		name := "in"
		if x.Not {
			name = "not-in"
		}
		d.w("(" + name + " ")
		d.expr(x.Elem)
		d.w(" ")
		d.expr(x.Coll)
		if x.IsMap {
			d.w(" map")
		}
		d.w(")")
	case *Logic:
		name := "or"
		if x.And {
			name = "and"
		}
		d.w("(" + name + " ")
		d.expr(x.A)
		d.w(" ")
		d.expr(x.B)
		d.w(")")
	case *Not:
		d.w("(not ")
		d.expr(x.X)
		d.w(")")
	case *Call:
		if x.Proc.Native != "" {
			d.w("(native " + q(x.Proc.Native))
		} else {
			d.w("(call " + q(x.Proc.Pkg.Name+"."+x.Proc.Name))
		}
		if x.Recv != nil {
			d.w(" (recv ")
			d.expr(x.Recv)
			d.w(")")
		}
		d.exprs(x.Args)
		if len(x.Blocks) > 0 {
			d.indent++
			for _, b := range x.Blocks {
				d.nl()
				d.w("(block")
				d.block(b)
				d.w(")")
			}
			d.indent--
		}
		d.w(")")
	case *PackageVal:
		d.w("(package " + q(x.Pkg.Path) + ")")
	case *FieldVal:
		d.w("(field-ref " + x.Field.Pkg.Name + "." + x.Field.Name + ")")
	case *Attachment:
		d.w("(attached " + q(x.Name) + " ")
		d.expr(x.Key)
		d.w(")")
	default:
		d.w("?")
	}
}
