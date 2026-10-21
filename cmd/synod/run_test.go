package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/synodlang/compiler/internal/ir"
	"github.com/synodlang/compiler/internal/sim"
)

const runMod = "module example.com/t\nsynod 0.2\n"

const runLib = `# package p
use std/net
use std/random
use std/sim

Ping has
  set n to 0

Node has
  set Got to 0

when [n: Node] (sim.Delivered Ping)
  net.Send (message Ping with n 1) from n to n

when [n: Node] (net.Received Ping from who)
  set n.Got to (random.Between 0 and 1000)

always (1 is 1)
`

const runMain = `# package main
use example.com/t/p
use std/sim

define main
  sim.Seed 5
  set a to (sim.Spawn p)
  deliver (message p.Ping with n 0) to a
  sim.StopAt 1s
`

// runModule writes a module with a library and a main file below dir and
// returns the folder of the main file.
func runModule(t *testing.T, dir, lib, main string) string {
	t.Helper()
	write(t, filepath.Join(dir, "synod.mod"), runMod)
	write(t, filepath.Join(dir, "p", "p.synod"), lib)
	write(t, filepath.Join(dir, "p", "main.synod"), main)
	return filepath.Join(dir, "p")
}

func TestRunOK(t *testing.T) {
	dir := t.TempDir()
	folder := runModule(t, dir, runLib, runMain)
	out := filepath.Join(dir, "out", "t.jsonl")
	code, stdout, stderr := exec("run", folder, "--trace", out)
	if code != 0 || stderr != "" {
		t.Fatalf("exit %d, stderr %q, stdout %q", code, stderr, stdout)
	}
	want := "ok seed 5: empty at 22.315ms, 1 nodes, 7 events -> " + out + "\n"
	if stdout != want {
		t.Errorf("stdout %q, want %q", stdout, want)
	}
	tr := read(t, out)
	lines := strings.Split(strings.TrimSuffix(tr, "\n"), "\n")
	if !strings.HasPrefix(lines[0], `{"format":"synod-trace/1","synod":"0.2","seed":5,"program":"example.com/t/p",`) {
		t.Errorf("header %s", lines[0])
	}
	if last := lines[len(lines)-1]; !strings.Contains(last, `"ev":"sim.Ended","reason":"empty"`) {
		t.Errorf("last line %s", last)
	}
}

func TestRunFlagForms(t *testing.T) {
	dir := t.TempDir()
	folder := runModule(t, dir, runLib, runMain)
	traces := map[string]string{}
	for name, args := range map[string][]string{
		"before-space": {"--seed", "7", "--trace", filepath.Join(dir, "a.jsonl"), folder},
		"after-space":  {folder, "--seed", "7", "--trace", filepath.Join(dir, "b.jsonl")},
		"equals":       {"--seed=7", "--trace=" + filepath.Join(dir, "c.jsonl"), folder},
		"mixed":        {"--seed=7", folder, "--trace", filepath.Join(dir, "d.jsonl")},
	} {
		code, stdout, stderr := exec(append([]string{"run"}, args...)...)
		if code != 0 {
			t.Fatalf("%s: exit %d: %s%s", name, code, stdout, stderr)
		}
		if !strings.HasPrefix(stdout, "ok seed 7: ") {
			t.Errorf("%s: stdout %q", name, stdout)
		}
		file := map[string]string{"before-space": "a", "after-space": "b", "equals": "c", "mixed": "d"}[name]
		traces[name] = read(t, filepath.Join(dir, file+".jsonl"))
	}
	for name, tr := range traces {
		if tr != traces["equals"] {
			t.Errorf("%s: trace differs", name)
		}
		if !strings.Contains(tr, `"seed":7,`) {
			t.Errorf("%s: seed not 7", name)
		}
	}
	// Without --seed the program's sim.Seed counts; --seed 0 is a real seed.
	exec("run", folder, "--trace", filepath.Join(dir, "e.jsonl"))
	if !strings.Contains(read(t, filepath.Join(dir, "e.jsonl")), `"seed":5,`) {
		t.Error("sim.Seed 5 not used")
	}
	exec("run", folder, "--seed", "0", "--trace", filepath.Join(dir, "f.jsonl"))
	if !strings.Contains(read(t, filepath.Join(dir, "f.jsonl")), `"seed":0,`) {
		t.Error("--seed 0 not used")
	}
}

