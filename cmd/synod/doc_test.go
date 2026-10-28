package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// docIn runs synod doc with dir (relative to this folder) as the working
// directory.
func docIn(t *testing.T, dir string, args ...string) (code int, stdout, stderr string) {
	t.Helper()
	abs, err := filepath.Abs(dir)
	if err != nil {
		t.Fatal(err)
	}
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(abs); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(old)
	var o, e bytes.Buffer
	code = runDoc(args, &o, &e)
	return code, o.String(), e.String()
}

// docGolden compares got with testdata/doc/<name>, or rewrites it with -update.
func docGolden(t *testing.T, name, got string) {
	t.Helper()
	file := filepath.Join("testdata", "doc", name)
	if *update {
		write(t, file, got)
		return
	}
	want, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("%v (run with -update)", err)
	}
	if string(want) != got {
		t.Errorf("%s differs (run with -update)\n--- got\n%.3000s\n--- want\n%.3000s", file, got, want)
	}
}

func okDoc(t *testing.T, dir string, args ...string) string {
	t.Helper()
	code, out, errs := docIn(t, dir, args...)
	if code != 0 || errs != "" {
		t.Fatalf("synod doc %v: exit %d, stderr %q, stdout %.300s", args, code, errs, out)
	}
	return out
}

func TestDocGoldens(t *testing.T) {
	for _, c := range []struct {
		name string
		dir  string
		args []string
	}{
		{"list.txt", examples, nil},
		{"std-net.txt", examples, []string{"std/net"}},
		{"std-net-rich.txt", examples, []string{"std/net", "--style", "rich", "--color", "never"}},
		{"std-net.json", examples, []string{"std/net", "--json"}},
		{"std-lesson.txt", examples, []string{"std/lesson"}},
		{"examples-twopc.txt", examples, []string{"twopc"}},
		{"examples-twopc.json", examples, []string{"twopc", "--json"}},
		{"scope-twopc-demo.json", examples, []string{"--json", "--scope", "twopc/demo.synod"}},
		{"alias-scope.json", "testdata/doc/alias", []string{"--json", "--scope", "app/app.synod"}},
		{"alias-list.txt", "testdata/doc/alias", nil},
		{"catalog.md", examples, []string{"--catalog"}},
	} {
		t.Run(c.name, func(t *testing.T) {
			out := okDoc(t, c.dir, c.args...)
			docGolden(t, c.name, out)
			for i, l := range strings.Split(out, "\n") {
				if strings.HasSuffix(l, " ") {
					t.Errorf("line %d has a trailing space: %q", i+1, l)
				}
			}
			if strings.HasSuffix(c.name, ".json") {
				var v map[string]any
				if err := json.Unmarshal([]byte(out), &v); err != nil || v["format"] != "synod-doc/1" {
					t.Errorf("not a synod-doc/1 document: %v", err)
				}
			}
		})
	}
}

func TestDocShortShape(t *testing.T) {
	out := okDoc(t, examples, "std/net")
	lines := strings.Split(out, "\n")
	if lines[0] != "net.Send [m: msg] from [a: node] to [b: node]" ||
		lines[1] != "    action; node; Send through the network from `a`, which must be the receiver" {
		t.Errorf("first entry: %q", lines[:2])
	}
	if !strings.Contains(out, "net.Id [m: msg]\n    gives number; look; The message id of the net fact that bound `m`\n") {
		t.Error("net.Id entry")
	}
	if strings.Contains(out, "\n\n") {
		t.Error("the short form has no blank lines")
	}
	lesson := okDoc(t, examples, "std/lesson")
	if !strings.Contains(lesson, "record lesson.Asked: Question text, Choices list of text\n") {
		t.Errorf("records: %s", lesson)
	}
	two := okDoc(t, examples, "twopc")
	for _, want := range []string{
		"twopc.Coordinate [ps: list of twopc.Node] from [c: twopc.Node]\n    action\n",
		"[n: twopc.Node] twopc.Decide [commit: bool]\n",
		"node twopc.Node: State text, Participants list of twopc.Node\n",
		"importing activates 0 observers, 1 invariants\n",
	} {
		if !strings.Contains(two, want) {
			t.Errorf("twopc lacks %q in\n%s", want, two)
		}
	}
}

func TestDocRichAndColor(t *testing.T) {
	out := okDoc(t, examples, "std/net", "--style", "rich", "--color", "never")
	if !strings.HasPrefix(out, "std/net — the simulated network\n\nnet.Send ") || !strings.Contains(out, "\n\nnet.Latency ") {
		t.Errorf("rich form: %.200q", out)
	}
	col := okDoc(t, examples, "std/net", "--style", "rich", "--color", "always")
	if !strings.Contains(col, "\x1b[1mstd/net — the simulated network\x1b[0m") || !strings.Contains(col, "\x1b[2m    action; node;") {
		t.Errorf("color: %.200q", col)
	}
	if plain := okDoc(t, examples, "std/net", "--color", "always"); strings.Contains(plain, "\x1b") {
		t.Error("piped output is the short form, without color")
	}
}

