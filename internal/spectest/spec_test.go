package spectest

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/synodlang/compiler/internal/format"
	"github.com/synodlang/compiler/internal/parse"
	"github.com/synodlang/compiler/internal/source"
)

// siblingDir returns the directory named by env, or def relative to the
// module root. If it does not exist the test is skipped, or fails when the
// require variable is "1".
func siblingDir(t *testing.T, env, require, def string) string {
	t.Helper()
	dir := os.Getenv(env)
	if dir == "" {
		dir = filepath.Join("..", def)
	}
	if !filepath.IsAbs(dir) {
		dir = filepath.Join(moduleRoot(t), dir) // relative paths are relative to the module root
	}
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		if os.Getenv(require) == "1" {
			t.Fatalf("%s is required but %s does not exist", require, dir)
		}
		t.Skipf("%s not found; set %s to run", dir, env)
	}
	return dir
}

// moduleRoot returns the directory holding go.mod, found from the test's
// working directory.
func moduleRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found")
		}
		dir = parent
	}
}

var (
	fenceOpen   = regexp.MustCompile("^( *)```synod\\s*$")
	headerBlock = regexp.MustCompile(`^# package [a-z][A-Za-z0-9]*$`)
)

type block struct {
	file string // slash path relative to the spec root
	line int    // line of the first line of code
	src  string
}

// extract returns the synod code blocks of one Markdown file.
func extract(name, text string) []block {
	var out []block
	lines := strings.Split(text, "\n")
	for i := 0; i < len(lines); i++ {
		m := fenceOpen.FindStringSubmatch(lines[i])
		if m == nil {
			continue
		}
		indent := m[1]
		b := block{file: name, line: i + 2}
		var body []string
		for i++; i < len(lines); i++ {
			l := strings.TrimRight(lines[i], "\r")
			if strings.TrimPrefix(l, indent) == "```" || strings.TrimSpace(l) == "```" {
				break
			}
			body = append(body, strings.TrimPrefix(l, indent))
		}
		b.src = strings.Join(body, "\n") + "\n"
		out = append(out, b)
	}
	return out
}

func TestSpecSnippets(t *testing.T) {
	root := siblingDir(t, "SYNOD_DSL", "SYNOD_REQUIRE_DSL", "dsl")
	var mds []string
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && p != root && strings.HasPrefix(d.Name(), ".") {
			return filepath.SkipDir
		}
		if !d.IsDir() && strings.HasSuffix(p, ".md") {
			mds = append(mds, p)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(mds)

	var complete, fragments, noncanon int
	for _, p := range mds {
		data, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		rel, _ := filepath.Rel(root, p)
		rel = filepath.ToSlash(rel)
		for _, b := range extract(rel, string(data)) {
			where := b.file + ":" + strconv.Itoa(b.line)
			isComplete := headerBlock.MatchString(firstLine(b.src))
			f := source.NewFile(where, []byte(b.src))
			if _, d := parse.Parse(f); d != nil {
				t.Errorf("%s: does not parse: %s", where, d.String())
				continue
			}
			d := format.Check(f)
			if isComplete {
				complete++
				t.Logf("complete file %s", where)
				if d != nil {
					t.Errorf("%s: not canonical: %s", where, d.String())
				}
				continue
			}
			fragments++
			if d != nil {
				noncanon++
				t.Logf("fragment %s: parses; not canonical: %s", where, d.String())
			} else {
				t.Logf("fragment %s: parses; canonical", where)
			}
		}
	}
	t.Logf("%d complete files checked, %d fragments (%d not canonical, logged only)", complete, fragments, noncanon)
	if complete+fragments == 0 {
		t.Error("no synod code blocks found")
	}
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

func TestExtract(t *testing.T) {
	md := "text\n```synod\n# package a\n\nfoo\n```\n- item\n\n  ```synod\n  x\n    y\n  ```\n```go\nnot\n```\n"
	bs := extract("m.md", md)
	if len(bs) != 2 {
		t.Fatalf("got %d blocks", len(bs))
	}
	if bs[0].src != "# package a\n\nfoo\n" || bs[0].line != 3 {
		t.Errorf("block 0: %+v", bs[0])
	}
	if bs[1].src != "x\n  y\n" || bs[1].line != 10 {
		t.Errorf("block 1: %+v", bs[1])
	}
}

func TestExamplesDrift(t *testing.T) {
	root := siblingDir(t, "SYNOD_EXAMPLES", "SYNOD_REQUIRE_EXAMPLES", "examples")
	vendored := filepath.Join(moduleRoot(t), "testdata", "examples")
	a, b := collectExamples(t, root), collectExamples(t, vendored)
	for name, data := range a {
		got, ok := b[name]
		switch {
		case !ok:
			t.Errorf("%s: in examples but not vendored", name)
		case !bytes.Equal(got, data):
			t.Errorf("%s: vendored copy differs from examples", name)
		}
	}
	for name := range b {
		if _, ok := a[name]; !ok {
			t.Errorf("%s: vendored but not in examples", name)
		}
	}
	if len(a) == 0 {
		t.Error("no example files found")
	}
}

func collectExamples(t *testing.T, root string) map[string][]byte {
	t.Helper()
	out := map[string][]byte{}
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if p != root && strings.HasPrefix(d.Name(), ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(p, ".synod") && d.Name() != "synod.mod" {
			return nil
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, p)
		out[filepath.ToSlash(rel)] = data
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}
