package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func call(f func([]string, io.Writer, io.Writer) int, args ...string) (int, string, string) {
	var o, e bytes.Buffer
	c := f(args, &o, &e)
	return c, o.String(), e.String()
}

func TestNewDefault(t *testing.T) {
	needReal(t)
	dir := filepath.Join(t.TempDir(), "hello")
	code, out, errs := call(runNew, dir)
	if code != 0 || errs != "" {
		t.Fatalf("code %d: %s", code, errs)
	}
	want := "created " + dir + ": synod.mod, .gitignore, hello/hello.synod, hello/demo.synod\n"
	if out != want {
		t.Errorf("out %q", out)
	}
	mod, _ := os.ReadFile(filepath.Join(dir, "synod.mod"))
	if string(mod) != "module hello\nsynod 0.3\n" {
		t.Errorf("synod.mod %q", mod)
	}
	gi, _ := os.ReadFile(filepath.Join(dir, ".gitignore"))
	if string(gi) != "vendor/\nout/\n" {
		t.Errorf(".gitignore %q", gi)
	}
	if c, _, e := call(runFmt, "--check", dir); c != 0 {
		t.Errorf("fmt --check %d: %s", c, e)
	}
	if c, o, e := call(runCheck, dir); c != 0 {
		t.Errorf("check %d: %s%s", c, o, e)
	}
	out2 := t.TempDir()
	if c, o, e := call(runRun, filepath.Join(dir, "hello"), "--trace", filepath.Join(out2, "t.jsonl"), "--scene", filepath.Join(out2, "s.json")); c != 0 {
		t.Errorf("run %d: %s%s", c, o, e)
	}
	// A second run on the same folder is refused and changes nothing.
	code, _, errs = call(runNew, dir)
	if code != 2 || !strings.Contains(errs, "is not empty") {
		t.Errorf("second new: %d %q", code, errs)
	}
}

func TestNewLibAndModule(t *testing.T) {
	needReal(t)
	dir := filepath.Join(t.TempDir(), "a", "b", "x")
	code, out, errs := call(runNew, dir, "--lib", "--module", "example.com/x/hello")
	if code != 0 {
		t.Fatalf("%d %s", code, errs)
	}
	if strings.Contains(out, "demo") {
		t.Errorf("lib has demo: %s", out)
	}
	if _, err := os.Stat(filepath.Join(dir, "hello", "demo.synod")); err == nil {
		t.Error("demo.synod exists")
	}
	mod, _ := os.ReadFile(filepath.Join(dir, "synod.mod"))
	if string(mod) != "module example.com/x/hello\nsynod 0.3\n" {
		t.Errorf("synod.mod %q", mod)
	}
	if c, _, e := call(runCheck, dir); c != 0 {
		t.Errorf("check: %s", e)
	}
	// With a path module the demo imports the full path.
	dir2 := filepath.Join(t.TempDir(), "y")
	if c, _, e := call(runNew, dir2, "--module", "example.com/x/hello"); c != 0 {
		t.Fatal(e)
	}
	if c, o, e := call(runCheck, dir2); c != 0 {
		t.Errorf("check demo: %s%s", o, e)
	}
}

func TestNewRefusals(t *testing.T) {
	root := t.TempDir()
	full := filepath.Join(root, "full")
	os.MkdirAll(full, 0o755)
	os.WriteFile(filepath.Join(full, "f"), nil, 0o644)
	empty := filepath.Join(root, "empty")
	os.MkdirAll(empty, 0o755)
	for _, c := range []struct {
		args []string
		want string
	}{
		{[]string{full}, "is not empty"},
		{[]string{filepath.Join(root, "my-app")}, `"my-app" is not a package name; use a lower-case word`},
		{[]string{filepath.Join(root, "Hello")}, `"Hello" is not a package name`},
		{[]string{filepath.Join(root, "define")}, `"define" is not a package name`},
		{[]string{filepath.Join(root, "ok"), "--module", "a//b"}, "is not a module path"},
		{[]string{}, "usage"},
		{[]string{empty, "--bogus"}, "unknown flag"},
	} {
		code, _, errs := call(runNew, c.args...)
		if code != 2 || !strings.Contains(errs, c.want) {
			t.Errorf("%v: %d %q", c.args, code, errs)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "my-app")); err == nil {
		t.Error("refused dir was created")
	}
	// An empty existing folder is fine.
	if code, _, e := call(runNew, empty); code != 0 {
		t.Errorf("empty folder: %s", e)
	}
}
