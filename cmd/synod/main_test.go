package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/synodlang/compiler/internal/parse"
	"github.com/synodlang/compiler/internal/source"
)

// fakeFormat strips trailing blanks from every line. It stands in for
// internal/format so the CLI logic is tested on its own.
func fakeFormat(f *source.File) ([]byte, *source.Diagnostic) {
	if _, d := parse.Parse(f); d != nil {
		return nil, d
	}
	lines := strings.Split(string(f.Src), "\n")
	for i, l := range lines {
		lines[i] = strings.TrimRight(l, " ")
	}
	return []byte(strings.Join(lines, "\n")), nil
}

// useFakes installs the fake formatter for the test.
func useFakes(t *testing.T) {
	t.Helper()
	of := formatFn
	formatFn = fakeFormat
	t.Cleanup(func() { formatFn = of })
}

func exec(args ...string) (code int, stdout, stderr string) {
	var o, e bytes.Buffer
	code = run(args, strings.NewReader(""), &o, &e)
	return code, o.String(), e.String()
}

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func read(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

const good = "# package pkg\n\nfoo bar\n"
const bad = "# package pkg\n\nfoo bar   \n"

func TestFmtRewrites(t *testing.T) {
	useFakes(t)
	dir := t.TempDir()
	p := filepath.Join(dir, "pkg", "a.synod")
	write(t, p, bad)
	if err := os.Chmod(p, 0o600); err != nil {
		t.Fatal(err)
	}
	// Windows only models the read-only bit, so compare with the mode as the
	// OS reports it rather than a literal 0o600.
	before, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	code, out, errs := exec("fmt", dir)
	if code != 0 || out != "" || errs != "" {
		t.Fatalf("code=%d out=%q err=%q", code, out, errs)
	}
	if got := read(t, p); got != good {
		t.Errorf("got %q", got)
	}
	if info, _ := os.Stat(p); info.Mode().Perm() != before.Mode().Perm() {
		t.Errorf("mode changed: %v", info.Mode())
	}
	ents, _ := os.ReadDir(filepath.Dir(p))
	if len(ents) != 1 {
		t.Errorf("temp file left behind: %v", ents)
	}
}

func TestFmtLeavesUnparseable(t *testing.T) {
	useFakes(t)
	dir := t.TempDir()
	broken := filepath.Join(dir, "pkg", "broken.synod")
	other := filepath.Join(dir, "pkg", "other.synod")
	write(t, broken, "# package pkg\n\nx-1\n")
	write(t, other, bad)
	code, out, errs := exec("fmt", dir)
	if code != 1 || out != "" {
		t.Fatalf("code=%d out=%q", code, out)
	}
	if !strings.Contains(errs, "error[syntax]") {
		t.Errorf("stderr %q", errs)
	}
	if read(t, broken) != "# package pkg\n\nx-1\n" {
		t.Error("broken file changed")
	}
	if read(t, other) != good {
		t.Error("other file not formatted")
	}
}

func TestFmtCheck(t *testing.T) {
	useFakes(t)
	dir := t.TempDir()
	p := filepath.Join(dir, "pkg", "a.synod")
	write(t, p, bad)
	write(t, filepath.Join(dir, "pkg", "b.synod"), good)
	code, out, _ := exec("fmt", "--check", dir)
	want := filepath.ToSlash(p) + "\n"
	if code != 1 || out != want {
		t.Fatalf("code=%d out=%q want %q", code, out, want)
	}
	if read(t, p) != bad {
		t.Error("--check wrote")
	}
	// Flag after the path.
	code, out, _ = exec("fmt", dir, "--check")
	if code != 1 || out != want {
		t.Fatalf("flag last: code=%d out=%q", code, out)
	}
	write(t, p, good)
	if code, out, _ = exec("fmt", dir, "--check"); code != 0 || out != "" {
		t.Fatalf("clean: code=%d out=%q", code, out)
	}
}

func TestFmtWalkSkips(t *testing.T) {
	useFakes(t)
	dir := t.TempDir()
	for _, d := range []string{"testdata", ".hidden", "_x", "pkg/testdata"} {
		write(t, filepath.Join(dir, d, "a.synod"), bad)
	}
	write(t, filepath.Join(dir, "pkg", "notes.txt"), "x   ")
	write(t, filepath.Join(dir, "pkg", "a.synod"), good)
	if code, out, errs := exec("fmt", "--check", dir); code != 0 || out != "" || errs != "" {
		t.Fatalf("code=%d out=%q err=%q", code, out, errs)
	}
	// Explicit files are processed whatever the extension and location.
	explicit := filepath.Join(dir, "testdata", "a.synod")
	if code, _, _ := exec("fmt", explicit); code != 0 || read(t, explicit) != good {
		t.Errorf("explicit file not formatted: %q", read(t, explicit))
	}
}

func TestExitCodes(t *testing.T) {
	dir := t.TempDir()
	empty := filepath.Join(dir, "empty")
	write(t, filepath.Join(empty, "readme.txt"), "")
	cases := []struct {
		name string
		args []string
	}{
		{"no args", nil},
		{"unknown command", []string{"frob"}},
		{"not implemented", []string{"run", "x"}},
		{"missing path", []string{"check", filepath.Join(dir, "nope")}},
		{"missing fmt path", []string{"fmt", filepath.Join(dir, "nope")}},
		{"no synod files", []string{"check", empty}},
		{"tokens without json", []string{"check", "--tokens", dir}},
		{"unknown flag", []string{"fmt", "--frob", dir}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if code, _, errs := exec(c.args...); code != 2 || errs == "" {
				t.Errorf("code=%d stderr=%q", code, errs)
			}
		})
	}
}

