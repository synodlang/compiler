package doc

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/synodlang/compiler/internal/driver"
	"github.com/synodlang/compiler/internal/load"
	"github.com/synodlang/compiler/internal/stdlib"
	"github.com/synodlang/compiler/internal/types"
)

func TestGivesText(t *testing.T) {
	holes := []string{"l", "p"}
	for _, c := range []struct {
		r    stdlib.Rule
		want string
	}{
		{stdlib.None, "—"},
		{stdlib.Fixed(types.TNumber), "number"},
		{stdlib.Fixed(types.List{Elem: types.TNode}), "list of node"},
		{stdlib.SameAs(0), "same as `l`"},
		{stdlib.ElemOf(0), "item type of `l`"},
		{stdlib.KeysOf(0), "list of the key type of `l`"},
		{stdlib.NodeOfPkg(1), "p.Node"},
		{stdlib.ListOfNodeOfPkg(1), "list of p.Node"},
	} {
		if got := GivesText(c.r, holes); got != c.want {
			t.Errorf("GivesText(%+v) = %q, want %q", c.r, got, c.want)
		}
	}
}

func TestMarkdownGives(t *testing.T) {
	for in, want := range map[string]string{
		"—": "—", "number": "number", "list of node": "`list of node`", "p.Node": "`p.Node`",
		"same as `l`": "same as `l`",
	} {
		if got := markdownGives(in); got != want {
			t.Errorf("markdownGives(%q) = %q, want %q", in, got, want)
		}
	}
}

func readme(t *testing.T) string {
	t.Helper()
	dir := os.Getenv("SYNOD_DSL")
	if dir == "" {
		dir = filepath.Join("..", "..", "..", "dsl")
	}
	data, err := os.ReadFile(filepath.Join(dir, "std", "README.md"))
	if err != nil {
		if os.Getenv("SYNOD_REQUIRE_DSL") == "1" {
			t.Fatal(err)
		}
		t.Skipf("dsl not found (%v); set SYNOD_DSL to run", err)
	}
	return string(data)
}

func stdBuilt(t *testing.T) []*Package {
	t.Helper()
	all := StdPackages()
	res := driver.Check(driver.Options{Load: load.Config{ExtraStd: all}}, nil)
	if len(res.Diags) > 0 {
		t.Fatalf("std has diagnostics: %v", res.Diags[0])
	}
	var out []*Package
	for _, p := range all {
		out = append(out, Build(res, res.ByPath[p], ViewOf(res.ByPath[p])))
	}
	return out
}

var headingRe = regexp.MustCompile(`^## (std/[\w/]+)(?: — (.*))?$`)

func TestTitles(t *testing.T) {
	text := readme(t)
	got := map[string]string{}
	for _, l := range strings.Split(text, "\n") {
		if m := headingRe.FindStringSubmatch(l); m != nil {
			got[m[1]] = m[2]
		}
	}
	if len(got) != 13 {
		t.Fatalf("README has %d std headings, want 13", len(got))
	}
	for p, title := range got {
		if Title(p) != title {
			t.Errorf("Title(%s) = %q, README heading says %q", p, Title(p), title)
		}
	}
	for _, p := range TitledPaths() {
		if _, ok := got[p]; !ok {
			t.Errorf("titles has %s, which README has no heading for", p)
		}
	}
	if len(StdPackages()) != 14 {
		t.Errorf("StdPackages = %v, want 14", StdPackages())
	}
}

func cells(line string) []string {
	parts := strings.Split(strings.TrimSpace(line), "|")
	parts = parts[1 : len(parts)-1]
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	return parts
}

