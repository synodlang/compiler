package explain

import (
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/synodlang/compiler/internal/driver"
	"github.com/synodlang/compiler/internal/format"
	"github.com/synodlang/compiler/internal/pipeline"
)

// specCodes reads the codes of spec/11 sections 11.2 and 11.3, in table order.
func specCodes(t *testing.T) []string {
	t.Helper()
	dir := os.Getenv("SYNOD_DSL")
	if dir == "" {
		wd, _ := os.Getwd()
		dir = filepath.Join(wd, "..", "..", "..", "dsl")
	}
	data, err := os.ReadFile(filepath.Join(dir, "spec", "11-errors.md"))
	if err != nil {
		if os.Getenv("SYNOD_REQUIRE_DSL") == "1" {
			t.Fatal(err)
		}
		t.Skipf("dsl not found (%v); set SYNOD_DSL to run", err)
	}
	text := string(data)
	start := strings.Index(text, "## 11.2")
	end := strings.Index(text, "### Look faults")
	if start < 0 || end < start {
		t.Fatal("spec/11 sections 11.2 and 11.3 not found")
	}
	row := regexp.MustCompile("^\\| (`[^|]*`) \\|")
	code := regexp.MustCompile("`([a-z-]+)`")
	var out []string
	for _, l := range strings.Split(text[start:end], "\n") {
		if m := row.FindStringSubmatch(l); m != nil {
			for _, c := range code.FindAllStringSubmatch(m[1], -1) {
				out = append(out, c[1])
			}
		}
	}
	return out
}

// TestExplainCodes: the codes of the engine are exactly spec/11's, in order.
func TestExplainCodes(t *testing.T) {
	got, want := Codes(), specCodes(t)
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Fatalf("codes differ from spec/11\n got: %v\nwant: %v", got, want)
	}
}

// TestEntriesCoverSpec: one entry file per code of spec/11. While entries are
// still being written, SYNOD_EXPLAIN_PARTIAL=1 skips this test (and only this
// one) so that the rest of the suite can run; the default is to fail.
func TestEntriesCoverSpec(t *testing.T) {
	t.Run("missing_entries", func(t *testing.T) {
		missing := Missing()
		if len(missing) == 0 {
			return
		}
		if os.Getenv("SYNOD_EXPLAIN_PARTIAL") == "1" {
			t.Skipf("SYNOD_EXPLAIN_PARTIAL=1: %d codes have no entry yet: %v", len(missing), missing)
		}
		t.Errorf("no entry in internal/explain/codes for %d codes: %v", len(missing), missing)
	})
	t.Run("unparsable_or_unknown_files", func(t *testing.T) {
		for name, err := range Problems() {
			t.Errorf("codes/%s.md: %v", name, err)
		}
	})
}

func mustLookup(t *testing.T, code string) *Entry {
	t.Helper()
	e, ok := Lookup(code)
	if !ok {
		t.Fatalf("no entry for %s", code)
	}
	return e
}

// exemptFormat lists the examples whose files are not canonical on purpose.
var exemptFormat = map[string]bool{"non-canonical": true, "indent": true, "syntax": true}