func TestIOErrorBeatsDiagnostics(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "pkg", "a.synod"), "# package pkg\n\nx-1\n")
	if code, _, _ := exec("check", filepath.Join(dir, "pkg"), filepath.Join(dir, "missing")); code != 2 {
		t.Errorf("code=%d", code)
	}
}

func TestVersion(t *testing.T) {
	code, out, _ := exec("version")
	want := "language 0.2\ncompiler dev\nformats synod-check/1 synod-trace/1 synod-scene/1\n"
	if code != 0 || out != want {
		t.Errorf("code=%d out=%q", code, out)
	}
	if code, _, _ := exec("version", "x"); code != 2 {
		t.Errorf("version x: code=%d", code)
	}
}

func TestCheckJSONAndText(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "one", "a.synod")
	b := filepath.Join(dir, "two", "b.synod")
	const okSrc = "# package one\nNode has\n  set Foo to 0\n"
	write(t, a, "# package one\nNode has\n  set Foo to 0   \n")
	write(t, b, "# package two\nNode has\n\tset Foo to 0\n")
	pa, pb := filepath.ToSlash(a), filepath.ToSlash(b)

	code, out, _ := exec("check", "--json", dir)
	want := `{"format":"synod-check/1","ok":false,"diagnostics":[` +
		`{"file":"` + pa + `","line":3,"col":15,"severity":"error","code":"non-canonical","message":"trailing spaces"},` +
		`{"file":"` + pb + `","line":3,"col":1,"severity":"error","code":"indent","message":"tabs are not allowed","hint":"indent with 2 spaces per level"}]}` + "\n"
	if code != 1 || out != want {
		t.Fatalf("code=%d\ngot  %q\nwant %q", code, out, want)
	}

	code, out, _ = exec("check", dir)
	want = pa + ":3:15: error[non-canonical]: trailing spaces\n" +
		pb + ":3:1: error[indent]: tabs are not allowed\n  hint: indent with 2 spaces per level\n"
	if code != 1 || out != want {
		t.Fatalf("text: code=%d\ngot  %q\nwant %q", code, out, want)
	}

	write(t, a, okSrc)
	write(t, b, "# package two\nNode has\n  set Foo to 0\n")
	code, out, _ = exec("check", dir, "--json")
	if code != 0 || out != `{"format":"synod-check/1","ok":true,"diagnostics":[]}`+"\n" {
		t.Errorf("clean: code=%d out=%q", code, out)
	}
}

func TestBadHeader(t *testing.T) {
	dir := t.TempDir()
	cases := []struct {
		name, folder, src string
		want              string // substring of output, "" = clean
	}{
		{"ok", "pkg", "# package pkg\nNode has\n  set Foo to 0\n", ""},
		{"missing", "pkg", "Node has\n  set Foo to 0\n", `bad-header]: missing "# package" header`},
		{"wrong", "pkg", "# package other\nNode has\n  set Foo to 0\n", `package "other" must be the folder name "pkg" or main`},
		{"malformed", "pkg", "# package a_b\nNode has\n  set Foo to 0\n", `bad package header`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p := filepath.Join(dir, c.name, c.folder, "a.synod")
			write(t, p, c.src)
			code, out, _ := exec("check", p)
			if c.want == "" {
				if code != 0 || out != "" {
					t.Errorf("code=%d out=%q", code, out)
				}
				return
			}
			if code != 1 || !strings.Contains(out, c.want) || !strings.Contains(out, "bad-header") {
				t.Errorf("code=%d out=%q", code, out)
			}
		})
	}
}

func TestBadHeaderPosition(t *testing.T) {
	p := filepath.Join(t.TempDir(), "pkg", "a.synod")
	write(t, p, "Node has\n  set Foo to 0\n")
	_, out, _ := exec("check", p)
	if !strings.Contains(out, ":1:1: error[bad-header]") {
		t.Errorf("out=%q", out)
	}
}

func TestParseArgs(t *testing.T) {
	paths, flags, err := parseArgs([]string{"a", "--check", "b", "--", "--c"}, "check")
	if err != nil || strings.Join(paths, ",") != "a,b,--c" || !flags["check"] {
		t.Errorf("paths=%v flags=%v err=%v", paths, flags, err)
	}
	if _, _, err := parseArgs([]string{"--x"}, "check"); err == nil {
		t.Error("expected error")
	}
}