// TestCatalogRows: every row of the README table that is not "Unexported"
// appears in the generated catalog with the same qualified pattern, Gives
// text, context and Meaning (decision 91).
func TestCatalogRows(t *testing.T) {
	text := readme(t)
	cat := Catalog(stdBuilt(t))
	type row struct{ gives, ctx, meaning string }
	have := map[string]row{}
	haveTheme := map[string]string{} // name -> "value|use" of std/theme/default
	section := ""
	for _, l := range strings.Split(cat, "\n") {
		if m := headingRe.FindStringSubmatch(l); m != nil {
			section = m[1]
		}
		if strings.HasPrefix(l, "| `") {
			cs := cells(l)
			if section == "std/theme/default" {
				haveTheme[strings.Trim(cs[0], "`")] = cs[1] + "|" + cs[2]
			} else if len(cs) == 4 {
				have[section+"|"+strings.Trim(cs[0], "`")] = row{strings.ReplaceAll(cs[1], "`", ""), cs[2], cs[3]}
			}
		}
	}
	section = ""
	checked := 0
	for _, l := range strings.Split(text, "\n") {
		if m := headingRe.FindStringSubmatch(l); m != nil {
			section = m[1]
		}
		if !strings.HasPrefix(l, "| `") || section == "" {
			continue
		}
		if section == "std/theme/default" {
			cs := cells(l)
			name := strings.Trim(cs[0], "`")
			if got, want := haveTheme[name], cs[1]+"|"+cs[2]; got != want {
				t.Errorf("std/theme/default %s: catalog has %q, README says %q", name, got, want)
			}
			delete(haveTheme, name)
			continue
		}
		cs := cells(l)
		pattern, gives := strings.Trim(cs[0], "`"), strings.ReplaceAll(cs[1], "`", "")
		var ctx, meaning string
		if section == "std/look" {
			ctx, meaning = "look", cs[3]
		} else {
			ctx, meaning = cs[2], cs[4]
		}
		if strings.HasPrefix(meaning, "Unexported") {
			continue
		}
		q := section[strings.LastIndexByte(section, '/')+1:]
		got, ok := have[section+"|"+q+"."+pattern]
		if !ok {
			t.Errorf("%s: the catalog has no row %q", section, q+"."+pattern)
			continue
		}
		checked++
		if got.gives != gives || got.ctx != ctx || got.meaning != meaning {
			t.Errorf("%s %s: catalog row is %+v, README says gives %q, ctx %q, meaning %q", section, pattern, got, gives, ctx, meaning)
		}
	}
	for name := range haveTheme {
		t.Errorf("std/theme/default: the catalog has %s, README does not", name)
	}
	if checked < 60 {
		t.Errorf("only %d rows compared", checked)
	}
	// std/theme/default is listed as Name | Value | Use, with the literals of
	// default.synod.
	for _, row := range []string{"| `Paper` | `#151A2E` | scene ground |", "| `Glow` | `24` | glow blur |", "| `NodeShape` | `\"card\"` | node cards are rects |"} {
		if !strings.Contains(cat, row) {
			t.Errorf("the catalog has no row %q", row)
		}
	}
}

// TestCatalogHeader pins the first lines and the order of the sections.
func TestCatalogHeader(t *testing.T) {
	cat := Catalog(stdBuilt(t))
	if !strings.HasPrefix(cat, "# Synod standard library\n\nGenerated by `synod doc --catalog` for Synod 0.2. Do not edit.\n\n## std/disk") {
		t.Errorf("header: %.200q", cat)
	}
	if !strings.HasSuffix(cat, "\n") || strings.HasSuffix(cat, "\n\n") || strings.Contains(cat, "\n\n\n") {
		t.Error("blank lines")
	}
	last := ""
	for _, l := range strings.Split(cat, "\n") {
		if strings.HasPrefix(l, "## ") {
			p := strings.Fields(l)[1]
			if p <= last {
				t.Errorf("section %s after %s", p, last)
			}
			last = p
		}
		if strings.HasSuffix(l, " ") {
			t.Errorf("trailing space in %q", l)
		}
	}
}