func TestEntries(t *testing.T) {
	for _, e := range Entries() {
		e := e
		t.Run(e.Code, func(t *testing.T) {
			data, err := codeFS.ReadFile("codes/" + e.Code + ".md")
			if err != nil {
				t.Fatal(err)
			}
			if Format(e) != string(data) {
				t.Errorf("the file is not in canonical entry form; Format(Parse(file)) differs")
			}

			// The example gives diagnostics, all with the entry's code.
			d, err := Render(e)
			if err != nil {
				t.Fatal(err)
			}
			for _, diag := range d.Diags {
				if string(diag.Code) != e.Code {
					t.Errorf("example gives %s, want only %s: %s", diag.Code, e.Code, diag.String())
				}
			}
			if e.Kind() == KindRun {
				if want := "error[" + e.Code + "]"; !strings.HasPrefix(d.Failure, "failed at ") || !strings.Contains(d.Failure, want) {
					t.Errorf("run failure line %q, want failed at …: %s", d.Failure, want)
				}
			}
			short := d.Diagnostic(pipeline.Style{})
			if !strings.Contains(short, "error["+e.Code+"]") {
				t.Errorf("short diagnostic lacks the code: %q", short)
			}

			// The fixed module checks clean and runs with exit 0.
			diags, code := CheckFixed(e)
			if len(diags) > 0 {
				t.Errorf("fixed module is not clean: %v", diags)
			}
			if code != 0 {
				t.Errorf("fixed run exits %d, want 0", code)
			}

			// Every block is canonical, except in the three exempt entries.
			if exemptFormat[e.Code] {
				return
			}
			for name, files := range map[string][]File{"example": e.ExampleFiles(), "fixed": e.FixedFiles()} {
				cfg, targets := inMemory(files)
				res := driver.Check(driver.Options{Load: cfg}, targets)
				for _, f := range files {
					if !strings.HasSuffix(f.Path, ".synod") {
						continue
					}
					src := res.Sources[f.Path]
					if src == nil {
						continue
					}
					out, fd := pipeline.FormatFile(res, f.Path, src, format.Format)
					if fd != nil {
						// Syntax-level failures are the example's point only
						// in the exempt entries.
						t.Errorf("%s %s: fmt: %s", name, f.Path, fd.String())
					} else if !bytes.Equal(out, []byte(f.Content)) {
						t.Errorf("%s %s: synod fmt --check would change it", name, f.Path)
					}
				}
			}
		})
	}
}

func TestParseStrict(t *testing.T) {
	good := "# missing-key\n\nS.\n\nProse.\n\n## Example\n\nRun with `synod run p`.\n\n### p/main.synod\n```synod\n# package main\n```\n\n## Fix\n\nF.\n\n### p/main.synod\n```synod\n# package main\n```\n"
	if _, err := Parse("missing-key", []byte(good)); err != nil {
		t.Fatalf("good entry rejected: %v", err)
	}
	bad := map[string]string{
		"wrong name":      strings.Replace(good, "# missing-key", "# index-range", 1),
		"no final nl":     strings.TrimSuffix(good, "\n"),
		"trailing space":  strings.Replace(good, "Prose.", "Prose. ", 1),
		"no run line":     strings.Replace(good, "Run with `synod run p`.\n\n", "", 1),
		"no fix":          good[:strings.Index(good, "\n## Fix")+1],
		"no fix file":     good[:strings.Index(good, "### p/main.synod\n```synod\n# package main\n```\n\n## Fix")] + "## Fix\n\nF.\n",
		"bad fence":       strings.Replace(good, "```synod", "```", 1),
		"no period":       strings.Replace(good, "S.", "S", 1),
		"four paragraphs": strings.Replace(good, "Prose.", "A.\n\nB.\n\nC.\n\nD.", 1),
		"extra blank":     strings.Replace(good, "## Fix\n\nF.", "## Fix\n\n\nF.", 1),
	}
	for name, text := range bad {
		if _, err := Parse("missing-key", []byte(text)); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	if _, err := Parse("unknown-phrase", []byte(strings.Replace(good, "# missing-key", "# unknown-phrase", 1))); err == nil {
		t.Error("a Run line on a compile-time code was accepted")
	}
}

func TestFixedFiles(t *testing.T) {
	e := &Entry{
		Example:    []File{{"a/a.synod", "1\n"}, {"a/b.synod", "2\n"}},
		FixFiles:   []File{{"a/a.synod", "3\n"}, {"a/c.synod", "4\n"}},
		FixDeleted: []string{"a/b.synod"},
	}
	var got []string
	for _, f := range e.FixedFiles() {
		got = append(got, f.Path+"="+strings.TrimSpace(f.Content))
	}
	want := "synod.mod=module example.com/explain\nsynod 0.3 a/a.synod=3 a/c.synod=4"
	if strings.Join(got, " ") != want {
		t.Errorf("got %q want %q", strings.Join(got, " "), want)
	}
}

func TestSuggest(t *testing.T) {
	for in, want := range map[string]string{"unknown-phase": "unknown-phrase", "sintax": "syntax", "zzzzzz": "", "missing-ke": "missing-key"} {
		if got := Suggest(in); got != want {
			t.Errorf("Suggest(%q) = %q, want %q", in, got, want)
		}
	}
}
