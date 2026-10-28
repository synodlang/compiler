package forms

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/synodlang/compiler/internal/doc"
)

// dslDir returns the dsl checkout: $SYNOD_DSL, else ../dsl next to the
// module. A missing one skips the test, or fails it with SYNOD_REQUIRE_DSL=1.
func dslDir(t *testing.T) string {
	t.Helper()
	dir := os.Getenv("SYNOD_DSL")
	if dir == "" {
		wd, err := os.Getwd()
		if err != nil {
			t.Fatal(err)
		}
		for {
			if _, err := os.Stat(filepath.Join(wd, "go.mod")); err == nil {
				break
			}
			if parent := filepath.Dir(wd); parent != wd {
				wd = parent
				continue
			}
			t.Fatal("go.mod not found")
		}
		dir = filepath.Join(wd, "..", "dsl")
	}
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		if os.Getenv("SYNOD_REQUIRE_DSL") == "1" {
			t.Fatalf("SYNOD_REQUIRE_DSL is 1 but %s does not exist", dir)
		}
		t.Skipf("%s not found; set SYNOD_DSL to run", dir)
	}
	return dir
}

func readSpec(t *testing.T, dsl, glob string) string {
	t.Helper()
	m, _ := filepath.Glob(filepath.Join(dsl, glob))
	if len(m) != 1 {
		t.Fatalf("%s: want one file, got %v", glob, m)
	}
	b, err := os.ReadFile(m[0])
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// section returns the text from the heading line starting with from up to
// the next line starting with to ("" for the end).
func section(t *testing.T, text, from, to string) string {
	t.Helper()
	i := strings.Index(text, "\n"+from)
	if i < 0 {
		t.Fatalf("heading %q not found", from)
	}
	text = text[i+1:]
	if to != "" {
		if j := strings.Index(text, "\n"+to); j >= 0 {
			text = text[:j]
		}
	}
	return text
}

var (
	ebnfFence = regexp.MustCompile("(?s)```ebnf\n(.*?)```")
	ebnfCmt   = regexp.MustCompile(`(?s)\(\*.*?\*\)`)
	ebnfProd  = regexp.MustCompile(`(?m)^(\w+)\s*=`)
	ebnfTerm  = regexp.MustCompile(`"([^"]+)"`)
)

// grammar returns the productions and the quoted terminals of the ebnf
// blocks of text, comments removed.
func grammar(text string) (prods, terms []string) {
	for _, m := range ebnfFence.FindAllStringSubmatch(text, -1) {
		body := ebnfCmt.ReplaceAllString(m[1], "")
		for _, p := range ebnfProd.FindAllStringSubmatch(body, -1) {
			prods = append(prods, p[1])
		}
		for _, q := range ebnfTerm.FindAllStringSubmatch(body, -1) {
			terms = append(terms, q[1])
		}
	}
	return
}

func TestCatalogShape(t *testing.T) {
	seen := map[string]bool{}
	for _, it := range Catalog() {
		if seen[it.Key()] {
			t.Errorf("duplicate catalog item %s", it.Key())
		}
		seen[it.Key()] = true
		if it.Spec == "" {
			t.Errorf("%s has no spec reference", it.Key())
		}
	}
	if n := len(formItems); n < 60 || n > 70 {
		t.Errorf("%d forms, want about 64", n)
	}
}

func TestGrammarCoverage(t *testing.T) {
	spec := readSpec(t, dslDir(t), "spec/02-*.md")
	s21 := section(t, spec, "## 2.1", "## 2.2")
	s22 := section(t, spec, "## 2.2", "## 2.3")
	prods22, terms22 := grammar(s22)
	_, terms21 := grammar(s21)
	if len(prods22) < 20 {
		t.Fatalf("found only %d productions in 2.2: %v", len(prods22), prods22)
	}
	haveProd := map[string]bool{}
	haveTerm := map[string]bool{}
	for _, it := range formItems {
		for _, p := range it.Productions {
			haveProd[p] = true
		}
		for _, q := range it.Terminals {
			haveTerm[q] = true
		}
	}
	for _, p := range prods22 {
		if !haveProd[p] {
			t.Errorf("spec/02 §2.2 production %q has no form", p)
		}
	}
	for _, q := range append(terms21, terms22...) {
		if !haveTerm[q] {
			t.Errorf("spec/02 terminal %q is in no form's terminals", q)
		}
	}
	// A form must not claim a production or terminal the spec lacks.
	specProd := map[string]bool{}
	specTerm := map[string]bool{}
	for _, p := range prods22 {
		specProd[p] = true
	}
	p21, _ := grammar(s21)
	for _, p := range p21 {
		specProd[p] = true
	}
	for _, q := range append(terms21, terms22...) {
		specTerm[q] = true
	}
	for _, it := range formItems {
		for _, p := range it.Productions {
			if !specProd[p] {
				t.Errorf("form %s names production %q, which spec/02 does not have", it.ID, p)
			}
		}
		for _, q := range it.Terminals {
			if !specTerm[q] {
				t.Errorf("form %s names terminal %q, which spec/02 does not have", it.ID, q)
			}
		}
	}
}

// firstColumn returns the first cell of each table row of text, backticks and
// bold marks removed, without the header and separator rows.
func firstColumn(text string) []string {
	var out []string
	for _, l := range strings.Split(text, "\n") {
		if !strings.HasPrefix(l, "|") {
			continue
		}
		cell := strings.TrimSpace(strings.Split(l, "|")[1])
		if cell == "" || strings.HasPrefix(cell, "---") {
			continue
		}
		out = append(out, strings.Trim(cell, "`*"))
	}
	return out[1:] // the header
}

func sameSet(t *testing.T, what string, got, want []string) {
	t.Helper()
	g, w := append([]string(nil), got...), append([]string(nil), want...)
	sort.Strings(g)
	sort.Strings(w)
	if strings.Join(g, "\n") != strings.Join(w, "\n") {
		t.Errorf("%s differ:\ncatalog: %v\nspec:    %v", what, g, w)
	}
}

func ids(k Kind) []string {
	var out []string
	for _, it := range Catalog() {
		if it.Kind == k {
			out = append(out, it.ID)
		}
	}
	return out
}

func TestTypesMatchSpec(t *testing.T) {
	spec := readSpec(t, dslDir(t), "spec/04-*.md")
	var want []string
	for _, sec := range []string{section(t, spec, "## 4.1", "## 4.2"), section(t, spec, "## 4.2", "## 4.3")} {
		for _, cell := range firstColumn(sec) {
			switch {
			case strings.HasPrefix(cell, "list of"):
				cell = "list"
			case strings.HasPrefix(cell, "map of"):
				cell = "map"
			case strings.Contains(cell, "Node"):
				cell = "Node"
			case strings.HasPrefix(cell, "<"):
				cell = "record"
			}
			want = append(want, cell)
		}
	}
	sameSet(t, "types", ids(KindType), want)
}

func TestContextsMatchSpec(t *testing.T) {
	spec := readSpec(t, dslDir(t), "spec/05-*.md")
	sameSet(t, "contexts", ids(KindContext), firstColumn(section(t, spec, "## 5.4", "## 5.5")))
}

func TestPackagesMatchSpec(t *testing.T) {
	readme := readSpec(t, dslDir(t), "std/README.md")
	var want []string
	for _, l := range strings.Split(readme, "\n") {
		if strings.HasPrefix(l, "## std/") {
			want = append(want, strings.Fields(l)[1])
		}
	}
	want = append(want, "std/look/basic")
	sameSet(t, "std packages", ids(KindPackage), want)
	sameSet(t, "doc.StdPackages", doc.StdPackages(), want)
}

var specRef = regexp.MustCompile(`^spec/(\d\d) §(\d+\.\d+)$`)

func TestSpecRefsExist(t *testing.T) {
	dsl := dslDir(t)
	for _, it := range Catalog() {
		if it.Spec == "std/README.md" {
			continue
		}
		m := specRef.FindStringSubmatch(it.Spec)
		if m == nil {
			t.Errorf("%s: bad spec reference %q", it.Key(), it.Spec)
			continue
		}
		text := readSpec(t, dsl, "spec/"+m[1]+"-*.md")
		if !regexp.MustCompile(`(?m)^#{2,3} ` + regexp.QuoteMeta(m[2]) + `\b`).MatchString(text) {
			t.Errorf("%s: spec/%s has no heading %s", it.Key(), m[1], m[2])
		}
	}
}
