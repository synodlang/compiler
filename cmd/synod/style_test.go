package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// inject replaces the terminal and environment probes for one test.
func inject(t *testing.T, tty func(io.Writer) bool, env map[string]string, vt func(io.Writer) bool) {
	t.Helper()
	ot, oe, ov := isTerminal, getenv, enableVT
	isTerminal = tty
	getenv = func(k string) string { return env[k] }
	if vt == nil {
		vt = func(io.Writer) bool { return true }
	}
	enableVT = vt
	t.Cleanup(func() { isTerminal, getenv, enableVT = ot, oe, ov })
}

func TestStyleResolution(t *testing.T) {
	var w bytes.Buffer
	for _, style := range []string{"", "rich", "short"} {
		for _, color := range []string{"", "auto", "always", "never"} {
			for _, noColor := range []string{"", "1"} {
				for _, term := range []string{"xterm", "dumb"} {
					for _, tty := range []bool{false, true} {
						inject(t, func(io.Writer) bool { return tty }, map[string]string{"NO_COLOR": noColor, "TERM": term}, nil)
						rich, color2 := styleFlags{style, color}.resolve(&w)
						wantRich := style == "rich" || style == "" && tty
						wantColor := wantRich
						switch color {
						case "never":
							wantColor = false
						case "always":
						default:
							wantColor = wantRich && tty && noColor == "" && term != "dumb"
						}
						if rich != wantRich || color2 != wantColor {
							t.Errorf("style=%q color=%q NO_COLOR=%q TERM=%q tty=%v: got rich=%v color=%v, want %v %v",
								style, color, noColor, term, tty, rich, color2, wantRich, wantColor)
						}
					}
				}
			}
		}
	}
}

func TestStyleVTFailureTurnsColorOff(t *testing.T) {
	inject(t, func(io.Writer) bool { return true }, map[string]string{}, func(io.Writer) bool { return false })
	rich, color := styleFlags{}.resolve(&bytes.Buffer{})
	if !rich || color {
		t.Errorf("rich=%v color=%v, want rich without color", rich, color)
	}
}

