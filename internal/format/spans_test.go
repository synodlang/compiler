package format

import (
	"bytes"
	"sort"
	"testing"
	"unicode/utf8"

	"github.com/synodlang/compiler/internal/source"
)

// applyEdits applies edits to src, from the last to the first.
func applyEdits(src []byte, edits []source.Edit) []byte {
	starts := []int{0}
	for i, b := range src {
		if b == '\n' {
			starts = append(starts, i+1)
		}
	}
	off := func(p source.Pos) int {
		o := starts[p.Line-1]
		for c := 1; c < p.Col; c++ {
			_, n := utf8.DecodeRune(src[o:])
			o += n
		}
		return o
	}
	type rep struct {
		from, to int
		new      string
	}
	var rs []rep
	for _, e := range edits {
		from := off(source.Pos{Line: e.Line, Col: e.Col})
		to := from
		if e.End != (source.Pos{}) {
			to = off(e.End)
		}
		rs = append(rs, rep{from, to, e.New})
	}
	sort.SliceStable(rs, func(i, j int) bool { return rs[i].from > rs[j].from })
	out := append([]byte(nil), src...)
	for _, r := range rs {
		out = append(out[:r.from], append([]byte(r.new), out[r.to:]...)...)
	}
	return out
}

// TestCheckSpans pins the primary span of each kind of non-canonical
// diagnostic, and applies its help edit: the file is canonical afterwards.
func TestCheckSpans(t *testing.T) {
	tests := []struct {
		name, src string
		line      int
		from, to  int  // columns of the span, to exclusive
		edit      bool // has an edit that makes the file canonical
	}{
		{"bom", "\xef\xbb\xbfa\n", 1, 1, 1, false},
		{"crlf", "define a\r\n  b\r\n", 1, 9, 10, false},
		{"trailing", "define a\n  b  \n", 2, 4, 6, true},
		{"extra blank", "define a\n\n  b\n", 2, 1, 1, true},
		{"blank with spaces", "define a\n  \n  b\n", 2, 1, 3, true},
		{"missing blank", "# package x\nuse a/b\ndefine c\n", 3, 1, 1, true},
		{"no newline", "define a\n  b", 2, 4, 4, true},
		{"blank at end", "a\n\n", 3, 1, 1, true},
		{"blanks at end", "a\n\n\n", 3, 1, 1, true},
		{"not canonical", "define a\n  b  c\n", 2, 5, 6, true},
		{"spaces in parens", "a ( b c )\n", 1, 4, 9, true},
		{"multi-byte", "define a\n  set b to \"ü\"  \n", 2, 15, 17, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := source.NewFile("x.synod", []byte(tt.src))
			d := Check(f)
			if d == nil {
				t.Fatal("canonical")
			}
			if d.Line != tt.line || d.Col != tt.from || d.End.Line != tt.line || d.End.Col != tt.to {
				t.Errorf("span %d:%d to %v, want %d:%d to %d:%d (%s)", d.Line, d.Col, d.End, tt.line, tt.from, tt.line, tt.to, d.Message)
			}
			if len(d.Help) != 1 {
				t.Fatalf("%d helps", len(d.Help))
			}
			e := d.Help[0].Edits
			if (len(e) > 0) != tt.edit {
				t.Fatalf("edits %+v", e)
			}
			if !tt.edit {
				return
			}
			// Each edit fixes the diagnostic it belongs to; several blank
			// lines at the end go one at a time.
			fixed := applyEdits(f.Src, e)
			for i := 0; i < 3; i++ {
				d2 := Check(source.NewFile("x.synod", fixed))
				if d2 == nil {
					break
				}
				if d2.Message != d.Message || len(d2.Help) == 0 || len(d2.Help[0].Edits) == 0 {
					t.Fatalf("after the edit: %s\n%q", d2, fixed)
				}
				fixed = applyEdits(fixed, d2.Help[0].Edits)
			}
			want, _ := Format(f)
			if !bytes.Equal(fixed, want) {
				t.Errorf("edit gives %q, Format gives %q", fixed, want)
			}
		})
	}
}
