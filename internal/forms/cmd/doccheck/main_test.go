package main

import (
	"flag"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/synodlang/compiler/internal/forms"
)

var update = flag.Bool("update", false, "rewrite the golden summaries and the Library index of testdata")

var synodBin string

func TestMain(m *testing.M) {
	flag.Parse()
	dir, err := os.MkdirTemp("", "doccheck-bin")
	if err != nil {
		panic(err)
	}
	synodBin = filepath.Join(dir, "synod")
	if runtime.GOOS == "windows" {
		synodBin += ".exe"
	}
	if out, err := exec.Command("go", "build", "-o", synodBin, "github.com/synodlang/compiler/cmd/synod").CombinedOutput(); err != nil {
		os.Stderr.Write(out)
		os.RemoveAll(dir)
		os.Exit(1)
	}
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

// tutorialConfig copies testdata/dsl to a temporary folder and returns the
// configuration for it, whose catalog is the items the mini coverage names.
func tutorialConfig(t *testing.T) *config {
	t.Helper()
	root := t.TempDir()
	dsl := filepath.Join(root, "dsl")
	if err := copyDir("testdata/dsl", dsl); err != nil {
		t.Fatal(err)
	}
	cfg := &config{
		synod: synodBin, tutorial: filepath.Join(dsl, "tutorial"), spec: filepath.Join(dsl, "spec"),
		chapters: 2, newChapter: 2, update: *update,
	}
	text, err := os.ReadFile(filepath.Join(cfg.tutorial, "coverage.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, it := range forms.Catalog() {
		if strings.Contains(string(text), "`"+it.Key()+"`") {
			cfg.catalog = append(cfg.catalog, it)
		}
	}
	return cfg
}

func copyDir(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, path)
		if d.IsDir() {
			return os.MkdirAll(filepath.Join(dst, rel), 0o755)
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(dst, rel), b, 0o644)
	})
}

func (c *config) check(library bool) string {
	var p problems
	run(c, &p, library)
	return strings.Join(p.list, "\n")
}

// edit replaces old by new in the file (relative to the tutorial folder).
func edit(t *testing.T, cfg *config, file, old, new string) {
	t.Helper()
	path := filepath.Join(cfg.tutorial, filepath.FromSlash(file))
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), old) {
		t.Fatalf("%s has no %q", file, old)
	}
	if err := os.WriteFile(path, []byte(strings.Replace(string(b), old, new, 1)), 0o644); err != nil {
		t.Fatal(err)
	}
}

func wantProblem(t *testing.T, got string, subs ...string) {
	t.Helper()
	for _, s := range subs {
		if !strings.Contains(got, s) {
			t.Errorf("no problem containing %q in:\n%s", s, got)
		}
	}
	if got == "" {
		t.Error("expected problems, found none")
	}
}

func TestFixture(t *testing.T) {
	if *update {
		cfg := tutorialConfig(t)
		cfg.check(false)
		// Copy the goldens back into testdata.
		for _, g := range []string{"01-hello/step1", "01-hello/step2", "02-counter/step1", "02-counter/step2"} {
			b, err := os.ReadFile(filepath.Join(cfg.tutorial, "code", g+".summary"))
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join("testdata/dsl/tutorial/code", g+".summary"), b, 0o644); err != nil {
				t.Fatal(err)
			}
		}
		return
	}
	cfg := tutorialConfig(t)
	if got := cfg.check(false); got != "" {
		t.Fatalf("the mini tutorial must pass:\n%s", got)
	}
}

func TestEarlyUse(t *testing.T) {
	cfg := tutorialConfig(t)
	edit(t, cfg, "code/01-hello/step2/hello/hello.synod", "use std/lesson\n", "use std/lesson\nuse std/text\n")
	edit(t, cfg, "code/01-hello/step2/hello/hello.synod", `  lesson.Note "Hello again"`, "  set x to (1 plus 2)\n  lesson.Note (text.From x)")
	cfg.update = true
	wantProblem(t, cfg.check(false), "chapter 1 uses form:plus, which chapter 2 introduces", "chapter 1 uses type:number")
}

func TestMissingRow(t *testing.T) {
	cfg := tutorialConfig(t)
	edit(t, cfg, "coverage.md", "| `form:plus` | prose | spec/01 §1.2 | 2 |\n", "")
	wantProblem(t, cfg.check(false), "no row for form:plus")
}

func TestDuplicateRow(t *testing.T) {
	cfg := tutorialConfig(t)
	row := "| `form:plus` | prose | spec/01 §1.2 | 2 |\n"
	edit(t, cfg, "coverage.md", row, row+row)
	wantProblem(t, cfg.check(false), "duplicate row for form:plus")
}