func TestIsTerminal(t *testing.T) {
	if isTerminal(&bytes.Buffer{}) {
		t.Error("a buffer is a terminal")
	}
	f, err := os.Create(filepath.Join(t.TempDir(), "f"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if isTerminal(f) {
		t.Error("a regular file is a terminal")
	}
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	defer w.Close()
	if isTerminal(w) {
		t.Error("a pipe is a terminal")
	}
	null, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer null.Close()
	if !isTerminal(null) {
		t.Error("the null device is not a character device")
	}
}

func TestEnableVTOnPipe(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	defer w.Close()
	if enableVTReal(w) {
		t.Error("enableVT on a pipe reported success")
	}
	if enableVTReal(&bytes.Buffer{}) {
		t.Error("enableVT on a buffer reported success")
	}
}

// styleCase writes a module with one unknown-phrase error and returns its
// folder.
func styleCase(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	write(t, filepath.Join(dir, "synod.mod"), "module example.com/t\nsynod 0.2\n")
	write(t, filepath.Join(dir, "p", "main.synod"), "# package main\ndefine main\n  frobnicate 3\n")
	return filepath.ToSlash(filepath.Join(dir, "p"))
}

func TestCheckStyleFlags(t *testing.T) {
	folder := styleCase(t)
	code, short, _ := exec("check", folder)
	if code != 1 || strings.Contains(short, " |") {
		t.Fatalf("default (piped) is not the short form: code=%d\n%s", code, short)
	}
	for _, args := range [][]string{
		{"check", folder, "--style=rich", "--color=never"},
		{"check", "--style", "rich", "--color", "never", folder},
	} {
		code, out, errs := exec(args...)
		if code != 1 || errs != "" {
			t.Fatalf("%v: code=%d err=%q", args, code, errs)
		}
		if !strings.Contains(out, "\n  |\n") || !strings.Contains(out, "3 |   frobnicate 3\n") || strings.Contains(out, "\x1b") {
			t.Errorf("%v: not the rich form:\n%s", args, out)
		}
		if first, _, _ := strings.Cut(out, "\n"); first != strings.SplitN(short, "\n", 2)[0] {
			t.Errorf("%v: first line %q differs from the short form", args, first)
		}
	}
	_, out, _ := exec("check", folder, "--style=rich", "--color=always")
	if !strings.Contains(out, "\x1b[") {
		t.Errorf("--color always gives no escapes:\n%q", out)
	}
	_, out, _ = exec("check", folder, "--style", "short", "--color", "always")
	if out != short {
		t.Errorf("--style short is not the short form:\n%s", out)
	}
	for _, bad := range [][]string{
		{"check", folder, "--style=fancy"},
		{"check", folder, "--color=sometimes"},
		{"check", folder, "--style"},
		{"fmt", folder, "--style=fancy"},
		{"run", folder, "--color=x"},
		{"run", folder, "--style=x"},
	} {
		if code, _, errs := exec(bad...); code != 2 || errs == "" {
			t.Errorf("%v: code=%d err=%q, want exit 2", bad, code, errs)
		}
	}
}

func TestCheckJSONIgnoresStyle(t *testing.T) {
	folder := styleCase(t)
	_, plain, _ := exec("check", folder, "--json")
	for _, args := range [][]string{
		{"check", folder, "--json", "--style", "rich", "--color", "always"},
		{"check", folder, "--json", "--style=short"},
	} {
		if _, out, _ := exec(args...); out != plain || strings.Contains(out, "\x1b") {
			t.Errorf("%v changes the JSON:\n%s", args, out)
		}
	}
}

// onlyTTY makes exactly the writers in tty look like terminals.
func onlyTTY(t *testing.T, tty ...io.Writer) {
	t.Helper()
	inject(t, func(w io.Writer) bool {
		for _, x := range tty {
			if x == w {
				return true
			}
		}
		return false
	}, map[string]string{"TERM": "xterm"}, nil)
}

func TestCheckDetectsStdout(t *testing.T) {
	folder := styleCase(t)
	var o, e bytes.Buffer
	onlyTTY(t, &o)
	run([]string{"check", folder}, strings.NewReader(""), &o, &e)
	if !strings.Contains(o.String(), "\x1b[") || !strings.Contains(o.String(), "frobnicate 3") {
		t.Errorf("a terminal stdout gets no rich color output:\n%q", o.String())
	}
	o.Reset()
	inject(t, func(w io.Writer) bool { return w == &o }, map[string]string{"NO_COLOR": "1", "TERM": "xterm"}, nil)
	run([]string{"check", folder}, strings.NewReader(""), &o, &e)
	if strings.Contains(o.String(), "\x1b") || !strings.Contains(o.String(), " | ") {
		t.Errorf("NO_COLOR keeps rich without escapes:\n%q", o.String())
	}
}

func TestFmtDetectsStderr(t *testing.T) {
	useFakes(t)
	p := filepath.Join(t.TempDir(), "a.synod")
	write(t, p, "# package a\n\ndefine A\n  result (1\n")
	var o, e bytes.Buffer
	onlyTTY(t, &e)
	if code := run([]string{"fmt", p, "--color", "never"}, strings.NewReader(""), &o, &e); code != 1 {
		t.Fatalf("exit %d", code)
	}
	if o.Len() != 0 {
		t.Errorf("stdout: %q", o.String())
	}
	if !strings.Contains(e.String(), "4 |   result (1\n") || strings.Contains(e.String(), "\x1b") {
		t.Errorf("fmt diagnostic is not rich on a terminal stderr:\n%s", e.String())
	}
	// With stdout a terminal but stderr not, the short form is used.
	o.Reset()
	e.Reset()
	onlyTTY(t, &o)
	run([]string{"fmt", p}, strings.NewReader(""), &o, &e)
	if strings.Contains(e.String(), " | ") {
		t.Errorf("fmt followed stdout, not stderr:\n%s", e.String())
	}
}

func TestRunTraceDashFailureRendersOnStderr(t *testing.T) {
	folder, _ := filepath.Abs("../../testdata/run/errors/division-by-zero/number/p")
	chdir(t, t.TempDir())
	var o, e bytes.Buffer
	onlyTTY(t, &e)
	code := run([]string{"run", folder, "--trace", "-", "--color", "never"}, strings.NewReader(""), &o, &e)
	if code != 1 {
		t.Fatalf("exit %d: %s", code, e.String())
	}
	if !strings.Contains(o.String(), `"sim.Failed"`) || strings.Contains(o.String(), " | ") {
		t.Errorf("the trace on stdout is wrong:\n%s", o.String())
	}
	if !strings.HasPrefix(e.String(), "failed at 0s: p/main.synod:6:29: error[division-by-zero]: division by zero\n") ||
		!strings.Contains(e.String(), "6 |") {
		t.Errorf("stderr:\n%s", e.String())
	}
}

// TestRunErrorsRich runs every case of testdata/run/errors with --style rich
// and compares stderr (the status stream with --trace -) with
// expect-rich.txt.
func TestRunErrorsRich(t *testing.T) {
	root, _ := filepath.Abs("../../testdata/run/errors")
	root = filepath.ToSlash(root)
	chdir(t, t.TempDir())
	n := 0
	err := filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || d.Name() != "expect.txt" {
			return err
		}
		dir := filepath.Dir(p)
		n++
		name, _ := filepath.Rel(root, dir)
		t.Run(filepath.ToSlash(name), func(t *testing.T) {
			code, _, stderr := exec("run", filepath.ToSlash(dir)+"/p", "--trace", "-", "--style", "rich", "--color", "never")
			if code != 1 {
				t.Fatalf("exit %d: %s", code, stderr)
			}
			if strings.Contains(stderr, "\x1b") || strings.Contains(stderr, "\r") {
				t.Errorf("escapes in the rich output")
			}
			short := strings.Split(strings.TrimSuffix(read(t, p), "\n"), "\n")
			if first, _, _ := strings.Cut(stderr, "\n"); first != short[len(short)-1] {
				t.Errorf("first line %q, want the failed line %q", first, short[len(short)-1])
			}
			golden := filepath.Join(dir, "expect-rich.txt")
			if *update {
				if err := os.WriteFile(golden, []byte(stderr), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			if want := read(t, golden); stderr != want {
				t.Errorf("differs from %s (run with -update)\n--- got\n%s--- want\n%s", golden, stderr, want)
			}
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if n == 0 {
		t.Fatal("no cases")
	}
}
