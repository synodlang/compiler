// Package render draws diagnostics in the rich form of spec/11 section 11.1:
// the header line, the source line with markers under the primary span and
// the secondary labels, notes and help, and the edits of a fix.
//
// The package is pure: it reads source through Options.Source and writes to
// an io.Writer. Column widths are an approximation (see width.go).
package render

import (
	"io"
	"strconv"
	"strings"

	"github.com/synodlang/compiler/internal/source"
)

// Options controls the rendering.
type Options struct {
	Color bool // ANSI colors; stripping them gives the plain output

	// Source returns the file shown for a display path, or nil. With a nil
	// Source, or for a file it does not know, no snippet is drawn.
	Source func(file string) *source.File

	// Header replaces the first line (run failures); "" uses d.Header().
	Header string
}

// Write writes d to w, ending with a newline.
func Write(w io.Writer, d source.Diagnostic, o Options) error {
	var sb strings.Builder
	one(&sb, d, o)
	_, err := io.WriteString(w, sb.String())
	return err
}

// All writes ds to w, the blocks separated by one empty line.
func All(w io.Writer, ds []source.Diagnostic, o Options) error {
	var sb strings.Builder
	for i, d := range ds {
		if i > 0 {
			sb.WriteByte('\n')
		}
		one(&sb, d, o)
	}
	_, err := io.WriteString(w, sb.String())
	return err
}

func header(d source.Diagnostic, o Options) row {
	h := o.Header
	if h == "" {
		h = d.Header()
	}
	tok := d.Severity + "[" + string(d.Code) + "]"
	i := strings.Index(h, tok+": ")
	if i < 0 {
		return row{{plain, h}}
	}
	return row{
		{plain, h[:i]},
		{red, tok},
		{plain, ": "},
		{bold, h[i+len(tok)+2:]},
	}
}

func digits(n int) int { return len(strconv.Itoa(n)) }

func one(sb *strings.Builder, d source.Diagnostic, o Options) {
	var main *section
	var others []*section
	if o.Source != nil && d.Line >= 1 {
		if src := o.Source(d.File); src != nil && d.Line <= src.LineCount() {
			main = &section{path: d.File, src: src}
			main.add(d.Line, d.Col, d.End, d.Label, true)
		}
	}
	if main != nil {
		byFile := map[string]*section{}
		for _, l := range d.Labels {
			if l.File == "" || l.File == d.File {
				main.add(l.Line, l.Col, l.End, l.Text, false)
				continue
			}
			s, ok := byFile[l.File]
			if !ok {
				src := o.Source(l.File)
				if src == nil {
					byFile[l.File] = nil
					continue
				}
				s = &section{path: l.File, src: src}
				byFile[l.File] = s
				others = append(others, s)
			}
			if s != nil {
				s.add(l.Line, l.Col, l.End, l.Text, false)
			}
		}
	}
	live := others[:0]
	for _, s := range others {
		if len(s.labs) > 0 {
			live = append(live, s)
		}
	}
	others = live

	// Plan the edit blocks; helps whose edits are unusable show their text.
	type block struct {
		text    string
		changes []change
	}
	var blocks []block
	var textHelp []string
	hasHelp := len(d.Help) > 0
	for _, h := range d.Help {
		if len(h.Edits) > 0 && main != nil {
			if cs, ok := planEdits(main.src, h.Edits); ok {
				blocks = append(blocks, block{h.Text, cs})
				continue
			}
		}
		textHelp = append(textHelp, h.Text)
	}
	if !hasHelp && len(d.Notes) == 0 && d.Hint != "" {
		textHelp = append(textHelp, d.Hint)
	}

	w := 1
	if main != nil {
		m := main.maxLine()
		for _, s := range others {
			if x := s.maxLine(); x > m {
				m = x
			}
		}
		for _, b := range blocks {
			for _, c := range b.changes {
				if x := c.maxLine(); x > m {
					m = x
				}
			}
		}
		w = digits(m)
	}

	rows := []row{header(d, o)}
	if main != nil {
		rows = append(rows, blankGutter(w))
		main.emit(&rows, w)
		for _, s := range others {
			ln, col := s.first()
			rows = append(rows, blankGutter(w),
				row{{plain, strings.Repeat(" ", w)}, {blue, "::: " + s.path + ":" + strconv.Itoa(ln) + ":" + strconv.Itoa(col)}},
				blankGutter(w))
			s.emit(&rows, w)
		}
	}
	var tail []row
	eq := func(st style, label, text string) {
		tail = append(tail, row{{plain, strings.Repeat(" ", w) + " "}, {st, "= " + label}, {plain, " " + sanitize(text)}})
	}
	for _, n := range d.Notes {
		eq(bold, "note:", n)
	}
	for _, t := range textHelp {
		eq(cyan, "help:", t)
	}
	if main != nil && (len(tail) > 0 || len(blocks) > 0) {
		rows = append(rows, blankGutter(w))
	}
	rows = append(rows, tail...)
	for _, b := range blocks {
		rows = append(rows, row{{cyan, "help:"}, {plain, " " + sanitize(b.text)}}, blankGutter(w))
		for _, c := range b.changes {
			c.emit(&rows, w, main.src)
		}
		rows = append(rows, blankGutter(w))
	}
	for _, r := range rows {
		r.write(sb, o.Color)
	}
}
