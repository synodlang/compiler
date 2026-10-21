package parse

import (
	"path/filepath"
	"testing"

	"github.com/synodlang/compiler/internal/source"
)

// FuzzParse checks that Parse never panics, that it returns exactly one of a
// tree and a diagnostic, and that a tree holds every comment of the source.
func FuzzParse(f *testing.F) {
	for _, dir := range []string{"examples", "parse", "lex"} {
		walkSynodSeeds(f, filepath.Join(testdata, dir))
	}
	f.Fuzz(func(t *testing.T, src []byte) {
		file, d := Parse(source.NewFile("fuzz.synod", src))
		if (file == nil) == (d == nil) {
			t.Fatalf("file %v and diagnostic %v", file != nil, d != nil)
		}
		if d != nil {
			if d.Line < 1 || d.Col < 1 || d.Message == "" {
				t.Fatalf("malformed diagnostic %+v", *d)
			}
			return
		}
		checkInvariants(t, src, file)
		_ = Dump(file)
		HeaderDiagnostic(file, "x")
	})
}

// walkSynodSeeds adds every .synod file below dir to the corpus.
func walkSynodSeeds(f *testing.F, dir string) {
	f.Helper()
	walkSynod(f, dir, func(_ string, src []byte) { f.Add(src) })
}
