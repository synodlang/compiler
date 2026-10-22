package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/synodlang/compiler/internal/explain"
)

func explainRun(args ...string) (code int, stdout, stderr string) {
	var o, e bytes.Buffer
	code = runExplain(args, &o, &e)
	return code, o.String(), e.String()
}

// golden compares got with testdata/explain/name; -update rewrites it.
func golden(t *testing.T, name, got string) {
	t.Helper()
	path := filepath.Join("testdata", "explain", name)
	if *update {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(want) != got {
		t.Errorf("%s differs; run go test ./cmd/synod -run Explain -update\n--- got\n%s", name, got)
	}
}

func TestExplainGoldens(t *testing.T) {
	cases := []struct {
		name string
		args []string
	}{
		{"explain-unknown-phrase.txt", []string{"unknown-phrase", "--style", "short"}},
		{"explain-unknown-phrase-rich.txt", []string{"unknown-phrase", "--style", "rich", "--color", "never"}},
		{"explain-unknown-phrase-color.txt", []string{"unknown-phrase", "--style", "rich", "--color", "always"}},
		{"explain-unknown-phrase.json", []string{"unknown-phrase", "--json"}},
		{"explain-missing-key.txt", []string{"missing-key", "--style", "short"}},
		{"explain-missing-key-rich.txt", []string{"missing-key", "--style", "rich", "--color", "never"}},
		{"explain-list.txt", nil},
		{"explain-catalog.md", []string{"--catalog"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			code, out, errs := explainRun(c.args...)
			if code != 0 || errs != "" {
				t.Fatalf("exit %d, stderr %q", code, errs)
			}
			golden(t, c.name, out)
			if !strings.HasSuffix(out, "\n") || strings.Contains(out, " \n") {
				t.Errorf("output must end with a newline and have no trailing spaces")
			}
		})
	}
}

func TestExplainShortIsDefaultOffTerminal(t *testing.T) {
	_, def, _ := explainRun("missing-key")
	_, short, _ := explainRun("missing-key", "--style", "short")
	if def != short {
		t.Errorf("a pipe gets the short form")
	}
}

func TestExplainJSON(t *testing.T) {
	code, out, _ := explainRun("--json")
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	var doc struct {
		Format string `json:"format"`
		Codes  []struct {
			Code, Kind, Summary, Text, Command, Diagnostic string
			Example                                        struct {
				Files   map[string]string
				Program string
			}
			Fix struct {
				Note    string
				Files   map[string]string
				Deleted []string
			}
		}
	}
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatal(err)
	}
	if doc.Format != "synod-explain/1" || len(doc.Codes) != len(explain.Codes()) {
		t.Fatalf("format %q, %d codes", doc.Format, len(doc.Codes))
	}
	for i, c := range doc.Codes {
		if c.Code != explain.Codes()[i] {
			t.Errorf("code %d is %s, want %s", i, c.Code, explain.Codes()[i])
		}
		if !strings.Contains(c.Diagnostic, "error["+c.Code+"]") {
			t.Errorf("%s: diagnostic %q", c.Code, c.Diagnostic)
		}
		if (c.Kind == "run") != (c.Command == "run") || (c.Command == "run") != (c.Example.Program != "") {
			t.Errorf("%s: kind %s command %s program %q", c.Code, c.Kind, c.Command, c.Example.Program)
		}
		if strings.Contains(c.Diagnostic, "\x1b") || c.Fix.Deleted == nil || len(c.Example.Files) == 0 {
			t.Errorf("%s: bad json fields", c.Code)
		}
	}
	// --json ignores --style and --color.
	_, again, _ := explainRun("--json", "--style", "rich", "--color", "always")
	if again != out {
		t.Errorf("--json must ignore --style and --color")
	}
	if strings.Count(out, "\n") != 1 {
		t.Errorf("json is one line")
	}
}

func TestExplainList(t *testing.T) {
	_, out, _ := explainRun()
	lines := strings.Split(strings.TrimSuffix(out, "\n"), "\n")
	if len(lines) != len(explain.Codes()) {
		t.Fatalf("%d lines, want %d", len(lines), len(explain.Codes()))
	}
	if !strings.HasPrefix(lines[0], "syntax  ") || !strings.HasPrefix(lines[len(lines)-1], "run-limit  ") {
		t.Errorf("order: first %q last %q", lines[0], lines[len(lines)-1])
	}
}

func TestExplainUnknownCode(t *testing.T) {
	code, out, errs := explainRun("unknown-phase")
	want := "synod explain: unknown code \"unknown-phase\"\n  hint: did you mean \"unknown-phrase\"?\n"
	if code != 2 || out != "" || errs != want {
		t.Errorf("exit %d stdout %q stderr %q", code, out, errs)
	}
	code, _, errs = explainRun("qqqqqqqq")
	if code != 2 || errs != "synod explain: unknown code \"qqqqqqqq\"\n" {
		t.Errorf("exit %d stderr %q", code, errs)
	}
}

func TestExplainUsage(t *testing.T) {
	for _, args := range [][]string{
		{"a", "b"}, {"--bogus"}, {"--style", "fancy"}, {"--color=x"},
		{"--json", "--catalog"}, {"--catalog", "syntax"}, {"--json=1"},
	} {
		if code, out, errs := explainRun(args...); code != 2 || out != "" || !strings.HasPrefix(errs, "synod explain: ") {
			t.Errorf("%v: exit %d stdout %q stderr %q", args, code, out, errs)
		}
	}
}
