package driver

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/synodlang/compiler/internal/load"
	render2 "github.com/synodlang/compiler/internal/render"
	"github.com/synodlang/compiler/internal/source"
)

var update = flag.Bool("update", false, "rewrite the expect.txt, expect-rich.txt and expect.json files of testdata/check")

// render is the text output of synod check for the diagnostics.
func render(ds []source.Diagnostic) string {
	var b strings.Builder
	for _, d := range ds {
		b.WriteString(d.String())
		b.WriteByte('\n')
	}
	return b.String()
}

// renderRich is the rich output of synod check without color.
func renderRich(t *testing.T, res Result, ds []source.Diagnostic) string {
	t.Helper()
	var b bytes.Buffer
	if err := render2.All(&b, ds, render2.Options{Source: func(f string) *source.File { return res.Sources[f] }}); err != nil {
		t.Fatal(err)
	}
	return b.String()
}

// checkRich enforces the shape of the rich output: one block per
// diagnostic, each starting with the diagnostic's header, no empty line
// inside a block, no escapes.
func checkRich(t *testing.T, rich string, ds []source.Diagnostic) {
	t.Helper()
	if strings.Contains(rich, "\x1b") {
		t.Errorf("escape in the rich output")
	}
	if strings.Contains(rich, "\r") {
		t.Errorf("CR in the rich output")
	}
	blocks := strings.Split(strings.TrimSuffix(rich, "\n"), "\n\n")
	if len(blocks) != len(ds) {
		t.Errorf("%d rich blocks for %d diagnostics:\n%s", len(blocks), len(ds), rich)
		return
	}
	for i, b := range blocks {
		first, _, _ := strings.Cut(b, "\n")
		if first != ds[i].Header() {
			t.Errorf("block %d starts with %q, want %q", i, first, ds[i].Header())
		}
	}
}

func jsonOf(t *testing.T, ds []source.Diagnostic) string {
	t.Helper()
	var b bytes.Buffer
	if err := source.WriteJSON(&b, ds); err != nil {
		t.Fatal(err)
	}
	return b.String()
}

// TestGolden runs every case of testdata/check/<code>/<case>/: a module
// with package folders, and the output of synod check with paths relative to
// the case: expect.txt (the short form, the contract for agents),
// expect-rich.txt (--style rich --color never) and expect.json (--json). The case must report its folder's code.
func TestGolden(t *testing.T) {
	root := "../../testdata/check"
	codes, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, c := range codes {
		if !c.IsDir() {
			continue
		}
		cases, err := os.ReadDir(filepath.Join(root, c.Name()))
		if err != nil {
			t.Fatal(err)
		}
		for _, cs := range cases {
			if !cs.IsDir() {
				continue
			}
			dir := filepath.Join(root, c.Name(), cs.Name())
			code := source.Code(c.Name())
			t.Run(c.Name()+"/"+cs.Name(), func(t *testing.T) {
				n++
				res := checkDir(t, dir, false)
				var ds []source.Diagnostic
				found := false
				for _, d := range res.Diags {
					if strings.HasPrefix(d.File, "std/") {
						t.Errorf("diagnostic in std: %s", d)
						continue
					}
					ds = append(ds, d)
					found = found || d.Code == code
				}
				if !found {
					t.Errorf("no diagnostic with code %s in:\n%s", code, render(ds))
				}
				got := render(ds)
				rich := renderRich(t, res, ds)
				js := jsonOf(t, ds)
				checkRich(t, rich, ds)
				// The JSON renders to the same rich text.
				var doc struct {
					Format      string              `json:"format"`
					Diagnostics []source.Diagnostic `json:"diagnostics"`
				}
				if err := json.Unmarshal([]byte(js), &doc); err != nil {
					t.Fatal(err)
				}
				if again := renderRich(t, res, doc.Diagnostics); again != rich {
					t.Errorf("rich output of the decoded JSON differs\n--- got\n%s--- want\n%s", again, rich)
				}
				files := map[string]string{"expect.txt": got, "expect-rich.txt": rich, "expect.json": js}
				if *update {
					for name, content := range files {
						if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
							t.Fatal(err)
						}
					}
					return
				}
				for _, name := range []string{"expect.txt", "expect-rich.txt", "expect.json"} {
					want, err := os.ReadFile(filepath.Join(dir, name))
					if err != nil {
						t.Fatal(err)
					}
					if string(want) != files[name] {
						t.Errorf("%s differs (run with -update to rewrite)\n--- got\n%s--- want\n%s", name, files[name], want)
					}
				}
				// The result is the same on a second run.
				if again := render(checkDir(t, dir, false).Diags); again != render(res.Diags) {
					t.Errorf("not deterministic")
				}
			})
		}
	}
	_ = load.StdModule
}
