package check

import (
	"github.com/synodlang/compiler/internal/ir"
	"github.com/synodlang/compiler/internal/resolve"
	"github.com/synodlang/compiler/internal/types"
)

func (f *fnc) block(b *resolve.Block) *ir.Block {
	out := &ir.Block{}
	if b == nil {
		return out
	}
	for _, s := range b.Stmts {
		if st := f.stmt(s); st != nil {
			out.Stmts = append(out.Stmts, st)
		}
	}
	return out
}

// local returns the variable of a local, creating its slot on first use.
func (f *fnc) local(l *resolve.Local) *ir.Var {
	if v := f.by[l]; v != nil {
		return v
	}
	kind := ir.VarLocal
	switch l.Kind {
	case resolve.LocalEach:
		kind = ir.VarEach
	case resolve.LocalDraw:
		kind = ir.VarDraw
	}
	t := f.info.LocalTy[l]
	if t == nil {
		t = types.TInvalid
	}
	return f.newVar(l, l.Name, kind, t)
}

func (f *fnc) symVar(s resolve.Symbol) *ir.Var {
	switch x := s.(type) {
	case *resolve.Local:
		return f.local(x)
	}
	if v := f.by[s]; v != nil {
		return v
	}
	// A binding or parameter of an enclosing body that was not declared in
	// this frame cannot happen after a clean check.
	return f.newVar(s, s.SymName(), ir.VarLocal, types.TInvalid)
}

func (f *fnc) stmt(s resolve.Stmt) ir.Stmt {
	at := ir.StmtPos{At: f.span(s.Span())}
	switch x := s.(type) {
	case *resolve.Set:
		return f.set(at, x)
	case *resolve.CallStmt:
		return &ir.CallStmt{StmtPos: at, Call: f.call(x.Call)}
	case *resolve.RunBlock:
		return &ir.RunBlock{StmtPos: at, Var: f.symVar(x.Param)}
	case *resolve.If:
		st := &ir.If{StmtPos: at, Cond: f.expr(x.Cond), Then: f.block(x.Then)}
		if x.Else != nil {
			st.Else = f.block(x.Else)
		}
		return st
	case *resolve.Each:
		st := &ir.Each{StmtPos: at, Over: f.expr(x.Over)}
		_, st.IsMap = x.Over.Type().(types.Map)
		// The loop variables are declared before the body.
		switch {
		case x.B != nil:
			st.Key, st.Val = f.local(x.A), f.local(x.B)
		case st.IsMap:
			st.Key = f.local(x.A)
		default:
			st.Val = f.local(x.A)
		}
		st.Body = f.block(x.Body)
		return st
	case *resolve.After:
		return &ir.After{StmtPos: at, Delay: f.expr(x.Delay), Body: f.block(x.Body)}
	case *resolve.Deliver:
		return &ir.Deliver{StmtPos: at, Rec: f.expr(x.Rec), To: f.expr(x.To)}
	case *resolve.Emit:
		return &ir.Emit{StmtPos: at, Rec: f.expr(x.Rec), By: f.expr(x.By)}
	case *resolve.Result:
		return &ir.Result{StmtPos: at, X: f.expr(x.Value)}
	case *resolve.Draw:
		st := &ir.Draw{StmtPos: at, Shape: f.expr(x.Shape), Rebind: x.Rebind}
		st.Var = f.local(x.Name)
		if x.For != nil {
			st.For = f.expr(x.For)
			st.Name = x.Name.Name
		}
		if x.Body != nil {
			st.Reactive = f.block(x.Body)
		}
		return st
	}
	return nil
}

// selectPrefix lowers a select up to (not including) step upto.
func (f *fnc) selectPrefix(sel *resolve.Select, upto int) ir.Expr {
	cur := f.expr(sel.Base)
	si := f.info.Sel[sel]
	for i := 0; i < upto && i < len(si.Steps); i++ {
		st := si.Steps[i]
		b := ir.Base{Ty: st.Type, At: f.span(st.Sp)}
		switch st.Kind {
		case StepNodeField:
			cur = &ir.NodeField{Base: b, Node: cur, Field: f.l.field[st.Field]}
		case StepRecordField:
			cur = &ir.RecordField{Base: b, Rec: cur, Field: f.l.field[st.Field]}
		case StepRecvPhrase:
			cur = &ir.Call{Base: b, Proc: f.l.proc[st.Phrase], Recv: cur}
		}
	}
	return cur
}

// place lowers the place of a set; its node field is the last step of a
// select.
func (f *fnc) place(p resolve.Place) ir.Place {
	switch x := p.(type) {
	case *resolve.LocalPlace:
		return &ir.VarPlace{Var: f.local(x.Local)}
	case *resolve.SelectPlace:
		si := f.info.Sel[x.Select]
		last := si.Steps[len(si.Steps)-1]
		return &ir.FieldPlace{Node: f.selectPrefix(x.Select, len(si.Steps)-1), Field: f.l.field[last.Field]}
	}
	return nil
}

