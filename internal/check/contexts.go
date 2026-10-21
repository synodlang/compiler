package check

import (
	"strconv"
	"strings"

	"github.com/synodlang/compiler/internal/resolve"
	"github.com/synodlang/compiler/internal/source"
	"github.com/synodlang/compiler/internal/stdlib"
)

// Contexts checks, over the whole graph, that every statement stands in a
// context that allows it, and that node code reaches only its receiver's
// state (spec/05 section 5.4). It starts at the entries (define main, start,
// receiver phrases, whens, always) and follows calls of phrases without a
// receiver, once per (phrase, context) pair. The bodies of std phrases are
// not walked: their catalog contexts are enforced at the call site.
func Contexts(all []*resolve.Package, infos map[string]*Info) []source.Diagnostic {
	w := &ctxWalker{infos: infos, seen: map[string]bool{}, queued: map[qkey]bool{}}
	for _, rp := range all {
		w.info = infos[rp.Path]
		if w.info == nil {
			continue
		}
		if !rp.IsStd {
			for _, ph := range rp.Phrases {
				if ph.Body == nil {
					continue
				}
				switch {
				case ph.Kind == resolve.PhraseMain:
					w.entry(ph.File, stdlib.Main, nil, ph.Body)
				case ph.Kind == resolve.PhraseStart || ph.Receiver != nil:
					if ph.Receiver != nil && isInvalid(ph.Receiver.Type) {
						continue // bad-receiver was reported
					}
					w.entry(ph.File, stdlib.Node, ph.Receiver, ph.Body)
				}
			}
		}
		for _, h := range rp.Handlers {
			if h.Body == nil {
				continue
			}
			if h.Receiver != nil && isInvalid(h.Receiver.Type) || rp.IsMain && h.Kind == stdlib.NodeEvent {
				continue // bad-receiver was reported
			}
			w.file = h.File
			w.ctx, w.recv = stdlib.Look, nil
			w.clearCause()
			if h.Kind == stdlib.NodeEvent {
				w.ctx = stdlib.Node
				if h.Receiver != nil {
					w.recv = h.Receiver
				}
			}
			for _, m := range h.Matchers {
				if m.Kind == resolve.MatchEqual && m.Value != nil {
					w.expr(m.Value)
				}
			}
			w.block(h.Body)
		}
		for _, inv := range rp.Invariants {
			w.file = inv.File
			w.ctx, w.recv = stdlib.Check, nil
			w.clearCause()
			w.expr(inv.Cond)
		}
	}
	for i := 0; i < len(w.queue); i++ {
		q := w.queue[i]
		w.info = infos[q.ph.Pkg.Path]
		if w.info == nil {
			continue
		}
		w.file = q.ph.File
		w.ctx, w.recv = q.ctx, nil
		w.cause = `"` + q.ph.Head + `" runs in ` + ctxName(q.ctx) + ` context because ` + q.caller + ` calls it`
		w.causeNote = `"` + q.ph.Head + `" runs in ` + ctxName(q.ctx) + ` context`
		w.causeFile, w.causeSp = q.callerFile, q.callerSp
		w.causeText = "called from " + ctxName(q.ctx) + " context here"
		w.block(q.ph.Body)
	}
	return w.diags
}

type qkey struct {
	ph  *resolve.Phrase
	ctx stdlib.Ctx
}

type qitem struct {
	qkey
	caller     string
	callerFile *resolve.File
	callerSp   source.Span
}

type ctxWalker struct {
	infos map[string]*Info
	info  *Info
	diags []source.Diagnostic
	seen  map[string]bool

	queue  []qitem
	queued map[qkey]bool

	file  *resolve.File
	ctx   stdlib.Ctx
	recv  resolve.Symbol
	cause string
	// The rich form of the cause: the note, and the label at the call site.
	causeNote, causeText string
	causeFile            *resolve.File
	causeSp              source.Span
}

// clearCause forgets the cause: the walk is at an entry point.
func (w *ctxWalker) clearCause() {
	w.cause, w.causeNote, w.causeText, w.causeFile, w.causeSp = "", "", "", nil, source.Span{}
}