func TestDocImportPath(t *testing.T) {
	a := okDoc(t, examples, "github.com/synodlang/examples/twopc")
	b := okDoc(t, examples, "twopc")
	if a != b {
		t.Errorf("import path and folder differ:\n%s\n%s", a, b)
	}
	if c := okDoc(t, examples, "./twopc"); c != b {
		t.Error("./twopc differs from twopc")
	}
	// A vendored package is found by its import path.
	if v := okDoc(t, "testdata/doc/alias", "example.com/b/raft"); !strings.Contains(v, "[n: raft.Node] raft.Elect") {
		t.Errorf("vendored package: %s", v)
	}
}

func TestDocErrors(t *testing.T) {
	for _, c := range []struct {
		dir  string
		args []string
		want string
	}{
		{examples, []string{"net"}, "synod doc: no package \"net\"\n  hint: did you mean \"std/net\"?\n"},
		{examples, []string{"std/nope"}, "synod doc: no package \"std/nope\"\n"},
		{examples, []string{"nothere"}, "synod doc: no package \"nothere\"\n"},
		{examples, []string{"github.com/synodlang/examples/nothere"}, "synod doc: no package \"github.com/synodlang/examples/nothere\"\n  "},
		{examples, []string{"--scope", "twopc/demo.synod"}, "synod doc: --scope needs --json\n"},
		{examples, []string{"--catalog", "--json"}, "synod doc: --catalog takes no package and cannot be combined with --json or --scope\n"},
		{examples, []string{"--catalog", "std/net"}, "synod doc: --catalog takes no package and cannot be combined with --json or --scope\n"},
		{examples, []string{"--json", "--scope", "x.synod", "--catalog"}, "synod doc: --catalog takes no package and cannot be combined with --json or --scope\n"},
		{examples, []string{"--json", "--scope", "twopc/missing.synod"}, "synod doc: --scope twopc/missing.synod: no such .synod file\n"},
		{examples, []string{"a", "b"}, "synod doc: takes at most one package\n"},
		{examples, []string{"--style", "loud"}, "synod doc: --style needs rich or short, not \"loud\"\n"},
		{examples, []string{"--bogus"}, "synod doc: unknown flag \"--bogus\"\n"},
		{examples, []string{"--overlay", "nothere.json", "std/net"}, "synod doc: overlay: nothere.json: no such file\n"},
	} {
		code, out, errs := docIn(t, c.dir, c.args...)
		if code != 2 || out != "" || !strings.HasPrefix(errs, c.want) {
			t.Errorf("%v: exit %d, stdout %q, stderr %q, want exit 2 and %q", c.args, code, out, errs, c.want)
		}
	}
}

func TestDocMainOnlyFolder(t *testing.T) {
	tmp := t.TempDir()
	write(t, filepath.Join(tmp, "synod.mod"), "module example.com/m\nsynod 0.2\n")
	write(t, filepath.Join(tmp, "app", "app.synod"), "# package main\n\ndefine main\n  sim.StopAt 1s\n")
	code, _, errs := docIn(t, tmp, "app")
	if code != 2 || errs != "synod doc: app has no library package\n" {
		t.Errorf("exit %d, stderr %q", code, errs)
	}
}

// A package with an error prints the diagnostics as check does and exits 1.
func TestDocDiagnostics(t *testing.T) {
	tmp := t.TempDir()
	write(t, filepath.Join(tmp, "synod.mod"), "module example.com/m\nsynod 0.2\n")
	write(t, filepath.Join(tmp, "lib", "lib.synod"), "# package lib\n# Say hello\ndefine Hello\n  nothing here matches\n")
	code, out, errs := docIn(t, tmp, "lib", "--style", "short")
	if code != 1 || errs != "" || !strings.Contains(out, "lib/lib.synod:4:") || !strings.Contains(out, "error[unknown-phrase]") {
		t.Errorf("exit %d, stdout %q, stderr %q", code, out, errs)
	}
	_, check, _ := exec("check", "--style", "short", filepath.Join(tmp, "lib"))
	if !strings.Contains(check, "error[unknown-phrase]") {
		t.Fatalf("check says %q", check)
	}
	code, out, _ = docIn(t, tmp, "lib", "--json")
	var doc struct {
		Diagnostics []struct{ Code string } `json:"diagnostics"`
		Packages    []struct{ Path string } `json:"packages"`
	}
	if err := json.Unmarshal([]byte(out), &doc); err != nil || code != 1 || len(doc.Diagnostics) != 1 || doc.Diagnostics[0].Code != "unknown-phrase" || len(doc.Packages) != 1 {
		t.Errorf("json: exit %d, %v, %s", code, err, out)
	}
	// A file that does not parse leaves no package, only diagnostics.
	write(t, filepath.Join(tmp, "lib", "lib.synod"), "# package lib\n\ndefine (\n")
	code, out, _ = docIn(t, tmp, "lib", "--style", "short")
	if code != 1 || !strings.Contains(out, "error[syntax]") {
		t.Errorf("broken file: exit %d, %q", code, out)
	}
}