func (f *fnc) set(at ir.StmtPos, x *resolve.Set) ir.Stmt {
	switch pl := x.Place.(type) {
	case *resolve.LocalPlace:
		return &ir.SetLocal{StmtPos: at, Var: f.local(pl.Local), X: f.expr(x.Value)}
	case *resolve.SelectPlace:
		fp := f.place(pl).(*ir.FieldPlace)
		return &ir.SetField{StmtPos: at, Node: fp.Node, Field: fp.Field, X: f.expr(x.Value)}
	case *resolve.ItemPlace:
		base := f.place(pl.Base)
		_, isMap := base.Type().(types.Map)
		return &ir.SetItem{StmtPos: at, Base: base, Key: f.expr(pl.Key), X: f.expr(x.Value), IsMap: isMap}
	}
	return nil
}

func (f *fnc) base(e resolve.Expr) ir.Base {
	t := e.Type()
	if t == nil {
		t = types.TInvalid
	}
	return ir.Base{Ty: t, At: f.span(e.Span())}
}

func (f *fnc) call(e *resolve.Call) *ir.Call {
	c := &ir.Call{Base: f.base(e), Proc: f.l.proc[e.Phrase]}
	if e.Receiver != nil {
		c.Recv = f.expr(e.Receiver)
	}
	for _, a := range e.Args {
		c.Args = append(c.Args, f.expr(a))
	}
	for _, b := range e.Blocks {
		c.Blocks = append(c.Blocks, f.block(b))
	}
	return c
}

func (f *fnc) expr(e resolve.Expr) ir.Expr {
	b := f.base(e)
	switch x := e.(type) {
	case *resolve.Lit:
		c := &ir.Const{Base: b}
		switch x.Kind {
		case resolve.LitText:
			c.V = x.Str
		case resolve.LitNumber:
			c.V = x.Num
		case resolve.LitBool:
			c.V = x.Bool
		case resolve.LitTime:
			c.V = x.Micros
		}
		return c
	case *resolve.ColorLit:
		return &ir.Const{Base: b, V: x.Hex}
	case *resolve.Ref:
		return &ir.Load{Base: b, Var: f.symVar(x.Sym)}
	case *resolve.PkgRef:
		return &ir.PackageVal{Base: b, Pkg: f.l.pk[x.Pkg]}
	case *resolve.FieldRef:
		return &ir.FieldVal{Base: b, Field: f.l.field[x.Field]}
	case *resolve.EmptyLit:
		return &ir.Empty{Base: b}
	case *resolve.ListLit:
		out := &ir.ListLit{Base: b}
		for _, it := range x.Items {
			out.Items = append(out.Items, f.expr(it))
		}
		return out
	case *resolve.Select:
		si := f.info.Sel[x]
		return f.selectPrefix(x, len(si.Steps))
	case *resolve.Call:
		return f.call(x)
	case *resolve.RecordLit:
		out := &ir.MakeRecord{Base: b, Record: f.l.rec[x.Record], Values: make([]ir.Expr, len(x.Record.Fields))}
		for _, st := range x.Settings {
			out.Values[st.Field.Index] = f.expr(st.Value)
		}
		return out
	case *resolve.Item:
		_, isMap := x.Coll.Type().(types.Map)
		return &ir.Item{Base: b, Coll: f.expr(x.Coll), Key: f.expr(x.Key), IsMap: isMap}
	case *resolve.Not:
		return &ir.Not{Base: b, X: f.expr(x.X)}
	case *resolve.Attachment:
		return &ir.Attachment{Base: b, Name: x.Name, Key: f.expr(x.Key)}
	case *resolve.Binary:
		return f.binary(x, b)
	}
	return &ir.Const{Base: b}
}

func (f *fnc) binary(x *resolve.Binary, b ir.Base) ir.Expr {
	a, c := f.expr(x.A), f.expr(x.B)
	switch x.Op {
	case "plus":
		return &ir.Arith{Base: b, Op: ir.Plus, A: a, B: c}
	case "minus":
		return &ir.Arith{Base: b, Op: ir.Minus, A: a, B: c}
	case "times":
		return &ir.Arith{Base: b, Op: ir.Times, A: a, B: c}
	case "divided by":
		return &ir.Arith{Base: b, Op: ir.Divide, A: a, B: c}
	case "is":
		return &ir.Compare{Base: b, Op: ir.Eq, A: a, B: c}
	case "is not":
		return &ir.Compare{Base: b, Op: ir.Ne, A: a, B: c}
	case "less than":
		return &ir.Compare{Base: b, Op: ir.Lt, A: a, B: c}
	case "more than":
		return &ir.Compare{Base: b, Op: ir.Gt, A: a, B: c}
	case "at most":
		return &ir.Compare{Base: b, Op: ir.Le, A: a, B: c}
	case "at least":
		return &ir.Compare{Base: b, Op: ir.Ge, A: a, B: c}
	case "and":
		return &ir.Logic{Base: b, And: true, A: a, B: c}
	case "or":
		return &ir.Logic{Base: b, A: a, B: c}
	case "is in", "is not in":
		_, isMap := x.B.Type().(types.Map)
		return &ir.Contains{Base: b, Elem: a, Coll: c, IsMap: isMap, Not: x.Op == "is not in"}
	}
	return &ir.Const{Base: b}
}
