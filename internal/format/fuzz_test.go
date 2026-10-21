package format

import (
	"path/filepath"
	"testing"

	"github.com/synodlang/compiler/internal/lex"
	"github.com/synodlang/compiler/internal/parse"
	"github.com/synodlang/compiler/internal/source"
)

// countComments returns the number of comment tokens of src.
func countComments(t *testing.T, src []byte) int {
	toks, d := lex.Lex(source.NewFile("x.synod", src))
	if d != nil {
		t.Fatalf("lex: %s", d)
	}
	n := 0
	for _, tk := range toks {
		if tk.Kind == lex.COMMENT {
			n++
		}
	}
	return n
}

// FuzzFormat checks that formatting is idempotent, yields canonical parseable
// text and loses no comment.
func FuzzFormat(f *testing.F) {
	for _, dir := range []string{"examples", "format", "misformatted", "parse"} {
		walkSynod(f, filepath.Join(testdata, dir), func(_ string, src []byte) { f.Add(src) })
	}
	f.Fuzz(func(t *testing.T, src []byte) {
		out, d := Format(source.NewFile("fuzz.synod", src))
		if d != nil {
			if out != nil {
				t.Fatal("output together with a diagnostic")
			}
			return
		}
		again, d := Format(source.NewFile("fuzz.synod", out))
		if d != nil {
			t.Fatalf("output does not parse: %s\n%q", d, out)
		}
		if string(again) != string(out) {
			t.Fatalf("not idempotent:\n%q\n%q", out, again)
		}
		if d := Check(source.NewFile("fuzz.synod", out)); d != nil {
			t.Fatalf("output is not canonical: %s", d)
		}
		if _, d := parse.Parse(source.NewFile("fuzz.synod", out)); d != nil {
			t.Fatal(d)
		}
		if a, b := countComments(t, src), countComments(t, out); a != b {
			t.Fatalf("%d comments became %d", a, b)
		}
	})
}