func TestBadRows(t *testing.T) {
	cfg := tutorialConfig(t)
	edit(t, cfg, "coverage.md", "| `form:plus` | prose | spec/01 §1.2 | 2 |", "| `form:plus` | prose | spec/01 §9.9 | 7 |")
	wantProblem(t, cfg.check(false), "no heading 9.9", "chapter 7 is outside 1..2")
}

func TestTaughtButUnused(t *testing.T) {
	cfg := tutorialConfig(t)
	for _, id := range []string{"form:if", "form:define-native"} {
		edit(t, cfg, "coverage.md", "| `form:plus`", "| `"+id+"` | prose | spec/01 §1.2 | 2 |\n| `form:plus`")
	}
	for _, it := range forms.Catalog() {
		if it.Key() == "form:if" || it.Key() == "form:define-native" {
			cfg.catalog = append(cfg.catalog, it)
		}
	}
	got := cfg.check(false)
	wantProblem(t, got, "form:if is taught in chapter 2, but no step of chapter 2 uses it")
	if strings.Contains(got, "define-native is taught") {
		t.Errorf("a prose-only item needs no step:\n%s", got)
	}
}

func TestStdReference(t *testing.T) {
	cfg := tutorialConfig(t)
	cfg.spec = filepath.Join(cfg.tutorial, "..", "spec")
	good, bad := "std/README.md § std/time", "std/README.md § std/nope"
	if b := badRefs(cfg, good, false); len(b) > 0 {
		t.Errorf("%q: %v", good, b)
	}
	if b := badRefs(cfg, bad, false); len(b) == 0 {
		t.Errorf("%q accepted", bad)
	}
}

func TestBadPhraseRow(t *testing.T) {
	cfg := tutorialConfig(t)
	edit(t, cfg, "coverage.md", "phrase:lesson.Note [t: text]", "phrase:lesson.Nope [t: text]")
	wantProblem(t, cfg.check(false), `"lesson.Nope [t: text]" is no exported std pattern`)
}

