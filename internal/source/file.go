// Package source holds source files, positions and diagnostics shared by
// every stage of the Synod compiler.
package source

import (
	"sort"
	"unicode/utf8"
)

// bom is the UTF-8 byte order mark. A leading BOM is tolerated by the lexer
// and does not count as a column.
const bom = "\xef\xbb\xbf"

// Pos is a position in a source file. Line and Col are 1-based and Col
// counts code points, not bytes.
type Pos struct {
	Line int `json:"line"`
	Col  int `json:"col"`
}

// File is a source file together with an index of its line starts.
type File struct {
	Path string // path as given by the caller, used in diagnostics
	Src  []byte // raw file contents

	lines []int // byte offset of the start of each line
	skip  int   // length of a leading BOM, 0 if none
}

// NewFile indexes src. The slice is retained, not copied.
func NewFile(path string, src []byte) *File {
	f := &File{Path: path, Src: src, lines: []int{0}}
	if len(src) >= len(bom) && string(src[:len(bom)]) == bom {
		f.skip = len(bom)
	}
	for i, b := range src {
		if b == '\n' {
			f.lines = append(f.lines, i+1)
		}
	}
	return f
}

// PosOf returns the position of byte offset off. Offsets past the end of the
// file are clamped to the end. A leading BOM occupies no column.
func (f *File) PosOf(off int) Pos {
	if off < 0 {
		off = 0
	}
	if off > len(f.Src) {
		off = len(f.Src)
	}
	// Index of the last line starting at or before off.
	i := sort.Search(len(f.lines), func(i int) bool { return f.lines[i] > off }) - 1
	start := f.lines[i]
	if i == 0 && f.skip > 0 {
		if off < f.skip {
			return Pos{Line: 1, Col: 1}
		}
		start = f.skip
	}
	return Pos{Line: i + 1, Col: utf8.RuneCount(f.Src[start:off]) + 1}
}

// LineCount returns the number of lines, counting a final line that has no
// terminating newline and counting the empty line after a final newline.
func (f *File) LineCount() int { return len(f.lines) }

// Line returns the text of line n (1-based) without its line terminator: no
// "\n", no trailing "\r" and, on line 1, no BOM. It returns "" when n is out
// of range.
func (f *File) Line(n int) string {
	if n < 1 || n > len(f.lines) {
		return ""
	}
	start := f.lines[n-1]
	if n == 1 {
		start = f.skip
	}
	end := len(f.Src)
	if n < len(f.lines) {
		end = f.lines[n] - 1
	}
	if end > start && f.Src[end-1] == '\r' {
		end--
	}
	if end < start {
		end = start
	}
	return string(f.Src[start:end])
}
