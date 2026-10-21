package main

import (
	"os"
	"path/filepath"
	"regexp"
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
	sc := filepath.Join(dir, "out", "s.json")
	code, stdout, stderr := exec("run", folder, "--trace", out, "--scene", sc)
	if code != 0 || stderr != "" {
		t.Fatalf("exit %d, stderr %q, stdout %q", code, stderr, stdout)
	}
	want := "ok seed 5: empty at 22.315ms, 1 nodes, 7 events -> " + out + "\n"
	if i := strings.Index(stdout, "scene "); i < 0 || !strings.HasSuffix(stdout, " -> "+sc+"\n") {
		t.Errorf("no scene line in %q", stdout)
	} else {
		stdout = stdout[:i]
	}
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
	chdir(t, dir)
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
		{"scene empty", []string{"run", folder, "--scene", ""}, "--scene needs a file name"},
		{"scene missing", []string{"run", folder, "--scene"}, "--scene needs a value"},
		{"both dash", []string{"run", folder, "--trace", "-", "--scene", "-"}, `synod run: --trace and --scene cannot both be "-"`},
		{"pace low", []string{"run", folder, "--pace", "0.2"}, `synod run: --pace needs a number from 0.25 to 4, not "0.2"`},
		{"pace high", []string{"run", folder, "--pace=5"}, `synod run: --pace needs a number from 0.25 to 4, not "5"`},
		{"pace text", []string{"run", folder, "--pace", "abc"}, `synod run: --pace needs a number from 0.25 to 4, not "abc"`},
		{"pace digits", []string{"run", folder, "--pace", "1.2345"}, `--pace needs a number from 0.25 to 4, not "1.2345"`},
		{"pace sign", []string{"run", folder, "--pace", "+1"}, `--pace needs a number from 0.25 to 4, not "+1"`},
		{"pace exp", []string{"run", folder, "--pace", "1e0"}, `--pace needs a number from 0.25 to 4, not "1e0"`},
		{"pace dot", []string{"run", folder, "--pace", "1."}, `--pace needs a number from 0.25 to 4, not "1."`},
		{"pace empty", []string{"run", folder, "--pace", ""}, `--pace needs a number from 0.25 to 4, not ""`},
		{"mode", []string{"run", folder, "--mode", "fast"}, `synod run: --mode needs ordered or proportional, not "fast"`},
		{"mode missing", []string{"run", folder, "--mode"}, "--mode needs a value"},
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
	lines := strings.Split(strings.TrimSuffix(stdout, "\n"), "\n")
	if len(lines) != 2 || !strings.HasSuffix(lines[0], " events -> out/trace.jsonl") ||
		!regexp.MustCompile(`^scene [0-9.]+m?s, \d+ shapes -> out/scene\.json$`).MatchString(lines[1]) {
		t.Errorf("stdout %q", stdout)
	}
	if sc := read(t, filepath.Join(dir, "out", "scene.json")); !strings.HasPrefix(sc, `{"format":"synod-scene/1",`) {
		t.Errorf("scene %.60s", sc)
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
	if !strings.Contains(stderr, "\nscene ") || !strings.HasSuffix(stderr, " -> out/scene.json\n") {
		t.Errorf("stderr %q", stderr)
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
	if _, err := os.Stat(filepath.Join(dir, "out", "trace.jsonl")); err == nil {
		t.Error("--trace - wrote a trace file")
	}
}

func TestRunSameFromAnyDirectory(t *testing.T) {
	dir := t.TempDir()
	folder := runModule(t, dir, runLib, runMain)
	chdir(t, t.TempDir())
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
	chdir(t, dir)
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
	code, stdout, stderr := exec("run", folder, "--trace", filepath.Join(dir, "t.jsonl"), "--scene", filepath.Join(dir, "s.json"))
	if code != 0 {
		t.Fatalf("exit %d: %s%s", code, stdout, stderr)
	}
	lines := strings.Split(strings.TrimSuffix(stdout, "\n"), "\n")
	if len(lines) != 3 || !strings.HasPrefix(lines[0], "ok seed 5: ") || !strings.HasPrefix(lines[2], "scene ") {
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
	sc := filepath.Join(dir, "s.json")
	code, stdout, stderr := exec("run", folder, "--summary", "--trace", out, "--scene", sc)
	if code != 0 || stderr != "" {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	if strings.Contains(stdout, "ok seed") {
		t.Error("the status line is printed with --summary")
	}
	if _, err := os.Stat(out); err != nil {
		t.Error("--summary did not write the trace")
	}
	if _, err := os.Stat(sc); err != nil {
		t.Error("--summary did not write the scene")
	}
	if strings.Contains(stdout, "scene ") {
		t.Error("--summary prints a scene line")
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
	if code != 0 || !strings.Contains(stdout, "run <folder> [--seed n] [--trace file] [--scene file] [--pace x] [--mode m] [--summary]") {
		t.Errorf("usage %q", stdout)
	}
}

func TestRunSceneToStdout(t *testing.T) {
	dir := t.TempDir()
	folder := runModule(t, dir, runLib, runMain)
	chdir(t, dir)
	code, stdout, stderr := exec("run", folder, "--scene", "-", "--trace", filepath.Join(dir, "t.jsonl"))
	if code != 0 {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	if !strings.HasPrefix(stdout, `{"format":"synod-scene/1"`) || !strings.HasSuffix(stdout, "]}\n") || strings.Contains(stdout, "ok seed") {
		t.Errorf("stdout %.80q", stdout)
	}
	if !strings.HasPrefix(stderr, "ok seed 5: ") || !strings.HasSuffix(stderr, " shapes -> stdout\n") {
		t.Errorf("stderr %q", stderr)
	}
	if _, err := os.Stat(filepath.Join(dir, "out")); err == nil {
		t.Error("--scene - wrote a file")
	}
}

func TestRunScenePaceAndMode(t *testing.T) {
	dir := t.TempDir()
	folder := runModule(t, dir, runLib, runMain)
	tr := filepath.Join(dir, "t.jsonl")
	dur := func(args ...string) string {
		code, stdout, stderr := exec(append([]string{"run", folder, "--trace", tr, "--scene", "-"}, args...)...)
		if code != 0 {
			t.Fatalf("%v: exit %d: %s", args, code, stderr)
		}
		return stdout
	}
	a, b := dur(), dur("--pace", "1")
	if a != b {
		t.Error("--pace 1 differs from the default")
	}
	if dur("--pace", "2") == a || dur("--pace=0.25") == a || dur("--mode", "proportional") == a {
		t.Error("pace or mode did not change the scene")
	}
	if dur("--mode", "ordered") != a {
		t.Error("--mode ordered differs from the default")
	}
	if dur("--pace", "1.5") != dur("--pace", "1.50") {
		t.Error("1.5 and 1.50 differ")
	}
}

func TestRunSceneFileAndFolders(t *testing.T) {
	dir := t.TempDir()
	folder := runModule(t, dir, runLib, runMain)
	sc := filepath.Join(dir, "deep", "er", "s.json")
	code, stdout, _ := exec("run", folder, "--trace", filepath.Join(dir, "t.jsonl"), "--scene="+sc)
	if code != 0 || !strings.HasSuffix(stdout, " -> "+sc+"\n") {
		t.Fatalf("exit %d: %q", code, stdout)
	}
	if !strings.HasPrefix(read(t, sc), `{"format":"synod-scene/1","duration":`) {
		t.Error("scene file")
	}
}

func lookFault(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.CopyFS(dir, os.DirFS("testdata/lookfault")); err != nil {
		t.Fatal(err)
	}
	return filepath.Join(dir, "main")
}

func TestRunLookFault(t *testing.T) {
	folder := lookFault(t)
	dir := filepath.Dir(folder)
	tr, sc := filepath.Join(dir, "t.jsonl"), filepath.Join(dir, "s.json")
	code, stdout, stderr := exec("run", folder, "--trace", tr, "--scene", sc, "--style", "short")
	if code != 1 || stderr != "" {
		t.Fatalf("exit %d, stderr %q", code, stderr)
	}
	lines := strings.Split(strings.TrimSuffix(stdout, "\n"), "\n")
	if len(lines) != 2 || !strings.HasPrefix(lines[0], "ok seed ") ||
		!strings.HasPrefix(lines[1], "look failed at ") ||
		!strings.Contains(lines[1], ": lk/lk.synod:") || !strings.HasSuffix(lines[1], `error[missing-key]: no shape is attached to "nope" under "card"`) {
		t.Errorf("stdout %q", stdout)
	}
	if strings.Contains(stdout, "scene ") {
		t.Error("scene line after a look fault")
	}
	if strings.Contains(read(t, tr), "look failed") {
		t.Error("the look fault is in the trace")
	}
	if s := read(t, sc); !strings.Contains(s, `"kind":"failed"`) {
		t.Errorf("no failed marker in %.200s", s)
	}
	// Rich style, no escapes with --color never.
	code, stdout, _ = exec("run", folder, "--trace", tr, "--scene", sc, "--style", "rich", "--color", "never")
	if code != 1 || !strings.Contains(stdout, "look failed at ") || !strings.Contains(stdout, "nope") || strings.Contains(stdout, "\x1b") ||
		strings.Count(stdout, "\n") < 4 {
		t.Errorf("rich stdout %q", stdout)
	}
	// --summary: the summary text, then the failure line.
	_, plain, _ := exec("run", folder, "--trace", tr, "--scene", sc, "--summary", "--style", "short")
	code, stdout, _ = exec("run", folder, "--trace", tr, "--scene", sc, "--summary", "--style", "short")
	if code != 1 || !strings.HasPrefix(stdout, strings.TrimSuffix(plain, plain[strings.LastIndex(strings.TrimSuffix(plain, "\n"), "\n")+1:])) ||
		!strings.Contains(stdout[strings.LastIndex(strings.TrimSuffix(stdout, "\n"), "\n"):], "look failed at") {
		t.Errorf("summary stdout %q", stdout)
	}
}