func overlayFor(t *testing.T, key, content string) string {
	t.Helper()
	tmp := t.TempDir()
	write(t, filepath.Join(tmp, "replacement.synod"), content)
	abs, err := filepath.Abs(key)
	if err != nil {
		t.Fatal(err)
	}
	doc := map[string]any{"format": "synod-overlay/1", "replace": map[string]string{abs: filepath.Join(tmp, "replacement.synod")}}
	data, _ := json.Marshal(doc)
	write(t, filepath.Join(tmp, "overlay.json"), string(data))
	return filepath.Join(tmp, "overlay.json")
}

// The overlay adds a use line: the scope gains it, though the file has
// errors (an unused import), and the exit code is 1.
func TestDocScopeOverlay(t *testing.T) {
	demo := read(t, filepath.Join(examples, "twopc", "demo.synod"))
	changed := strings.Replace(demo, "use std/net\n", "use std/lists\nuse std/net\n", 1)
	if changed == demo {
		t.Fatal("demo has no use std/net")
	}
	abs, _ := filepath.Abs(examples)
	old, _ := os.Getwd()
	os.Chdir(abs)
	ov := overlayFor(t, "twopc/demo.synod", changed)
	os.Chdir(old)
	code, out, errs := docIn(t, examples, "--json", "--scope", "twopc/demo.synod", "--overlay", ov)
	if code != 1 || errs != "" {
		t.Fatalf("exit %d, stderr %q", code, errs)
	}
	docGolden(t, "scope-overlay.json", out)
	var doc struct {
		Scope struct{ Imports []struct{ Qualifier string } }
	}
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatal(err)
	}
	var quals []string
	for _, i := range doc.Scope.Imports {
		quals = append(quals, i.Qualifier)
	}
	if strings.Join(quals, " ") != "twopc lesson lists net sim" {
		t.Errorf("imports %v", quals)
	}
}

// A scope of a library file lists the package's own names with bare heads,
// unexported ones included.
func TestDocScopeOwnPackage(t *testing.T) {
	out := okDoc(t, examples, "--json", "--scope", "raft/raft.synod")
	var doc struct {
		Packages []struct {
			Path    string
			Phrases []struct {
				Pattern  string
				Exported bool
			}
			Node struct{ Fields []struct{ Name string } }
		}
	}
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatal(err)
	}
	own := doc.Packages[0]
	if !strings.HasSuffix(own.Path, "/raft") {
		t.Fatalf("own package: %s", own.Path)
	}
	bare, unexported := false, false
	for _, p := range own.Phrases {
		bare = bare || strings.HasPrefix(p.Pattern, "Meet ")
		unexported = unexported || !p.Exported
	}
	if !bare {
		t.Error("the own package has no bare head")
	}
	_ = unexported
	// Unexported names are listed in the scope and not in the package doc.
	tmp := t.TempDir()
	write(t, filepath.Join(tmp, "synod.mod"), "module example.com/m\nsynod 0.2\n")
	write(t, filepath.Join(tmp, "lib", "lib.synod"), "# package lib\n# Shown\ndefine Shown\n  result 1\n\n# Hidden\ndefine hidden\n  result 2\n")
	if s := okDoc(t, tmp, "--json", "--scope", "lib/lib.synod"); !strings.Contains(s, `"pattern":"hidden"`) || !strings.Contains(s, `"pattern":"Shown"`) {
		t.Errorf("scope: %s", s)
	}
	if s := okDoc(t, tmp, "lib"); strings.Contains(s, "hidden") || !strings.Contains(s, "lib.Shown") {
		t.Errorf("doc: %s", s)
	}
}

func TestDocScopeSpeed(t *testing.T) {
	start := time.Now()
	okDoc(t, examples, "--json", "--scope", "cassandra/demo.synod")
	if d := time.Since(start); d > time.Second {
		t.Errorf("doc --scope took %v", d)
	} else {
		t.Logf("doc --scope cassandra/demo.synod: %v", d)
	}
}

func TestDocListWithoutModule(t *testing.T) {
	tmp := t.TempDir()
	write(t, filepath.Join(tmp, "x.synod"), "# package x\n")
	out := okDoc(t, tmp)
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 15 || lines[0] != "std/disk  storage that survives crashes" || lines[14] != "." {
		t.Errorf("list: %q", lines)
	}
}