func ctxName(c stdlib.Ctx) string {
	switch c {
	case stdlib.Node:
		return "node"
	case stdlib.Main:
		return "main"
	case stdlib.Check:
		return "check"
	case stdlib.Look:
		return "look"
	}
	return c.String()
}

func (w *ctxWalker) entry(f *resolve.File, ctx stdlib.Ctx, recv *resolve.Param, b *resolve.Block) {
	w.file, w.ctx = f, ctx
	w.clearCause()
	w.recv = nil
	if recv != nil {
		w.recv = recv
	}
	w.block(b)
}

func (w *ctxWalker) errf(sp source.Span, code source.Code, msg, hint string, more ...source.Extra) {
	key := w.file.Path + ":" + strconv.Itoa(sp.Start.Line) + ":" + strconv.Itoa(sp.Start.Col) + ":" + string(code)
	if w.seen[key] {
		return
	}
	w.seen[key] = true
	own := hint
	if w.cause != "" {
		if hint != "" {
			hint += "; "
		}
		hint += w.cause
	}
	d := source.NewSpan(w.file.Load.Src, sp, code, msg, hint, more...)
	if w.cause != "" {
		// The note stands for the cause; the hint's own part becomes help.
		if own != "" && len(d.Help) == 0 {
			d.Help = []source.Help{{Text: own}}
		}
		if w.causeFile != nil && !strings.HasPrefix(w.causeFile.Path, "std/") && w.causeSp.Start != w.causeSp.End {
			source.Also(w.causeFile.Path, w.causeSp, w.causeText)(&d)
		}
		d.Notes = append(d.Notes, w.causeNote)
	}
	w.diags = append(w.diags, d)
}

// recvLabel is the label at the declaration of the receiver in scope.
func (w *ctxWalker) recvLabel() source.Extra {
	var sp source.Span
	switch r := w.recv.(type) {
	case *resolve.Param:
		sp = r.HoleSp
	case *resolve.Binding:
		sp = r.Sp
	}
	if sp.Start == sp.End {
		return func(*source.Diagnostic) {}
	}
	return source.Also(w.file.Path, sp, `the receiver is "`+w.recvName()+`"`)
}

// notAllowed reports a construct that the current context does not allow.
func (w *ctxWalker) notAllowed(sp source.Span, what string) {
	w.errf(sp, source.Context, `"`+what+`" is not allowed in `+ctxName(w.ctx)+` context`, "")
}

// isRecv reports that e is a read of the receiver in scope.
func (w *ctxWalker) isRecv(e resolve.Expr) bool {
	r, ok := e.(*resolve.Ref)
	return ok && w.recv != nil && r.Sym == w.recv
}

// recvName is the name of the receiver in scope, or "".
func (w *ctxWalker) recvName() string {
	if w.recv == nil {
		return ""
	}
	return w.recv.SymName()
}

func (w *ctxWalker) foreign(sp source.Span, item, via string, base resolve.Expr) {
	msg := `node code reaches "` + item + `" through "` + via + `"`
	if r := w.recvName(); r != "" {
		msg += `, not its receiver "` + r + `"`
	} else {
		msg += `; a phrase without a receiver reaches no node state`
	}
	var more []source.Extra
	if base != nil && base.Span().Start != base.Span().End {
		more = append(more, source.Also(w.file.Path, base.Span(), "not the receiver"))
	}
	if w.recv != nil {
		more = append(more, w.recvLabel())
	}
	w.errf(sp, source.ForeignState, msg, "other nodes learn through net.Send", more...)
}

func (w *ctxWalker) block(b *resolve.Block) {
	if b == nil {
		return
	}
	for _, s := range b.Stmts {
		w.stmt(s)
	}
}

