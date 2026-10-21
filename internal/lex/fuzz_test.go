package lex

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/synodlang/compiler/internal/source"
)

// seedFiles adds every .synod file below dir to the fuzz corpus.
func seedFiles(f *testing.F, dir string) {
	f.Helper()
	filepath.WalkDir(dir, func(path string, e fs.DirEntry, err error) error {
		if err == nil && !e.IsDir() && strings.HasSuffix(path, ".synod") {
			if src, err := os.ReadFile(path); err == nil {
				f.Add(src)
			}
		}
		return nil
	})
}

// FuzzLex checks the structural invariants of the token stream for arbitrary
// input: no panic, ordered offsets, balanced INDENT and DEDENT, a single
// final EOF, and progress through the input.
func FuzzLex(f *testing.F) {
	seedFiles(f, filepath.Join("..", "..", "testdata", "examples"))
	seedFiles(f, filepath.Join("..", "..", "testdata", "lex"))
	f.Fuzz(func(t *testing.T, src []byte) {
		toks, d := Lex(source.NewFile("fuzz.synod", src))
		if d != nil {
			if toks != nil {
				t.Fatal("tokens returned with a diagnostic")
			}
			if d.Line < 1 || d.Col < 1 || d.Code == "" || d.Message == "" {
				t.Fatalf("malformed diagnostic %+v", *d)
			}
			if d.End.Line != d.Line || d.End.Col < d.Col {
				t.Fatalf("diagnostic span ends before it starts: %+v", *d)
			}
			return
		}
		if len(toks) == 0 || toks[len(toks)-1].Kind != EOF {
			t.Fatal("token stream does not end in EOF")
		}
		depth, prev, prevReal := 0, 0, -1
		for i, tok := range toks {
			if tok.Off < prev || tok.Off > len(src) {
				t.Fatalf("token %d (%s) offset %d out of order (prev %d)", i, tok.Kind, tok.Off, prev)
			}
			prev = tok.Off
			switch tok.Kind {
			case INDENT:
				depth++
			case DEDENT:
				depth--
				if depth < 0 {
					t.Fatal("DEDENT without INDENT")
				}
			case EOF:
				if i != len(toks)-1 {
					t.Fatal("EOF before the end")
				}
			case NEWLINE:
			default:
				if tok.Off <= prevReal {
					t.Fatalf("token %d (%s) offset %d does not advance past %d", i, tok.Kind, tok.Off, prevReal)
				}
				prevReal = tok.Off
				if e := tok.End(); e.Line != tok.Pos.Line || e.Col <= tok.Pos.Col {
					t.Fatalf("token %d (%s) ends at %v, starts at %v", i, tok.Kind, e, tok.Pos)
				}
				if tok.Text == "" || !strings.HasPrefix(string(src[tok.Off:]), tok.Text) {
					t.Fatalf("token %d (%s) text %q is not the source at %d", i, tok.Kind, tok.Text, tok.Off)
				}
			}
		}
		if depth != 0 {
			t.Fatalf("INDENT/DEDENT unbalanced by %d", depth)
		}
	})
}
