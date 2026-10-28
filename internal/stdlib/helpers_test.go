package stdlib

import (
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/synodlang/compiler/internal/parse"
	"github.com/synodlang/compiler/internal/source"
)

// siblingDir returns the directory named by env, or ../def relative to the
// module root. When it does not exist the test is skipped, or it fails when
// the variable require is "1".
func siblingDir(t *testing.T, env, require, def string) string {
	t.Helper()
	dir := os.Getenv(env)
	if dir == "" {
		dir = filepath.Join("..", def)
	}
	if !filepath.IsAbs(dir) {
		dir = filepath.Join(moduleRoot(t), dir)
	}
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		if os.Getenv(require) == "1" {
			t.Fatalf("%s is required but %s does not exist", require, dir)
		}
		t.Skipf("%s not found; set %s to run", dir, env)
	}
	return dir
}

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

// stdFile is one parsed std source file.
type stdFile struct {
	Path   string // slash path inside the std repository, e.g. "look/basic/basic.synod"
	Folder string // base name of the folder
	Data   []byte
	Tree   *parse.File
	Source *source.File
}

// loadStd parses every .synod file of the embedded std.
func loadStd(t *testing.T) []*stdFile {
	t.Helper()
	root := Root()
	var out []*stdFile
	err := fs.WalkDir(root, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(p, ".synod") {
			return err
		}
		data, err := fs.ReadFile(root, p)
		if err != nil {
			return err
		}
		f := source.NewFile("std/"+p, data)
		tree, diag := parse.Parse(f)
		if diag != nil {
			t.Errorf("%s does not parse: %s", p, diag.String())
			return nil
		}
		out = append(out, &stdFile{Path: p, Folder: path.Base(path.Dir(p)), Data: data, Tree: tree, Source: f})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}

// pkgPath is the std package path of a file, such as "look/basic".
func (f *stdFile) pkgPath() string { return path.Dir(f.Path) }

// def is one "define" line of a std file.
type def struct {
	File        *stdFile
	Pkg         string // package path, e.g. "look"
	Line        *parse.Line
	Pattern     string // the pattern as written: "Seed [n: number]"
	Skeleton    string // head and literal words: "Cluster of running"
	Head        string
	Holes       []*parse.Hole
	Native      string // native id, or ""
	HasBody     bool
	Comment     string // text of the last leading comment, or ""
	AllComments []string
}

func typeText(t parse.Type) string {
	switch t := t.(type) {
	case *parse.NamedType:
		return t.Name
	case *parse.ListType:
		return "list of " + typeText(t.Elem)
	case *parse.MapType:
		return "map of " + typeText(t.Key) + " to " + typeText(t.Value)
	}
	return "?"
}

func itemText(it parse.Item) string {
	switch it := it.(type) {
	case *parse.Word:
		return it.Name
	case *parse.Hole:
		return "[" + it.Name.Name + ": " + typeText(it.Type) + "]"
	}
	return "?"
}

// defsOf returns the define lines of a file in order.
func defsOf(f *stdFile) []*def {
	var out []*def
	if f.Tree.Theme {
		return nil // the definitions of a theme are values, see themeDefaults
	}
	for _, n := range f.Tree.Body {
		l, ok := n.(*parse.Line)
		if !ok || len(l.Sentence.Groups) != 1 {
			continue
		}
		items := l.Sentence.Groups[0]
		if len(items) < 2 || !parse.IsWord(items[0], "define") {
			continue
		}
		d := &def{File: f, Pkg: f.pkgPath(), Line: l, HasBody: l.Block != nil}
		pat := items[1:]
		if n := len(pat); n >= 2 && parse.IsWord(pat[n-2], "native") {
			if tx, ok := pat[n-1].(*parse.Text); ok {
				d.Native = strings.Trim(tx.Raw, `"`)
				pat = pat[:n-2]
			}
		}
		var words, texts []string
		for _, it := range pat {
			texts = append(texts, itemText(it))
			switch it := it.(type) {
			case *parse.Word:
				words = append(words, it.Name)
			case *parse.Hole:
				d.Holes = append(d.Holes, it)
			}
		}
		if len(words) > 0 {
			d.Head = words[0]
		}
		d.Pattern = strings.Join(texts, " ")
		d.Skeleton = strings.Join(words, " ")
		for _, c := range l.Leading {
			d.AllComments = append(d.AllComments, c.Text)
		}
		if len(l.Leading) > 0 {
			d.Comment = l.Leading[len(l.Leading)-1].Text
		}
		out = append(out, d)
	}
	return out
}

// allDefs returns the defines of the whole std.
func allDefs(t *testing.T) []*def {
	t.Helper()
	var out []*def
	for _, f := range loadStd(t) {
		out = append(out, defsOf(f)...)
	}
	return out
}

var fenceOpen = regexp.MustCompile("^```synod\\s*$")

// readmeBlocks returns the synod code blocks of a Markdown text.
func readmeBlocks(text string) []string {
	var out []string
	lines := strings.Split(text, "\n")
	for i := 0; i < len(lines); i++ {
		if !fenceOpen.MatchString(lines[i]) {
			continue
		}
		var body []string
		for i++; i < len(lines) && strings.TrimSpace(lines[i]) != "```"; i++ {
			body = append(body, lines[i])
		}
		out = append(out, strings.Join(body, "\n")+"\n")
	}
	return out
}

// themeValue is one definition of a theme file: its name and its literal.
type themeValue struct {
	Name    string
	Literal parse.Item
	Comment string
}

// themeDefaults returns the definitions of std/theme/default in file order.
func themeDefaults(t *testing.T) []themeValue {
	t.Helper()
	var out []themeValue
	for _, f := range loadStd(t) {
		if f.pkgPath() != "theme/default" {
			continue
		}
		for _, n := range f.Tree.Body {
			l, ok := n.(*parse.Line)
			if !ok {
				continue
			}
			g := l.Sentence.Groups[0]
			if len(g) != 2 || !parse.IsWord(g[0], "define") || l.Block == nil || len(l.Block.Lines) != 1 {
				t.Errorf("%s:%d: not a theme definition", f.Path, l.Pos.Line)
				continue
			}
			r := l.Block.Lines[0].Sentence.Groups[0]
			if len(r) != 2 || !parse.IsWord(r[0], "result") {
				t.Errorf("%s:%d: not one result line", f.Path, l.Pos.Line)
				continue
			}
			v := themeValue{Name: g[1].(*parse.Word).Name, Literal: r[1]}
			if len(l.Leading) > 0 {
				v.Comment = l.Leading[len(l.Leading)-1].Text
			}
			out = append(out, v)
		}
	}
	return out
}