// TestDescribeAs: with the phrase's own package as the reader, the pattern
// is the one Describe prints, for every phrase of std and the examples.
func TestDescribeAs(t *testing.T) {
	root := filepath.Join("..", "..", "testdata", "examples")
	abs, err := filepath.Abs(root)
	if err != nil {
		t.Fatal(err)
	}
	fsys := load.OSFS{}
	targets, err := load.Discover(fsys, filepath.ToSlash(abs))
	if err != nil {
		t.Fatal(err)
	}
	res := driver.Check(driver.Options{Load: load.Config{FS: fsys}, DefaultLook: true}, targets)
	n := 0
	for _, rp := range res.Packages {
		for _, ph := range rp.Phrases {
			if ph.Kind != 0 {
				continue
			}
			n++
			if got, want := ph.DescribeAs("", rp.Path, nil), ph.Describe(""); got != want {
				t.Errorf("%s: DescribeAs = %q, Describe = %q", rp.Path, got, want)
			}
			if got, want := ph.DescribeAs("q", rp.Path, nil), ph.Describe("q"); got != want {
				t.Errorf("%s: DescribeAs(q) = %q, Describe(q) = %q", rp.Path, got, want)
			}
		}
	}
	if n < 100 {
		t.Errorf("only %d phrases compared", n)
	}
}

// TestPatternsMatchItems: the items of a phrase, joined, are its pattern
// without the receiver and the blocks.
func TestPatternsMatchItems(t *testing.T) {
	for _, p := range stdBuilt(t) {
		for _, ph := range p.Phrases {
			var parts []string
			for _, it := range ph.Items {
				if it.Word != "" {
					parts = append(parts, it.Word)
				} else {
					parts = append(parts, "["+it.Hole+": "+it.Type+"]")
				}
			}
			if !strings.Contains(ph.Pattern, strings.Join(parts, " ")) {
				t.Errorf("%s: items %v do not make up the pattern", ph.Pattern, parts)
			}
			if ph.Doc == "" {
				t.Errorf("%s has no doc comment", ph.Pattern)
			}
		}
	}
}

// TestThemePackageDoc: a theme package is documented by its values
// (implementation/04-formats.md, synod-doc/1): kind "theme", the 34 values in
// file order with their types and doc comments, and no phrase.
func TestThemePackageDoc(t *testing.T) {
	res := driver.Check(driver.Options{Load: load.Config{ExtraStd: []string{"std/theme", "std/theme/default"}}}, nil)
	if len(res.Diags) > 0 {
		t.Fatalf("%v", res.Diags[0])
	}
	rp := res.ByPath["std/theme/default"]
	p := Build(res, rp, ViewOf(rp))
	if p.Kind != "theme" || len(p.Values) != len(stdlib.ThemeNames) || len(p.Phrases) != 0 || p.Title != "the default theme values" {
		t.Fatalf("package = kind %q, %d values, %d phrases, title %q", p.Kind, len(p.Values), len(p.Phrases), p.Title)
	}
	for i, n := range stdlib.ThemeNames {
		v := p.Values[i]
		if v.Name != n.Name || v.Doc == "" || v.Def.Line == 0 {
			t.Errorf("value %d = %+v, want %s with a doc and a position", i, v, n.Name)
		}
		want := "number"
		switch n.Type {
		case types.TColor:
			want = "color"
		case types.TText:
			want = "text"
		}
		if v.Type != want {
			t.Errorf("%s has type %s, want %s", n.Name, v.Type, want)
		}
	}
	var buf strings.Builder
	if err := WriteJSON(&buf, nil, nil, []*Package{p}); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"title":"the default theme values","kind":"theme","values":[{"name":"Paper","type":"color","value":"#151A2E","doc":"scene ground"`, `"name":"Glow","type":"number","value":24,`, `"name":"NodeShape","type":"text","value":"card",`, `"std":true,"activates":false,"phrases":[]`} {
		if !strings.Contains(buf.String(), want) {
			t.Errorf("JSON lacks %s", want)
		}
	}
	other := res.ByPath["std/theme"]
	var o strings.Builder
	WriteJSON(&o, nil, nil, []*Package{Build(res, other, ViewOf(other))})
	if strings.Contains(o.String(), `"kind":"theme"`) || strings.Contains(o.String(), `"values"`) {
		t.Error("a package that is not a theme has kind or values")
	}
}
