package trace_test

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/synodlang/compiler/internal/trace"
)

// moduleRoot returns the folder that holds the synod.mod of the program of
// the trace in dir: dir or its parents up to testdata/run, or the vendored
// examples for testdata/run/examples.
func moduleRoot(t *testing.T, dir, root string) string {
	t.Helper()
	if strings.HasPrefix(filepath.ToSlash(dir), filepath.ToSlash(filepath.Join(root, "examples"))+"/") {
		return filepath.Join(filepath.Dir(root), "examples")
	}
	for d := dir; d != root; d = filepath.Dir(d) {
		if _, err := os.Stat(filepath.Join(d, "synod.mod")); err == nil {
			return d
		}
	}
	t.Fatalf("no synod.mod for %s", dir)
	return ""
}

// TestDecodeRoundTrip checks Encode(Decode(f)) == f for every golden trace.
func TestDecodeRoundTrip(t *testing.T) {
	root, err := filepath.Abs("../../testdata/run")
	if err != nil {
		t.Fatal(err)
	}
	var files []string
	filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err == nil && d.Name() == "trace.jsonl" {
			files = append(files, p)
		}
		return nil
	})
	if len(files) < 30 {
		t.Fatalf("only %d traces found", len(files))
	}
	cache := map[string]map[string]*programRef{}
	for _, f := range files {
		rel, _ := filepath.Rel(root, f)
		t.Run(filepath.ToSlash(filepath.Dir(rel)), func(t *testing.T) {
			golden, err := os.ReadFile(f)
			if err != nil {
				t.Fatal(err)
			}
			mod := moduleRoot(t, filepath.Dir(f), root)
			progs := cache[mod]
			if progs == nil {
				progs = compile(t, mod)
				cache[mod] = progs
			}
			first := string(golden[:bytes.IndexByte(golden, '\n')])
			var pr *programRef
			for name, p := range progs {
				if strings.Contains(first, `"program":"`+name+`"`) {
					pr = p
				}
			}
			if pr == nil {
				t.Fatalf("no program for header %s", first)
			}
			h, facts, err := trace.Decode(bytes.NewReader(golden), pr.prog, pr.extra...)
			if err != nil {
				t.Fatal(err)
			}
			var out bytes.Buffer
			if err := trace.Encode(&out, h, facts, trace.Options{Qual: trace.Qualifiers(pr.prog)}); err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(out.Bytes(), golden) {
				t.Fatalf("round trip differs:\n%s", firstDiff(out.String(), string(golden)))
			}
		})
	}
}

func firstDiff(a, b string) string {
	al, bl := strings.Split(a, "\n"), strings.Split(b, "\n")
	for i := 0; i < len(al) && i < len(bl); i++ {
		if al[i] != bl[i] {
			return "got  " + al[i] + "\nwant " + bl[i]
		}
	}
	return "different lengths"
}

func TestDecodeErrors(t *testing.T) {
	for _, in := range []string{
		"",
		"not json\n",
		`{"format":"synod-trace/2","seed":1,"program":"p","packages":[]}` + "\n",
	} {
		if _, _, err := trace.Decode(strings.NewReader(in), emptyProg()); err == nil {
			t.Errorf("accepted %q", in)
		}
	}
	hdr := `{"format":"synod-trace/1","synod":"0.2","seed":1,"program":"p","packages":[]}` + "\n"
	for _, in := range []string{
		hdr + `{"seq":2,"t":0,"ev":"sim.Started"}` + "\n",
		hdr + `{"seq":1,"t":0,"ev":"sim.Nope"}` + "\n",
		hdr + `{"seq":1,"t":0,"ev":"sim.Started"}`,
	} {
		if _, _, err := trace.Decode(strings.NewReader(in), emptyProg()); err == nil {
			t.Errorf("accepted %q", in)
		}
	}
}

func TestDecodeFailedPos(t *testing.T) {
	in := `{"format":"synod-trace/1","synod":"0.2","seed":1,"program":"p","packages":[]}` + "\n" +
		`{"seq":1,"t":5,"ev":"sim.Failed","code":"nobody","message":"m","pos":"a/b:c.synod:12:3"}` + "\n"
	_, facts, err := trace.Decode(strings.NewReader(in), emptyProg())
	if err != nil {
		t.Fatal(err)
	}
	f := facts[0].Fault
	if f.Pos.File != "a/b:c.synod" || f.Pos.Line != 12 || f.Pos.Col != 3 || !f.HasPos || facts[0].Node != -1 {
		t.Fatalf("%+v %+v", f, facts[0])
	}
}
