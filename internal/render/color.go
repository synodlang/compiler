package render

import "strings"

// style is a color scheme entry (ANSI SGR).
type style int

const (
	plain   style = iota
	bold          // message, notes
	red           // error, primary markers and labels
	blue          // gutter, line numbers, secondary markers and labels
	cyan          // help
	green         // insertions
	diffRed       // removed lines
	yellow        // replacements
)

var sgr = [...]string{plain: "", bold: "1", red: "1;31", blue: "1;34", cyan: "1;36", green: "32", diffRed: "31", yellow: "33"}

// seg is a run of text in one style. A row is a list of segments; stripping
// the styles leaves exactly the plain output.
type seg struct {
	st style
	s  string
}

type row []seg

// rowBuilder assembles a row and tracks the display column of its end.
type rowBuilder struct {
	segs row
	col  int
}

func (b *rowBuilder) add(st style, s string, w int) {
	if s == "" && w == 0 {
		return
	}
	b.segs = append(b.segs, seg{st, s})
	b.col += w
}

// text adds s with its measured width.
func (b *rowBuilder) text(st style, s string) { b.add(st, s, textWidth(s)) }

// padTo adds spaces up to display column col.
func (b *rowBuilder) padTo(col int) {
	if col > b.col {
		b.add(plain, strings.Repeat(" ", col-b.col), col-b.col)
	}
}

// write appends r to sb, without trailing spaces and, when color is set,
// with each styled run followed by a reset.
func (r row) write(sb *strings.Builder, color bool) {
	end := len(r)
	trim := ""
	for end > 0 {
		t := strings.TrimRight(r[end-1].s, " ")
		if t != "" {
			trim = t
			break
		}
		end--
	}
	for i := 0; i < end; i++ {
		s := r[i].s
		if i == end-1 {
			s = trim
		}
		if s == "" {
			continue
		}
		if color && r[i].st != plain {
			sb.WriteString("\x1b[" + sgr[r[i].st] + "m")
			sb.WriteString(s)
			sb.WriteString("\x1b[0m")
		} else {
			sb.WriteString(s)
		}
	}
	sb.WriteByte('\n')
}