func TestStaleGolden(t *testing.T) {
	cfg := tutorialConfig(t)
	path := filepath.Join(cfg.tutorial, "code/01-hello/step1.summary")
	if err := os.WriteFile(path, []byte("run hello seed 1\nend: other\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	wantProblem(t, cfg.check(false), "step1.summary: stale")
	cfg.update = true
	cfg.check(false)
	cfg.update = false
	if got := cfg.check(false); got != "" {
		t.Errorf("after -update:\n%s", got)
	}
}

func TestWhyWrongCode(t *testing.T) {
	cfg := tutorialConfig(t)
	edit(t, cfg, "code/01-hello/why-indent/expect", "indent", "syntax")
	wantProblem(t, cfg.check(false), "expected syntax, synod check reports [indent]")
}

func TestWhyPasses(t *testing.T) {
	cfg := tutorialConfig(t)
	edit(t, cfg, "code/02-counter/why-unused-name/p/p.synod", "  set x to 1\n", "")
	wantProblem(t, cfg.check(false), "expected unused-name, but synod check passes")
}

func TestNotSynodNew(t *testing.T) {
	cfg := tutorialConfig(t)
	edit(t, cfg, "code/02-counter/step1/tutorial/tutorial.synod", "# Count up", "# Count up,")
	wantProblem(t, cfg.check(false), "tutorial.synod: differs from `synod new")
}

func TestChapterStructure(t *testing.T) {
	cfg := tutorialConfig(t)
	edit(t, cfg, "01-hello.md", "## Goal\n", "")
	edit(t, cfg, "02-counter.md", "<!-- step 2 program=tutorial -->", "<!-- step 2 program=nothing -->")
	wantProblem(t, cfg.check(false), `missing heading "## Goal"`, `program "nothing" is no folder`)
}

func TestWhyWithoutDirective(t *testing.T) {
	cfg := tutorialConfig(t)
	edit(t, cfg, "01-hello.md", "<!-- why indent -->", "")
	wantProblem(t, cfg.check(false), "no `<!-- why indent -->` directive")
}

func TestRequireVersion(t *testing.T) {
	cfg := tutorialConfig(t)
	cfg.pkgChapter = 2
	edit(t, cfg, "code/02-counter/step2/synod.mod", "synod 0.2\n", "synod 0.2\nrequire "+examplesModule+" v0.1.0\n")
	wantProblem(t, cfg.check(false), "requires "+examplesModule+" v0.1.0, versions.env pins EXAMPLES_VERSION v0.2.1")
}

func TestExampleCopy(t *testing.T) {
	root := t.TempDir()
	ex := filepath.Join(root, "examples")
	last := filepath.Join(root, "step")
	files := map[string][2]string{
		"raft/raft.synod":         {"# package raft\n", "# package raft\n"},
		"raft/demo.synod":         {"use " + examplesModule + "/raft\n", "use example.com/tutorial/raft\n"},
		"raftlook/raftlook.synod": {"use " + examplesModule + "/raft\n", "use example.com/tutorial/raft\n"},
	}
	for name, c := range files {
		for dir, text := range map[string]string{ex: c[0], last: c[1]} {
			p := filepath.Join(dir, filepath.FromSlash(name))
			os.MkdirAll(filepath.Dir(p), 0o755)
			os.WriteFile(p, []byte(text), 0o644)
		}
	}
	cfg := &config{examples: ex}
	var p problems
	checkExampleCopy(cfg, &p, last, map[string]string{"EXAMPLES_VERSION": "v0.2.1"})
	if len(p.list) > 0 {
		t.Fatalf("equal trees reported: %v", p.list)
	}
	os.WriteFile(filepath.Join(last, "raft/raft.synod"), []byte("# package raft\n# more\n"), 0o644)
	os.WriteFile(filepath.Join(last, "raft/extra.synod"), []byte("x"), 0o644)
	checkExampleCopy(cfg, &p, last, map[string]string{"EXAMPLES_VERSION": "v0.2.1"})
	wantProblem(t, strings.Join(p.list, "\n"), "raft.synod: differs", "extra.synod: is not in examples/raft")
}

// ---- Library ----

func libraryConfig(t *testing.T) *config {
	t.Helper()
	lib := filepath.Join(t.TempDir(), "examples")
	if err := copyDir("testdata/library", lib); err != nil {
		t.Fatal(err)
	}
	return &config{synod: synodBin, library: lib, packages: []string{"std/time"}, update: true}
}

func (c *config) checkLibrary() string {
	c.docs = &stdDocs{synod: c.synod}
	var p problems
	checkLibrary(c, &p)
	return strings.Join(p.list, "\n")
}

func TestLibrary(t *testing.T) {
	cfg := libraryConfig(t)
	if got := cfg.checkLibrary(); got != "" {
		t.Fatalf("the mini library must pass:\n%s", got)
	}
	b, err := os.ReadFile(filepath.Join(cfg.library, "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"| `time.Every [d: time] [b: block]` | `time/ticker` |", "| `time.Now` | `time/ticker` |"} {
		if !strings.Contains(string(b), want) {
			t.Errorf("index lacks %q:\n%s", want, b)
		}
	}
	cfg.update = false
	if got := cfg.checkLibrary(); got != "" {
		t.Errorf("index must be current:\n%s", got)
	}
	os.WriteFile(filepath.Join(cfg.library, "README.md"), []byte("old\n"), 0o644)
	wantProblem(t, cfg.checkLibrary(), "README.md: stale")
}

func TestLibraryUnusedPhrase(t *testing.T) {
	cfg := libraryConfig(t)
	path := filepath.Join(cfg.library, "time/ticker/ticker.synod")
	b, _ := os.ReadFile(path)
	os.WriteFile(path, []byte(strings.Replace(string(b), "time.Now", "1s", 1)), 0o644)
	wantProblem(t, cfg.checkLibrary(), `no example uses std/time (phrase "time.Now")`)
}

func TestLibraryHelper(t *testing.T) {
	cfg := libraryConfig(t)
	path := filepath.Join(cfg.library, "time/ticker/ticker.synod")
	b, _ := os.ReadFile(path)
	text := strings.Replace(string(b), "use example.com/library/time/shared\n", "", 1)
	os.WriteFile(path, []byte(strings.Replace(text, "shared.Step", "1s", 1)), 0o644)
	wantProblem(t, cfg.checkLibrary(), filepath.Join("time", "shared")+": has no main file, so it is a helper package, and no example imports it")
}

func TestLibraryExpect(t *testing.T) {
	cfg := libraryConfig(t)
	path := filepath.Join(cfg.library, "time/divide/expect")
	os.WriteFile(path, []byte("nobody\n"), 0o644)
	wantProblem(t, cfg.checkLibrary(), "should fail with nobody")
	os.Remove(path)
	wantProblem(t, cfg.checkLibrary(), "an example without an expect file must run")
}

func TestLibraryMissingFolder(t *testing.T) {
	cfg := libraryConfig(t)
	cfg.packages = []string{"std/time", "std/maps"}
	wantProblem(t, cfg.checkLibrary(), "no folder maps/ for std/maps")
}
