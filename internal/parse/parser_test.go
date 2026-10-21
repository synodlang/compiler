package parse

import (
	"flag"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/synodlang/compiler/internal/lex"
	"github.com/synodlang/compiler/internal/source"
)

var update = flag.Bool("update", false, "rewrite the golden files in testdata/parse")

var testdata = filepath.Join("..", "..", "testdata")

// walkSynod calls fn for every .synod file below dir.
func walkSynod(t testing.TB, dir string, fn func(path string, src []byte)) {
	t.Helper()
	n := 0
	err := filepath.WalkDir(dir, func(path string, e fs.DirEntry, err error) error {
		if err != nil || e.IsDir() || !strings.HasSuffix(path, ".synod") {
			return err
		}
		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		n++
		fn(path, src)
		return nil
	})
	if err != nil || n == 0 {
		t.Fatalf("no .synod files below %s: %v", dir, err)
	}
}

// golden compares got with the file path, or rewrites it under -update.
func golden(t *testing.T, path, got string) {
	t.Helper()
	if *update {
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v (run with -update)", err)
	}
	if string(want) != got {
		t.Errorf("%s differs\n--- got ---\n%s--- want ---\n%s", path, got, want)
	}
}

func TestExamplesParse(t *testing.T) {
	walkSynod(t, filepath.Join(testdata, "examples"), func(path string, src []byte) {
		f, d := Parse(source.NewFile(path, src))
		if d != nil {
			t.Errorf("%s", d)
			return
		}
		if f.Header == nil || f.Package == "" {
			t.Errorf("%s: no package header", path)
		}
		if len(f.Trailing) != 0 {
			t.Errorf("%s: unexpected trailing comments", path)
		}
	})
}

func TestGoldenOK(t *testing.T) {
	walkSynod(t, filepath.Join(testdata, "parse", "ok"), func(path string, src []byte) {
		t.Run(strings.TrimSuffix(filepath.Base(path), ".synod"), func(t *testing.T) {
			f, d := Parse(source.NewFile(filepath.Base(path), src))
			if d != nil {
				t.Fatalf("unexpected error: %s", d)
			}
			golden(t, strings.TrimSuffix(path, ".synod")+".tree", Dump(f))
			checkInvariants(t, src, f)
		})
	})
}

func TestGoldenErr(t *testing.T) {
	walkSynod(t, filepath.Join(testdata, "parse", "err"), func(path string, src []byte) {
		t.Run(strings.TrimSuffix(filepath.Base(path), ".synod"), func(t *testing.T) {
			f, d := Parse(source.NewFile(filepath.Base(path), src))
			if d == nil {
				t.Fatalf("no error; tree:\n%s", Dump(f))
			}
			if f != nil {
				t.Error("file returned together with a diagnostic")
			}
			if d.Code != source.Syntax && d.Code != source.Indent {
				t.Errorf("code %q, want syntax or indent", d.Code)
			}
			golden(t, strings.TrimSuffix(path, ".synod")+".err", d.String()+"\n")
		})
	})
}

// checkInvariants verifies that every comment of the source is in the tree
// exactly once and that the tree is well formed.
func checkInvariants(t testing.TB, src []byte, f *File) {
	t.Helper()
	toks, d := lex.Lex(source.NewFile("x", src))
	if d != nil {
		t.Fatalf("lexer error on parsed source: %s", d)
	}
	want := 0
	for _, tok := range toks {
		if tok.Kind == lex.COMMENT {
			want++
		}
	}
	got := 0
	if f.Header != nil {
		got++
	}
	got += len(f.Trailing)
	var walkSent func(s *Sentence)
	var walkLines func(ls []*Line)
	walkBlock := func(b *Block) {
		if len(b.Lines) == 0 {
			t.Error("empty block")
		}
		got += len(b.Trailing)
		walkLines(b.Lines)
	}
	walkSent = func(s *Sentence) {
		if s == nil || len(s.Groups) == 0 {
			t.Error("empty sentence")
			return
		}
		for _, g := range s.Groups {
			if len(g) == 0 {
				t.Error("empty group")
			}
		}
	}
	walkLines = func(ls []*Line) {
		for _, l := range ls {
			got += len(l.Leading)
			if l.Comment != nil {
				got++
			}
			walkSent(l.Sentence)
			if l.Block != nil {
				walkBlock(l.Block)
			} else if len(l.Conts) > 0 {
				t.Error("continuation without block")
			}
			for _, c := range l.Conts {
				got += len(c.Leading)
				if c.Comment != nil {
					got++
				}
				walkBlock(c.Block)
			}
		}
	}
	for _, n := range f.Body {
		switch n := n.(type) {
		case *CommentGroup:
			if len(n.Comments) == 0 {
				t.Error("empty comment group")
			}
			got += len(n.Comments)
		case *Line:
			walkLines([]*Line{n})
		}
	}
	if got != want {
		t.Errorf("tree holds %d comments, source has %d", got, want)
	}
}

