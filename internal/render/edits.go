package render

import (
	"sort"
	"strconv"
	"strings"

	"github.com/synodlang/compiler/internal/source"
)

// lineEdit is one single-line edit in code point indexes of its line.
type lineEdit struct {
	c0, c1 int
	new    string
	eol    bool // inserts the line break at the end of the last line
}

type changeKind int

const (
	insertLines changeKind = iota // whole lines inserted before line
	deleteLine                    // line removed
	editLine                      // edits inside one line
)

// change is the effect of a help's edits on one line.
type change struct {
	kind  changeKind
	line  int
	lines []string // insertLines: the inserted lines
	edits []lineEdit
}

// planEdits validates edits against src and groups them by line. It returns
// false when any edit is invalid: outside the file, spanning lines, or
// overlapping another.
func planEdits(src *source.File, edits []source.Edit) ([]change, bool) {
	lc := src.LineCount()
	var out []change
	byLine := map[int]int{} // line -> index in out of its editLine change
	deleted := map[int]bool{}
	for _, e := range edits {
		end := e.End
		if end.Line == 0 {
			end = source.Pos{Line: e.Line, Col: e.Col}
		}
		if e.Line < 1 || e.Line > lc || e.Col < 1 {
			return nil, false
		}
		n := len(cells(src.Line(e.Line)))
		isInsert := end.Line == e.Line && end.Col == e.Col
		switch {
		case e.Col == 1 && isInsert && strings.HasSuffix(e.New, "\n"):
			out = append(out, change{kind: insertLines, line: e.Line, lines: strings.Split(strings.TrimSuffix(e.New, "\n"), "\n")})
		case e.Col == 1 && end.Line == e.Line+1 && end.Col == 1 && e.New == "" && end.Line <= lc:
			if deleted[e.Line] {
				return nil, false
			}
			deleted[e.Line] = true
			out = append(out, change{kind: deleteLine, line: e.Line})
		case end.Line == e.Line && end.Col >= e.Col && end.Col <= n+1:
			le := lineEdit{c0: e.Col - 1, c1: end.Col - 1, new: e.New}
			if e.New == "\n" && isInsert && le.c0 == n {
				le.eol = true
			} else if strings.Contains(e.New, "\n") {
				return nil, false
			}
			i, ok := byLine[e.Line]
			if !ok {
				i = len(out)
				byLine[e.Line] = i
				out = append(out, change{kind: editLine, line: e.Line})
			}
			out[i].edits = append(out[i].edits, le)
		default:
			return nil, false
		}
	}
	for i := range out {
		c := &out[i]
		if c.kind == deleteLine {
			if _, ok := byLine[c.line]; ok {
				return nil, false
			}
		}
		if c.kind != editLine {
			continue
		}
		sort.SliceStable(c.edits, func(a, b int) bool {
			if c.edits[a].c0 != c.edits[b].c0 {
				return c.edits[a].c0 < c.edits[b].c0
			}
			return c.edits[a].c1 < c.edits[b].c1
		})
		for k := 1; k < len(c.edits); k++ {
			if c.edits[k-1].c1 > c.edits[k].c0 {
				return nil, false
			}
		}
	}
	sort.SliceStable(out, func(a, b int) bool {
		if out[a].line != out[b].line {
			return out[a].line < out[b].line
		}
		return out[a].kind < out[b].kind
	})
	return out, true
}

func (c change) maxLine() int {
	if c.kind == insertLines {
		return c.line + len(c.lines) - 1
	}
	return c.line
}

// diffRow is "N - text" or "N + text" in one style.
func diffRow(w, n int, sign string, st style, text string) row {
	num := strconv.Itoa(n)
	return row{{plain, pad(w, num)}, {st, num + " " + sign + " " + text}}
}

func joinCells(cs []cell) string {
	var sb strings.Builder
	for _, c := range cs {
		sb.WriteString(c.out)
	}
	return sb.String()
}

// emit writes the rows of the change.
func (c change) emit(rows *[]row, w int, src *source.File) {
	switch c.kind {
	case insertLines:
		for i, t := range c.lines {
			*rows = append(*rows, diffRow(w, c.line+i, "+", green, joinCells(cells(t))))
		}
	case deleteLine:
		*rows = append(*rows, diffRow(w, c.line, "-", diffRed, joinCells(cells(src.Line(c.line)))))
	case editLine:
		c.emitLine(rows, w, src)
	}
}

type piece struct {
	cs   []cell
	kind int // 0 unchanged, 1 inserted, 2 replaced
}

func (c change) emitLine(rows *[]row, w int, src *source.File) {
	old := cells(src.Line(c.line))
	diff := false
	for _, e := range c.edits {
		if e.c1 > e.c0 && (e.new == "" || (len(c.edits) == 1 && e.c0 == 0 && e.c1 == len(old))) {
			diff = true
		}
	}
	var ps []piece
	pos := 0
	for _, e := range c.edits {
		if e.c0 > pos {
			ps = append(ps, piece{old[pos:e.c0], 0})
		}
		switch {
		case e.eol:
			ps = append(ps, piece{[]cell{{"", 1}}, 1})
		case e.new != "":
			k := 1
			if e.c1 > e.c0 {
				k = 2
			}
			ps = append(ps, piece{cells(e.new), k})
		}
		pos = e.c1
	}
	if pos < len(old) {
		ps = append(ps, piece{old[pos:], 0})
	}
	if diff {
		var nw []cell
		for _, p := range ps {
			nw = append(nw, p.cs...)
		}
		*rows = append(*rows, diffRow(w, c.line, "-", diffRed, joinCells(old)))
		*rows = append(*rows, diffRow(w, c.line, "+", green, joinCells(nw)))
		return
	}
	code := row{{plain, pad(w, strconv.Itoa(c.line))}, {blue, strconv.Itoa(c.line) + " |"}, {plain, " "}}
	var mk rowBuilder
	for _, p := range ps {
		st, ch := plain, ""
		switch p.kind {
		case 1:
			st, ch = green, "+"
		case 2:
			st, ch = yellow, "~"
		}
		wd := 0
		for _, cl := range p.cs {
			wd += cl.w
		}
		code = append(code, seg{st, joinCells(p.cs)})
		if p.kind != 0 {
			if wd < 1 {
				wd = 1
			}
			mk.add(st, strings.Repeat(ch, wd), wd)
		} else {
			mk.add(plain, strings.Repeat(" ", wd), wd)
		}
	}
	*rows = append(*rows, code, prefixed(w, mk.segs))
}