func TestRunUsageErrors(t *testing.T) {
	dir := t.TempDir()
	folder := runModule(t, dir, runLib, runMain)
	cases := []struct {
		name string
		args []string
		msg  string
	}{
		{"no folder", []string{"run"}, "needs a folder"},
		{"two folders", []string{"run", folder, folder}, "exactly one folder"},
		{"unknown flag", []string{"run", folder, "--frob"}, "unknown flag"},
		{"bad seed", []string{"run", folder, "--seed", "abc"}, "--seed needs a whole number"},
		{"negative seed", []string{"run", folder, "--seed=-1"}, "--seed needs a whole number"},
		{"seed too large", []string{"run", folder, "--seed", "9007199254740993"}, "--seed needs a whole number"},
		{"seed missing", []string{"run", folder, "--seed"}, "--seed needs a value"},
		{"trace missing", []string{"run", folder, "--trace"}, "--trace needs a value"},
		{"summary value", []string{"run", folder, "--summary=1"}, "takes no value"},
		{"missing folder", []string{"run", filepath.Join(dir, "nope")}, "nope"},
		{"file not folder", []string{"run", filepath.Join(folder, "main.synod")}, "not a folder"},
		{"scene", []string{"run", folder, "--scene", "x"}, "synod run: --scene is not implemented yet"},
		{"scene equals", []string{"run", folder, "--scene=x"}, "synod run: --scene is not implemented yet"},
		{"pace", []string{"run", folder, "--pace", "2"}, "synod run: --pace is not implemented yet"},
		{"mode", []string{"run", folder, "--mode", "ordered"}, "synod run: --mode is not implemented yet"},
		{"watch", []string{"run", folder, "--watch"}, "synod run: --watch is not implemented yet"},
		{"json-events", []string{"run", folder, "--json-events"}, "synod run: --json-events is not implemented yet"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			code, stdout, stderr := exec(c.args...)
			if code != 2 {
				t.Errorf("exit %d", code)
			}
			if stdout != "" {
				t.Errorf("stdout %q", stdout)
			}
			if !strings.Contains(stderr, c.msg) {
				t.Errorf("stderr %q, want %q", stderr, c.msg)
			}
		})
	}
	if _, err := os.Stat(filepath.Join(dir, "out")); err == nil {
		t.Error("a usage error wrote a trace")
	}
}

// chdir changes the working directory for the test.
func chdir(t *testing.T, dir string) {
	t.Helper()
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(old) })
}

func TestRunDefaultTracePath(t *testing.T) {
	dir := t.TempDir()
	runModule(t, dir, runLib, runMain)
	chdir(t, dir)
	code, stdout, stderr := exec("run", "p")
	if code != 0 || stderr != "" {
		t.Fatalf("exit %d: %s%s", code, stdout, stderr)
	}
	if !strings.HasSuffix(stdout, " events -> out/trace.jsonl\n") {
		t.Errorf("stdout %q", stdout)
	}
	if tr := read(t, filepath.Join(dir, "out", "trace.jsonl")); !strings.HasPrefix(tr, `{"format":"synod-trace/1"`) {
		t.Errorf("trace %.60s", tr)
	}
}

