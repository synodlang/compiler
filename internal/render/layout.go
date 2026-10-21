package render

import (
	"sort"
	"strconv"
	"strings"

	"github.com/synodlang/compiler/internal/source"
)

// lab is a label normalized to one line of a file: c0 and c1 are 0-based
// code point indexes into the line, c1 exclusive.
type lab struct {
	line, c0, c1 int
	text         string
	primary      bool
}

// section is the labels of one file, shown together.
type section struct {
	path  string
	src   *source.File
	labs  []lab
	cache map[int][]cell
}

func (s *section) cells(line int) []cell {
	if s.cache == nil {
		s.cache = map[int][]cell{}
	}
	c, ok := s.cache[line]
	if !ok {
		c = cells(s.src.Line(line))
		s.cache[line] = c
	}
	return c
}

// disp returns the display column of code point index i of line.
func (s *section) disp(line, i int) int {
	cs := s.cells(line)
	w := 0
	for k := 0; k < i; k++ {
		if k < len(cs) {
			w += cs[k].w
		} else {
			w++
		}
	}
	return w
}

// add normalizes a span into a label on its start line. An end on a later
// line is clamped to the end of the start line and marks the text with "…".
// Spans outside the file are dropped.
func (s *section) add(line, col int, end source.Pos, text string, primary bool) {
	if line < 1 || line > s.src.LineCount() {
		return
	}
	n := len(s.cells(line))
	c0 := col - 1
	if c0 < 0 {
		c0 = 0
	}
	if c0 > n {
		c0 = n
	}
	c1 := c0
	switch {
	case end.Line > line:
		c1 = n
		text += "…"
	case end.Line == line:
		c1 = end.Col - 1
		if c1 > n {
			c1 = n
		}
		if c1 < c0 {
			c1 = c0
		}
	}
	s.labs = append(s.labs, lab{line, c0, c1, text, primary})
}

func (s *section) maxLine() int {
	m := 0
	for _, l := range s.labs {
		if l.line > m {
			m = l.line
		}
	}
	return m
}

// first returns the position shown on the "::: " row.
func (s *section) first() (int, int) {
	best := s.labs[0]
	for _, l := range s.labs[1:] {
		if l.line < best.line || (l.line == best.line && l.c0 < best.c0) {
			best = l
		}
	}
	return best.line, best.c0 + 1
}

func (l lab) style() style {
	if l.primary {
		return red
	}
	return blue
}

func pad(w int, n string) string {
	if len(n) >= w {
		return ""
	}
	return strings.Repeat(" ", w-len(n))
}

func blankGutter(w int) row {
	return row{{plain, strings.Repeat(" ", w)}, {blue, " |"}}
}

func codeRow(w, n int, text string) row {
	num := strconv.Itoa(n)
	return row{{plain, pad(w, num)}, {blue, num + " |"}, {plain, " " + text}}
}

// prefixed puts the marker gutter before content.
func prefixed(w int, content row) row {
	r := row{{plain, strings.Repeat(" ", w)}, {blue, " |"}, {plain, " "}}
	return append(r, content...)
}

// emit writes the rows of the section: code lines with their markers, the
// middle line for a gap of one and "..." for larger gaps.
func (s *section) emit(rows *[]row, w int) {
	set := map[int]bool{}
	for _, l := range s.labs {
		set[l.line] = true
	}
	lines := make([]int, 0, len(set))
	for n := range set {
		lines = append(lines, n)
	}
	sort.Ints(lines)
	prev := 0
	for _, n := range lines {
		if prev != 0 {
			switch gap := n - prev; {
			case gap == 2:
				*rows = append(*rows, codeRow(w, prev+1, joinCells(s.cells(prev+1))))
			case gap > 2:
				*rows = append(*rows, row{{blue, "..."}})
			}
		}
		*rows = append(*rows, codeRow(w, n, joinCells(s.cells(n))))
		*rows = append(*rows, s.markerRows(w, n)...)
		prev = n
	}
}

type placed struct {
	lab   lab
	s, wd int
}

// markerRows draws the markers of the labels of line n. Labels that do not
// overlap share a row; the rightmost of those has its text inline and the
// others hang below on connectors. Overlapping labels get their own rows,
// the primary first.
func (s *section) markerRows(w, n int) []row {
	var ls []placed
	for _, l := range s.labs {
		if l.line != n {
			continue
		}
		st := s.disp(n, l.c0)
		wd := s.disp(n, l.c1) - st
		if wd < 1 {
			wd = 1
		}
		ls = append(ls, placed{l, st, wd})
	}
	sort.SliceStable(ls, func(i, j int) bool {
		if ls[i].lab.primary != ls[j].lab.primary {
			return ls[i].lab.primary
		}
		return ls[i].s < ls[j].s
	})
	var groups [][]placed
next:
	for _, p := range ls {
		for gi, g := range groups {
			ok := true
			for _, q := range g {
				if p.s < q.s+q.wd && q.s < p.s+p.wd {
					ok = false
					break
				}
			}
			if ok {
				groups[gi] = append(g, p)
				continue next
			}
		}
		groups = append(groups, []placed{p})
	}
	var out []row
	for _, g := range groups {
		sort.SliceStable(g, func(i, j int) bool { return g[i].s < g[j].s })
		var b rowBuilder
		for _, p := range g {
			b.padTo(p.s)
			ch := "-"
			if p.lab.primary {
				ch = "^"
			}
			b.add(p.lab.style(), strings.Repeat(ch, p.wd), p.wd)
		}
		pending := g
		last := g[len(g)-1]
		if last.lab.text != "" {
			b.add(plain, " ", 1)
			b.text(last.lab.style(), sanitize(last.lab.text))
			pending = g[:len(g)-1]
		}
		out = append(out, prefixed(w, b.segs))
		var hang []placed
		for _, p := range pending {
			if p.lab.text != "" {
				hang = append(hang, p)
			}
		}
		if len(hang) == 0 {
			continue
		}
		var c rowBuilder
		for _, p := range hang {
			c.padTo(p.s)
			c.add(p.lab.style(), "|", 1)
		}
		out = append(out, prefixed(w, c.segs))
		for k := len(hang) - 1; k >= 0; k-- {
			var t rowBuilder
			for j := 0; j < k; j++ {
				t.padTo(hang[j].s)
				t.add(hang[j].lab.style(), "|", 1)
			}
			t.padTo(hang[k].s)
			t.text(hang[k].lab.style(), sanitize(hang[k].lab.text))
			out = append(out, prefixed(w, t.segs))
		}
	}
	return out
}