func TestParseEmpty(t *testing.T) {
	for _, src := range []string{"", "\n\n", "\xef\xbb\xbf"} {
		f, d := Parse(source.NewFile("x.synod", []byte(src)))
		if d != nil || len(f.Body) != 0 || f.Header != nil || len(f.Trailing) != 0 {
			t.Errorf("%q: got %+v, %v", src, f, d)
		}
	}
}

func TestIsWord(t *testing.T) {
	w := &Word{Name: "else"}
	for _, tc := range []struct {
		it   Item
		name string
		want bool
	}{
		{w, "else", true},
		{w, "if", false},
		{&Member{Parts: []string{"else", "x"}}, "else", false},
		{&Text{Raw: `"else"`}, "else", false},
	} {
		if got := IsWord(tc.it, tc.name); got != tc.want {
			t.Errorf("IsWord(%T, %q) = %v", tc.it, tc.name, got)
		}
	}
}

func TestHeaderDiagnostic(t *testing.T) {
	tests := []struct {
		name, src, folder string
		want              string // "" for no diagnostic
	}{
		{"folder", "# package gossip\n", "gossip", ""},
		{"main", "# package main\n", "gossip", ""},
		{"blank lines first", "\n\n# package gossip\nx\n", "gossip", ""},
		{"missing", "define main\n  a\n", "gossip",
			"h.synod:1:1: error[bad-header]: missing \"# package\" header\n  hint: the first line is \"# package <folder>\""},
		{"empty file", "", "gossip",
			"h.synod:1:1: error[bad-header]: missing \"# package\" header\n  hint: the first line is \"# package <folder>\""},
		{"other comment", "# hello\n# package gossip\n", "gossip",
			"h.synod:1:1: error[bad-header]: missing \"# package\" header\n  hint: the first line is \"# package <folder>\""},
		{"not a comment", "package gossip\n", "gossip",
			"h.synod:1:1: error[bad-header]: missing \"# package\" header\n  hint: the first line is \"# package <folder>\""},
		{"two words", "# package a b\n", "a",
			"h.synod:1:1: error[bad-header]: bad package header \"# package a b\"\n  hint: write \"# package <name>\""},
		{"underscore", "\n# package a_b\n", "a",
			"h.synod:2:1: error[bad-header]: bad package header \"# package a_b\"\n  hint: write \"# package <name>\""},
		{"leading digit", "# package 1a\n", "a",
			"h.synod:1:1: error[bad-header]: bad package header \"# package 1a\"\n  hint: write \"# package <name>\""},
		{"empty name", "# package \nx\n", "a", // trailing space is trimmed by the lexer: not a header
			"h.synod:1:1: error[bad-header]: missing \"# package\" header\n  hint: the first line is \"# package <folder>\""},
		{"wrong name", "# package raft\n", "gossip",
			"h.synod:1:1: error[bad-header]: package \"raft\" must be the folder name \"gossip\" or main"},
		{"case matters", "# package Gossip\n", "gossip",
			"h.synod:1:1: error[bad-header]: package \"Gossip\" must be the folder name \"gossip\" or main"},
		{"wrong name after blanks", "\n\n# package raft\n", "gossip",
			"h.synod:3:1: error[bad-header]: package \"raft\" must be the folder name \"gossip\" or main"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f, d := Parse(source.NewFile("h.synod", []byte(tc.src)))
			if d != nil {
				t.Fatal(d)
			}
			got := ""
			if hd := HeaderDiagnostic(f, tc.folder); hd != nil {
				got = hd.String()
				if hd.Code != source.BadHeader {
					t.Errorf("code %q", hd.Code)
				}
			}
			if got != tc.want {
				t.Errorf("got  %q\nwant %q", got, tc.want)
			}
		})
	}
}

// TestPositions pins a few positions that diagnostics of later stages use.
func TestPositions(t *testing.T) {
	src := "# package d\nuse std/net\ndefine [n: Node] Go\n  set x to (a b)\n"
	f, d := Parse(source.NewFile("x.synod", []byte(src)))
	if d != nil {
		t.Fatal(d)
	}
	def := f.Body[1].(*Line)
	if def.Pos != (source.Pos{Line: 3, Col: 1}) {
		t.Errorf("line pos %v", def.Pos)
	}
	hole := def.Sentence.Groups[0][1].(*Hole)
	if hole.P != (source.Pos{Line: 3, Col: 8}) || hole.Name.P != (source.Pos{Line: 3, Col: 9}) {
		t.Errorf("hole pos %v %v", hole.P, hole.Name.P)
	}
	set := def.Block.Lines[0]
	if p := set.Sentence.Groups[0][3].(*Paren); p.P != (source.Pos{Line: 4, Col: 12}) {
		t.Errorf("paren pos %v", p.P)
	}
}