func TestRunTraceToStdout(t *testing.T) {
	dir := t.TempDir()
	folder := runModule(t, dir, runLib, runMain)
	chdir(t, dir)
	code, stdout, stderr := exec("run", folder, "--trace", "-")
	if code != 0 {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	if !strings.HasPrefix(stdout, `{"format":"synod-trace/1"`) || !strings.HasSuffix(stdout, "\n") {
		t.Errorf("stdout %.80q", stdout)
	}
	if strings.Contains(stdout, "ok seed") {
		t.Error("the status line is on stdout")
	}
	if !strings.HasPrefix(stderr, "ok seed 5: ") {
		t.Errorf("stderr %q", stderr)
	}
	if _, err := os.Stat(filepath.Join(dir, "out")); err == nil {
		t.Error("--trace - wrote a file")
	}
}

func TestRunSameFromAnyDirectory(t *testing.T) {
	dir := t.TempDir()
	folder := runModule(t, dir, runLib, runMain)
	_, a, _ := exec("run", folder, "--trace", "-")
	chdir(t, dir)
	_, b, _ := exec("run", "p", "--trace", "-")
	chdir(t, filepath.Join(dir, "p"))
	_, c, _ := exec("run", ".", "--trace", "-")
	if a != b || a != c {
		t.Error("the trace depends on the working directory")
	}
}

func TestRunDiagnostics(t *testing.T) {
	dir := t.TempDir()
	folder := runModule(t, dir, runLib, strings.Replace(runMain, "sim.StopAt 1s", "sim.StopAt (1 plus 1s)", 1))
	out := filepath.Join(dir, "out.jsonl")
	code, stdout, _ := exec("run", folder, "--trace", out)
	if code != 1 {
		t.Fatalf("exit %d", code)
	}
	if !strings.Contains(stdout, "error[") {
		t.Errorf("stdout %q", stdout)
	}
	if _, err := os.Stat(out); err == nil {
		t.Error("a trace was written for a program with diagnostics")
	}
}

func TestRunLibraryFolder(t *testing.T) {
	dir := t.TempDir()
	runModule(t, dir, runLib, runMain)
	lib := filepath.Join(dir, "lib")
	write(t, filepath.Join(lib, "lib.synod"), "# package lib\nX has\n  set a to 0\n")
	out := filepath.Join(dir, "out.jsonl")
	code, stdout, _ := exec("run", lib, "--trace", out)
	want := lib + ": error[missing-main]: no main file in " + lib + "\n"
	if code != 1 || stdout != filepath.ToSlash(want) && stdout != want {
		t.Errorf("exit %d, stdout %q, want %q", code, stdout, want)
	}
	if _, err := os.Stat(out); err == nil {
		t.Error("a trace was written")
	}
}

func TestRunFailure(t *testing.T) {
	dir := t.TempDir()
	main := strings.Replace(runMain, "sim.StopAt 1s", "emit (message p.Ping with n (1 divided by (2 minus 2))) by sim.Nobody", 1)
	folder := runModule(t, dir, runLib, main)
	out := filepath.Join(dir, "out.jsonl")
	code, stdout, _ := exec("run", folder, "--trace", out)
	if code != 1 {
		t.Fatalf("exit %d", code)
	}
	want := "failed at 0s: p/main.synod:9:3: error[division-by-zero]: division by zero\n"
	if !strings.HasPrefix(stdout, "failed at 0s: ") || !strings.Contains(stdout, "error[division-by-zero]: division by zero") {
		t.Errorf("stdout %q, want like %q", stdout, want)
	}
	if strings.Contains(stdout, dir) {
		t.Errorf("the position is not module-relative: %q", stdout)
	}
	lines := strings.Split(strings.TrimSuffix(read(t, out), "\n"), "\n")
	if !strings.Contains(lines[len(lines)-1], `"ev":"sim.Failed","code":"division-by-zero"`) {
		t.Errorf("last line %s", lines[len(lines)-1])
	}
}

func TestRunBrokenInvariantExitsZero(t *testing.T) {
	dir := t.TempDir()
	lib := strings.Replace(runLib, "always (1 is 1)", "always Fine", 1) + `
define Fine
  result (time.Now less than 100ms)
`
	lib = strings.Replace(lib, "use std/sim\n", "use std/sim\nuse std/time\n", 1)
	main := strings.Replace(runMain, "sim.StopAt 1s", "after 200ms\n    deliver (message p.Ping with n 0) to a\n  sim.StopAt 1s", 1)
	folder := runModule(t, dir, lib, main)
	code, stdout, stderr := exec("run", folder, "--trace", filepath.Join(dir, "t.jsonl"))
	if code != 0 {
		t.Fatalf("exit %d: %s%s", code, stdout, stderr)
	}
	lines := strings.Split(strings.TrimSuffix(stdout, "\n"), "\n")
	if len(lines) != 2 || !strings.HasPrefix(lines[0], "ok seed 5: ") {
		t.Fatalf("stdout %q", stdout)
	}
	if !strings.HasPrefix(lines[1], "broken at ") || !strings.Contains(lines[1], " (1 time): p/p.synod:") || !strings.HasSuffix(lines[1], ": always Fine") && !strings.Contains(lines[1], "always") {
		t.Errorf("broken line %q", lines[1])
	}
}

func TestRunSummary(t *testing.T) {
	dir := t.TempDir()
	folder := runModule(t, dir, runLib, runMain)
	out := filepath.Join(dir, "t.jsonl")
	code, stdout, stderr := exec("run", folder, "--summary", "--trace", out)
	if code != 0 || stderr != "" {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	if strings.Contains(stdout, "ok seed") {
		t.Error("the status line is printed with --summary")
	}
	if _, err := os.Stat(out); err != nil {
		t.Error("--summary did not write the trace")
	}
	golden := filepath.Join("testdata", "run-summary.txt")
	if *update {
		if err := os.WriteFile(golden, []byte(stdout), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if want := read(t, golden); stdout != want {
		t.Errorf("summary differs from %s (run with -update)\n--- got\n%s--- want\n%s", golden, stdout, want)
	}
}

func TestRunInternalError(t *testing.T) {
	dir := t.TempDir()
	folder := runModule(t, dir, runLib, runMain)
	old := simRun
	simRun = func(*ir.Program, sim.Options) *sim.Result { panic("boom") }
	defer func() { simRun = old }()
	code, stdout, stderr := exec("run", folder, "--trace", filepath.Join(dir, "t.jsonl"))
	if code != 2 || stdout != "" || stderr != "synod run: internal error: boom\n" {
		t.Errorf("exit %d, stdout %q, stderr %q", code, stdout, stderr)
	}
}

func TestRunUsageListsRun(t *testing.T) {
	code, stdout, _ := exec("help")
	if code != 0 || !strings.Contains(stdout, "run <folder> [--seed n] [--trace file] [--summary]") {
		t.Errorf("usage %q", stdout)
	}
}