func (w *ctxWalker) stmt(s resolve.Stmt) {
	switch x := s.(type) {
	case *resolve.Set:
		w.setPlace(x.Place)
		w.expr(x.Value)
	case *resolve.CallStmt:
		w.call(x.Call)
	case *resolve.If:
		w.expr(x.Cond)
		w.block(x.Then)
		w.block(x.Else)
	case *resolve.Each:
		w.expr(x.Over)
		w.block(x.Body)
	case *resolve.After:
		if w.ctx == stdlib.Check {
			w.notAllowed(x.Span(), "after")
		}
		w.expr(x.Delay)
		w.block(x.Body)
	case *resolve.Deliver:
		if w.ctx != stdlib.Main {
			w.notAllowed(x.Span(), "deliver")
		}
		w.expr(x.Rec)
		w.expr(x.To)
	case *resolve.Emit:
		w.emit(x)
	case *resolve.Draw:
		if w.ctx != stdlib.Look {
			w.notAllowed(x.Span(), "draw")
		}
		w.expr(x.Shape)
		if x.For != nil {
			w.expr(x.For)
		}
		w.block(x.Body)
	case *resolve.ExprStmt:
		w.expr(x.X)
	}
}

func (w *ctxWalker) emit(x *resolve.Emit) {
	w.expr(x.Rec)
	w.expr(x.By)
	switch w.ctx {
	case stdlib.Node:
		if !w.isRecv(x.By) {
			msg := `"by ` + x.By.Src() + `" must name the receiver`
			if r := w.recvName(); r != "" {
				msg += ` "` + r + `"`
			} else {
				msg += `; a phrase without a receiver names none`
			}
			var more []source.Extra
			if r := w.recvName(); r != "" {
				more = append(more, source.HelpEdit(`write "by `+r+`"`, source.EditAt(x.By.Span(), r)))
			}
			w.errf(x.By.Span(), source.ForeignState, msg, "", more...)
		}
	case stdlib.Main:
		if c, ok := x.By.(*resolve.Call); !ok || c.Phrase == nil || c.Phrase.Native != "sim.nobody" {
			var more []source.Extra
			if q := w.simQual(); q != "" {
				more = append(more, source.HelpEdit(`write "by `+q+`.Nobody"`, source.EditAt(x.By.Span(), q+".Nobody")))
			}
			w.errf(x.By.Span(), source.Context, `main is not a node: write "emit … by sim.Nobody"`, "", more...)
		}
	default:
		w.notAllowed(x.Span(), "emit")
	}
}

// setPlace checks the target of a set.
func (w *ctxWalker) setPlace(p resolve.Place) {
	switch x := p.(type) {
	case *resolve.SelectPlace:
		w.nodeState(x.Select, true)
	case *resolve.ItemPlace:
		w.setPlace(x.Base)
		w.expr(x.Key)
	}
}

// nodeState checks a read (write false) or write of node state through a
// member.
func (w *ctxWalker) nodeState(sel *resolve.Select, write bool) {
	w.expr(sel.Base)
	si := w.info.Sel[sel]
	if si == nil || si.Bad {
		return
	}
	states := 0
	for _, st := range si.Steps {
		if st.Kind == StepRecordField && w.ctx == stdlib.Look && !st.Field.Exported {
			w.errf(st.Sp, source.Context, "look context reads only exported fields", "")
			return
		}
		switch st.Kind {
		case StepNodeField, StepRecvPhrase:
		default:
			continue
		}
		states++
		switch w.ctx {
		case stdlib.Node:
			if states > 1 || !w.isRecv(sel.Base) {
				var base resolve.Expr
				if states <= 1 {
					base = sel.Base
				}
				w.foreign(sel.Span(), sel.Src(), baseOf(sel, states), base)
				return
			}
		case stdlib.Main:
		case stdlib.Check:
			if write {
				w.notAllowed(sel.Span(), "set "+sel.Src())
				return
			}
		case stdlib.Look:
			if write {
				w.notAllowed(sel.Span(), "set "+sel.Src())
				return
			}
			if st.Kind == StepNodeField && !st.Field.Exported {
				w.errf(st.Sp, source.Context, "look context reads only exported fields", "")
				return
			}
		}
		if st.Kind == StepRecvPhrase && w.ctx != stdlib.Node {
			w.errf(sel.Span(), source.Context,
				`"`+sel.Src()+`" is node code; it cannot run in `+ctxName(w.ctx)+` context`, "")
			return
		}
	}
}

// baseOf names the value that node state was reached through: the base, or
// for a second state step the member read so far.
func baseOf(sel *resolve.Select, states int) string {
	if states <= 1 {
		return sel.Base.Src()
	}
	return sel.Src()
}

