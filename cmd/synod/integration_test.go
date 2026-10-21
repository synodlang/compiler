package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// These tests need the real formatter (internal/format) bound in binding.go.

const examples = "../../testdata/examples"

func needReal(t *testing.T) {
	t.Helper()
	if formatFn == nil {
		t.Skip("internal/format not bound")
	}
}

func TestExamplesClean(t *testing.T) {
	needReal(t)
	if code, out, errs := exec("fmt", "--check", examples); code != 0 || out != "" || errs != "" {
		t.Errorf("fmt --check: code=%d out=%q err=%q", code, out, errs)
	}
	code, out, errs := exec("check", "--json", examples)
	if code != 0 || errs != "" || out != `{"format":"synod-check/1","ok":true,"diagnostics":[]}`+"\n" {
		t.Errorf("check --json: code=%d out=%q err=%q", code, out, errs)
	}
}

func TestRealFmtRestoresMisformatted(t *testing.T) {
	needReal(t)
	orig, err := os.ReadFile(filepath.Join(examples, "gossip", "gossip.synod"))
	if err != nil {
		t.Fatal(err)
	}
	mess := strings.Replace(string(orig), "\n", "  \n", 3) // trailing spaces
	mess = "\n" + mess                                     // leading blank line
	p := filepath.Join(t.TempDir(), "gossip", "gossip.synod")
	write(t, p, mess)
	code, out, _ := exec("check", p)
	if code != 1 || !strings.Contains(out, "non-canonical") {
		t.Fatalf("check: code=%d out=%q", code, out)
	}
	if code, out, _ := exec("fmt", "--check", p); code != 1 || strings.TrimSpace(out) != filepath.ToSlash(p) {
		t.Fatalf("fmt --check: code=%d out=%q", code, out)
	}
	if code, _, _ := exec("fmt", p); code != 0 {
		t.Fatalf("fmt: code=%d", code)
	}
	if got := read(t, p); got != string(orig) {
		t.Errorf("not restored:\n%s", got)
	}
	if code, out, _ := exec("check", p); code != 0 || out != "" {
		t.Errorf("after fmt: code=%d out=%q", code, out)
	}
}

func TestRealSyntaxErrorUntouched(t *testing.T) {
	needReal(t)
	p := filepath.Join(t.TempDir(), "pkg", "a.synod")
	const src = "# package pkg\n\nfoo x-1\n"
	write(t, p, src)
	if code, _, errs := exec("fmt", p); code != 1 || !strings.Contains(errs, "error[syntax]") {
		t.Errorf("fmt: code=%d err=%q", code, errs)
	}
	if read(t, p) != src {
		t.Error("file changed")
	}
}

// TestRunExamples runs every example through the CLI: the status line gets a
// scene line, --summary is byte-identical to the golden summary, and the
// scene file is written both ways.
func TestRunExamples(t *testing.T) {
	root, _ := filepath.Abs(examples)
	root = filepath.ToSlash(root)
	golden, _ := filepath.Abs("../../testdata/run/examples")
	dirs, err := os.ReadDir(golden)
	if err != nil || len(dirs) != 9 {
		t.Fatalf("goldens: %v, %d", err, len(dirs))
	}
	for _, d := range dirs {
		name := d.Name()
		t.Run(name, func(t *testing.T) {
			chdir(t, t.TempDir())
			code, stdout, stderr := exec("run", root+"/"+name, "--style", "short")
			if code != 0 || stderr != "" {
				t.Fatalf("exit %d: %s%s", code, stdout, stderr)
			}
			if !regexp.MustCompile(`\nscene [0-9.]+m?s, [1-9]\d* shapes -> out/scene\.json\n$`).MatchString(stdout) {
				t.Errorf("stdout %q", stdout)
			}
			want := read(t, filepath.Join(golden, name, "scene.json"))
			if got := read(t, "out/scene.json"); got != want {
				t.Errorf("scene differs from the golden")
			}
			os.RemoveAll("out")
			code, stdout, _ = exec("run", root+"/"+name, "--summary")
			if code != 0 || stdout != read(t, filepath.Join(golden, name, "summary.txt")) {
				t.Errorf("--summary differs from the golden (exit %d)", code)
			}
			if got := read(t, "out/scene.json"); got != want {
				t.Errorf("--summary scene differs")
			}
		})
	}
}