func (w *ctxWalker) expr(e resolve.Expr) {
	switch x := e.(type) {
	case *resolve.Select:
		w.nodeState(x, false)
	case *resolve.Call:
		w.call(x)
	case *resolve.ListLit:
		for _, it := range x.Items {
			w.expr(it)
		}
	case *resolve.RecordLit:
		for _, st := range x.Settings {
			w.expr(st.Value)
		}
	case *resolve.Item:
		w.expr(x.Coll)
		w.expr(x.Key)
	case *resolve.Binary:
		w.expr(x.A)
		w.expr(x.B)
	case *resolve.Not:
		w.expr(x.X)
	case *resolve.Attachment:
		if w.ctx != stdlib.Look {
			w.notAllowed(x.Span(), x.Src())
		}
		w.expr(x.Key)
	case *resolve.ActionExpr:
		w.stmt(x.Stmt)
	}
}

// call checks a phrase call and walks its arguments and blocks.
func (w *ctxWalker) call(e *resolve.Call) {
	ph := e.Phrase
	if ph == nil {
		return
	}
	switch {
	case ph.Receiver != nil:
		head := ph.Head
		if e.Receiver != nil {
			head = e.Receiver.Src() + "." + ph.Head
		}
		switch {
		case w.ctx != stdlib.Node:
			w.errf(e.Span(), source.Context,
				`"`+head+`" is node code; it cannot run in `+ctxName(w.ctx)+` context`, "")
		case e.Receiver != nil && !w.isRecv(e.Receiver):
			w.foreign(e.Span(), head, e.Receiver.Src(), e.Receiver)
		}
		if e.Receiver != nil {
			w.expr(e.Receiver)
		}
	case ph.Pkg.IsStd:
		if !ph.Contexts.Has(w.ctx) {
			name := ph.Head
			if e.Qual != nil {
				name = e.Qual.Qual + "." + ph.Head
			} else if ph.Pkg != nil {
				name = ph.Pkg.Name + "." + ph.Head
			}
			w.errf(e.Span(), source.Context, `"`+name+`" is not allowed in `+ctxName(w.ctx)+` context`, "")
		}
		if ph.Sig != nil && ph.Sig.NamesNode >= 0 && w.ctx == stdlib.Node {
			w.namesNode(e, ph)
		}
	default:
		k := qkey{ph, w.ctx}
		if ph.Body != nil && !w.queued[k] {
			w.queued[k] = true
			w.queue = append(w.queue, qitem{k, w.file.Path + ":" + strconv.Itoa(e.Where().Line), w.file, e.Span()})
		}
	}
	for _, a := range e.Args {
		w.expr(a)
	}
	for _, b := range e.Blocks {
		w.block(b)
	}
}

// namesNode checks that the node named in a hole of a std phrase ("from a"
// of net.Send, "on n" of disk.Read) is the receiver.
func (w *ctxWalker) namesNode(e *resolve.Call, ph *resolve.Phrase) {
	i := ph.Sig.NamesNode
	if i >= len(e.Args) || i >= len(ph.Params) {
		return
	}
	a := e.Args[i]
	if w.isRecv(a) {
		return
	}
	word := ""
	for k, it := range ph.Items {
		if it.Param == ph.Params[i] && k > 0 && it.Param != nil {
			word = ph.Items[k-1].Word
		}
	}
	msg := `"` + word + " " + a.Src() + `" must name the receiver`
	if r := w.recvName(); r != "" {
		msg += ` "` + r + `"`
	} else {
		msg += `; a phrase without a receiver names none`
	}
	var more []source.Extra
	if r := w.recvName(); r != "" {
		more = append(more, source.HelpEdit(`name the receiver "`+r+`"`, source.EditAt(a.Span(), r)))
	}
	w.errf(a.Span(), source.ForeignState, msg, "", more...)
}

// simQual is the qualifier through which the current file imports std/sim,
// or "".
func (w *ctxWalker) simQual() string {
	for _, imp := range w.file.Imports {
		if imp.Pkg != nil && imp.Pkg.Path == "std/sim" {
			return imp.Qual
		}
	}
	return ""
}
