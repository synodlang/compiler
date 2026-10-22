package stdlib

import (
	"bytes"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
	"unicode"

	"github.com/synodlang/compiler/internal/format"
	"github.com/synodlang/compiler/internal/parse"
	"github.com/synodlang/compiler/internal/types"
)

func TestStdSources(t *testing.T) {
	mod, err := fs.ReadFile(Root(), "synod.mod")
	if err != nil {
		t.Fatal(err)
	}
	if want := "module github.com/synodlang/std\nsynod 0.2\n"; string(mod) != want {
		t.Errorf("synod.mod = %q, want %q", mod, want)
	}
	files := loadStd(t)
	if len(files) == 0 {
		t.Fatal("no std files")
	}
	for _, f := range files {
		if d := parse.HeaderDiagnostic(f.Tree, f.Folder); d != nil {
			t.Errorf("%s: %s", f.Path, d.String())
		}
		if f.Tree.Package != f.Folder {
			t.Errorf("%s: package %q is not the folder %q", f.Path, f.Tree.Package, f.Folder)
		}
		if d := format.Check(f.Source); d != nil {
			t.Errorf("%s: not canonical: %s", f.Path, d.String())
		}
		out, d := format.Format(f.Source)
		if d != nil || !bytes.Equal(out, f.Data) {
			t.Errorf("%s: formatting changes the file", f.Path)
		}
	}
}

// ignoredInRepo reports whether a path of the std repository is not part of
// the vendored copy.
func ignoredInRepo(rel string) bool {
	parts := strings.Split(rel, "/")
	for _, p := range parts {
		if strings.HasPrefix(p, ".") {
			return true
		}
	}
	return len(parts) == 1 && (rel == "LICENSE" || rel == "README.md" || rel == "CHANGELOG.md")
}

func TestStdDrift(t *testing.T) {
	dir := siblingDir(t, "SYNOD_STD", "SYNOD_REQUIRE_STD", "std")
	want := map[string][]byte{}
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(dir, p)
		rel = filepath.ToSlash(rel)
		if rel == "." {
			return nil
		}
		if ignoredInRepo(rel) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			return nil
		}
		data, err := os.ReadFile(p)
		want[rel] = data
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	got := map[string][]byte{}
	err = fs.WalkDir(Root(), ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := fs.ReadFile(Root(), p)
		got[p] = data
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	for p, w := range want {
		g, ok := got[p]
		switch {
		case !ok:
			t.Errorf("%s is in the std repository but not in internal/stdlib/std", p)
		case !bytes.Equal(g, w):
			t.Errorf("%s differs between the std repository and internal/stdlib/std", p)
		}
	}
	for p := range got {
		if _, ok := want[p]; !ok {
			t.Errorf("%s is in internal/stdlib/std but not in the std repository", p)
		}
	}
}

func TestNatives(t *testing.T) {
	inStd := map[string]*def{}
	for _, d := range allDefs(t) {
		if d.Native == "" {
			continue
		}
		if prev, dup := inStd[d.Native]; dup {
			t.Errorf("native id %q is declared twice: %s and %s", d.Native, prev.Pattern, d.Pattern)
		}
		inStd[d.Native] = d
	}
	for id := range inStd {
		if Natives[id] == nil {
			t.Errorf("std declares native %q, which has no entry in stdlib.Natives", id)
		}
	}
	for id := range Natives {
		if inStd[id] == nil {
			t.Errorf("stdlib.Natives has %q, which no std file declares", id)
		}
	}
	ids := make([]string, 0, len(inStd))
	for id := range inStd {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		d, s := inStd[id], Natives[id]
		if s == nil {
			continue
		}
		if s.ID != id {
			t.Errorf("%s: Sig.ID is %q", id, s.ID)
		}
		if !strings.HasPrefix(id, d.Pkg[strings.LastIndexByte(d.Pkg, '/')+1:]+".") {
			t.Errorf("%s: id is not namespaced by package %s", id, d.Pkg)
		}
		wantID := d.Pkg + "." + strings.ToLower(d.Head[:1]) + d.Head[1:]
		if d.Head == "Place" {
			wantID = "look.place" + strings.ToUpper(d.Skeleton[len("Place "):len("Place ")+1]) + d.Skeleton[len("Place ")+1:]
		}
		if id != wantID {
			t.Errorf("%s: the id rule gives %q", id, wantID)
		}
		checkSig(t, d, s)
	}
}

func checkSig(t *testing.T, d *def, s *Sig) {
	t.Helper()
	n := len(d.Holes)
	hole := func(i int, what string) *parse.Hole {
		if i < 0 || i >= n {
			t.Errorf("%s: %s index %d is out of range for %q (%d holes)", s.ID, what, i, d.Pattern, n)
			return nil
		}
		return d.Holes[i]
	}
	for _, h := range d.Holes {
		if tn, ok := h.Type.(*parse.NamedType); ok && tn.Name == "block" {
			t.Errorf("%s: natives have no block holes", s.ID)
		}
	}
	typ := func(h *parse.Hole) string {
		if h == nil {
			return ""
		}
		return typeText(h.Type)
	}
	switch s.Result.Kind {
	case RuleNone:
	case RuleFixed:
		if s.Result.T == nil {
			t.Errorf("%s: fixed result without a type", s.ID)
		}
	case RuleSameAs:
		hole(s.Result.Arg, "SameAs")
	case RuleElemOf:
		if h := hole(s.Result.Arg, "ElemOf"); h != nil && !strings.HasPrefix(typ(h), "list of ") {
			t.Errorf("%s: ElemOf hole %s is not a list", s.ID, itemText(h))
		}
	case RuleKeysOf:
		if h := hole(s.Result.Arg, "KeysOf"); h != nil && !strings.HasPrefix(typ(h), "map of ") {
			t.Errorf("%s: KeysOf hole %s is not a map", s.ID, itemText(h))
		}
	case RuleNodeOfPkg, RuleListOfNodeOfPkg:
		if h := hole(s.Result.Arg, "NodeOfPkg"); h != nil && typ(h) != "package" {
			t.Errorf("%s: NodeOfPkg hole %s is not a package", s.ID, itemText(h))
		}
	default:
		t.Errorf("%s: unknown result rule %d", s.ID, s.Result.Kind)
	}
	for _, r := range s.Rels {
		a := hole(r.A, "Rel.A")
		switch r.Kind {
		case RelElemAccepts:
			if b := hole(r.B, "Rel.B"); b != nil && !strings.HasPrefix(typ(b), "list of ") {
				t.Errorf("%s: ElemAccepts hole %s is not a list", s.ID, itemText(b))
			}
		case RelKeyAccepts:
			if b := hole(r.B, "Rel.B"); b != nil && !strings.HasPrefix(typ(b), "map of ") {
				t.Errorf("%s: KeyAccepts hole %s is not a map", s.ID, itemText(b))
			}
		case RelSameElem:
			for _, h := range []*parse.Hole{a, hole(r.B, "Rel.B")} {
				if h != nil && !strings.HasPrefix(typ(h), "list of ") {
					t.Errorf("%s: SameElem hole %s is not a list", s.ID, itemText(h))
				}
			}
		case RelSortable:
			if a != nil && !strings.HasPrefix(typ(a), "list of ") {
				t.Errorf("%s: Sortable hole %s is not a list", s.ID, itemText(a))
			}
		case RelExpectResultOn:
		default:
			t.Errorf("%s: unknown relation %d", s.ID, r.Kind)
		}
	}
	if s.NamesNode < -1 || s.NamesNode >= n {
		t.Errorf("%s: NamesNode %d is out of range", s.ID, s.NamesNode)
	} else if s.NamesNode >= 0 && typ(d.Holes[s.NamesNode]) != "node" {
		t.Errorf("%s: NamesNode hole %s is not a node", s.ID, itemText(d.Holes[s.NamesNode]))
	}
	if s.Contexts == 0 || s.Contexts&^Any != 0 {
		t.Errorf("%s: bad contexts %b", s.ID, s.Contexts)
	}
}

func TestContextTable(t *testing.T) {
	// Every key of Contexts names a phrase of std that has a body, and every
	// std phrase with a body (except those of std/look/basic, which has none)
	// has a key.
	have := map[string]bool{}
	for _, d := range allDefs(t) {
		if d.Native != "" {
			continue
		}
		key := d.Pkg + "." + d.Skeleton
		have[key] = true
		if _, ok := Contexts[key]; !ok {
			t.Errorf("no context entry for %q", key)
		}
	}
	for k := range Contexts {
		if !have[k] {
			t.Errorf("Contexts has %q, which std does not define", k)
		}
	}
}

func TestCtxString(t *testing.T) {
	for c, want := range map[Ctx]string{Any: "any", Node | Main: "node, main", Main | Check | Look: "main, check, look", Look: "look"} {
		if c.String() != want {
			t.Errorf("Ctx(%b) = %q, want %q", c, c.String(), want)
		}
	}
	if !Any.Has(Look) || Main.Has(Node) {
		t.Error("Ctx.Has")
	}
}

// ---- the catalog ----

var (
	sectionRe = regexp.MustCompile(`^## std/(\w+)`)
	eventRe   = regexp.MustCompile("(?s)\\*\\*(Node event|Trace events?):\\*\\*(.*?)(?:\\n\\n|\\z)")
	tickRe    = regexp.MustCompile("`([^`]+)`")
)

type catRow struct {
	Pkg     string
	Pattern string
	Gives   string
	Ctx     string // "" for std/look
	Kind    string // "N" or "S"
	Meaning string
}

type catEvent struct {
	Pkg     string
	Pattern string
	Node    bool
}

type catalog struct {
	Rows   []catRow
	Events []catEvent
	Theme  []catRow // Pattern = name, Gives = value
}

func cells(line string) []string {
	parts := strings.Split(strings.TrimSpace(line), "|")
	parts = parts[1 : len(parts)-1]
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	return parts
}

func parseCatalog(t *testing.T, text string) *catalog {
	t.Helper()
	c := &catalog{}
	for _, sec := range strings.Split(text, "\n## ")[1:] {
		sec = "## " + sec
		m := sectionRe.FindStringSubmatch(sec)
		if m == nil {
			continue
		}
		pkg := m[1]
		for _, line := range strings.Split(sec, "\n") {
			if !strings.HasPrefix(line, "| `") {
				continue
			}
			cs := cells(line)
			if pkg == "theme" {
				c.Theme = append(c.Theme, catRow{Pkg: pkg, Pattern: strings.Trim(cs[0], "`"), Gives: strings.Trim(cs[1], "`")})
				continue
			}
			r := catRow{Pkg: pkg, Pattern: strings.Trim(cs[0], "`"), Gives: cs[1]}
			if pkg == "look" {
				if len(cs) != 4 {
					t.Fatalf("look row %q has %d cells", line, len(cs))
				}
				r.Kind, r.Meaning = cs[2], cs[3]
			} else {
				if len(cs) != 5 {
					t.Fatalf("row %q has %d cells", line, len(cs))
				}
				r.Ctx, r.Kind, r.Meaning = cs[2], cs[3], cs[4]
			}
			c.Rows = append(c.Rows, r)
		}
		for _, e := range eventRe.FindAllStringSubmatch(sec, -1) {
			body := strings.ReplaceAll(e[2], "\n", " ")
			for _, tk := range tickRe.FindAllStringSubmatch(body, -1) {
				p := tk[1]
				if r := []rune(p)[0]; !unicode.IsUpper(r) {
					continue
				}
				c.Events = append(c.Events, catEvent{Pkg: pkg, Pattern: p, Node: e[1] == "Node event"})
			}
		}
	}
	return c
}

func parseCtx(t *testing.T, s string) Ctx {
	t.Helper()
	if s == "any" {
		return Any
	}
	var c Ctx
	for _, w := range strings.Split(s, ", ") {
		switch w {
		case "node":
			c |= Node
		case "main":
			c |= Main
		case "check":
			c |= Check
		case "look":
			c |= Look
		default:
			t.Fatalf("unknown context %q", w)
		}
	}
	return c
}

// wantRule derives the result rule that a Gives cell states, or reports that
// it cannot (second result false).
func wantRule(gives string, holes []*parse.Hole) (Rule, bool) {
	g := strings.ReplaceAll(gives, "`", "")
	idx := func(name string) int {
		for i, h := range holes {
			if h.Name.Name == name {
				return i
			}
		}
		return -1
	}
	pkgHole := func() int {
		for i, h := range holes {
			if typeText(h.Type) == "package" {
				return i
			}
		}
		return -1
	}
	switch g {
	case "—":
		return None, true
	case "number":
		return Fixed(types.TNumber), true
	case "text":
		return Fixed(types.TText), true
	case "bool":
		return Fixed(types.TBool), true
	case "time":
		return Fixed(types.TTime), true
	case "shape":
		return Fixed(types.TShape), true
	case "node":
		return Fixed(types.TNode), true
	case "list of node":
		return Fixed(types.List{Elem: types.TNode}), true
	case "list of number":
		return Fixed(types.List{Elem: types.TNumber}), true
	case "list of text":
		return Fixed(types.List{Elem: types.TText}), true
	case "p.Node":
		return NodeOfPkg(pkgHole()), true
	case "list of p.Node":
		return ListOfNodeOfPkg(pkgHole()), true
	}
	for _, pre := range []string{"same as ", "same type as "} {
		if strings.HasPrefix(g, pre) {
			return SameAs(idx(strings.TrimPrefix(g, pre))), true
		}
	}
	if strings.HasPrefix(g, "item type of ") {
		return ElemOf(idx(strings.TrimPrefix(g, "item type of "))), true
	}
	if strings.HasPrefix(g, "list of the key type of ") {
		return KeysOf(idx(strings.TrimPrefix(g, "list of the key type of "))), true
	}
	return Rule{}, false
}

func ruleEqual(a, b Rule) bool {
	return a.Kind == b.Kind && a.Arg == b.Arg && types.Equal(a.T, b.T)
}

func TestCatalog(t *testing.T) {
	dsl := siblingDir(t, "SYNOD_DSL", "SYNOD_REQUIRE_DSL", "dsl")
	data, err := os.ReadFile(filepath.Join(dsl, "std", "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	cat := parseCatalog(t, string(data))
	if len(cat.Rows) < 50 || len(cat.Events) < 20 || len(cat.Theme) != 8 {
		t.Fatalf("catalog parse looks wrong: %d rows, %d events, %d theme colors", len(cat.Rows), len(cat.Events), len(cat.Theme))
	}

	defs := allDefs(t)
	byKey := map[string][]*def{}
	for _, d := range defs {
		k := d.Pkg + "|" + d.Pattern
		byKey[k] = append(byKey[k], d)
	}
	one := func(pkg, pattern string) *def {
		ds := byKey[pkg+"|"+pattern]
		if len(ds) != 1 {
			t.Errorf("std/%s: pattern %q has %d defines, want exactly 1", pkg, pattern, len(ds))
			return nil
		}
		return ds[0]
	}
	seen := map[*def]bool{}

	for _, r := range cat.Rows {
		d := one(r.Pkg, r.Pattern)
		if d == nil {
			continue
		}
		seen[d] = true
		where := "std/" + r.Pkg + " " + r.Pattern
		switch r.Kind {
		case "N":
			if d.Native == "" || d.HasBody {
				t.Errorf("%s: a N row must be a native define", where)
				continue
			}
			s := Natives[d.Native]
			if s == nil {
				t.Errorf("%s: native %q is unknown", where, d.Native)
				continue
			}
			wantCtx := Look
			if r.Pkg != "look" {
				wantCtx = parseCtx(t, r.Ctx)
			}
			if s.Contexts != wantCtx {
				t.Errorf("%s: contexts %q, catalog says %q", where, s.Contexts, wantCtx)
			}
			if s.Kind != Phrase {
				t.Errorf("%s: kind %d, want Phrase", where, s.Kind)
			}
			want, ok := wantRule(r.Gives, d.Holes)
			if !ok {
				t.Errorf("%s: cannot read the Gives cell %q", where, r.Gives)
			} else if !ruleEqual(want, s.Result) {
				t.Errorf("%s: result rule %+v, Gives %q means %+v", where, s.Result, r.Gives, want)
			}
		case "S":
			if d.Native != "" || !d.HasBody {
				t.Errorf("%s: a S row must have a body", where)
				continue
			}
			wantCtx := Look
			if r.Pkg != "look" {
				wantCtx = parseCtx(t, r.Ctx)
			}
			got, ok := Contexts[d.Pkg+"."+d.Skeleton]
			if !ok || got != wantCtx {
				t.Errorf("%s: contexts %q, catalog says %q", where, got, wantCtx)
			}
		default:
			t.Errorf("%s: unknown kind %q", where, r.Kind)
		}
		if r.Meaning != "" && !readmeBlockFile(d) && d.Comment != r.Meaning {
			t.Errorf("%s: comment %q, catalog meaning %q", where, d.Comment, r.Meaning)
		}
	}

	for _, e := range cat.Events {
		d := one(e.Pkg, e.Pattern)
		if d == nil {
			continue
		}
		seen[d] = true
		s := Natives[d.Native]
		switch {
		case d.Native == "" || s == nil:
			t.Errorf("std/%s: event %q must be a native", e.Pkg, e.Pattern)
		case e.Node && s.Kind != NodeEvent:
			t.Errorf("std/%s: event %q is a node event, kind is %d", e.Pkg, e.Pattern, s.Kind)
		case !e.Node && s.Kind != TraceEvent:
			t.Errorf("std/%s: event %q is a trace event, kind is %d", e.Pkg, e.Pattern, s.Kind)
		case s.Result.Kind != RuleNone:
			t.Errorf("std/%s: event %q has a result", e.Pkg, e.Pattern)
		}
	}

	for _, r := range cat.Theme {
		d := one("theme", r.Pattern)
		if d == nil {
			continue
		}
		seen[d] = true
		if d.Native != "" || !d.HasBody || len(d.Line.Block.Lines) != 1 {
			t.Errorf("theme %s: want one result line", r.Pattern)
			continue
		}
		items := d.Line.Block.Lines[0].Sentence.Groups[0]
		c, ok := items[len(items)-1].(*parse.Color)
		if len(items) != 2 || !parse.IsWord(items[0], "result") || !ok || c.Text != r.Gives {
			t.Errorf("theme %s: body is not \"result %s\"", r.Pattern, r.Gives)
		}
		if got := Contexts["theme."+r.Pattern]; got != Look {
			t.Errorf("theme %s: context %q, want look", r.Pattern, got)
		}
	}

	// No extra phrases: everything std defines is in the catalog.
	for _, d := range defs {
		if !seen[d] {
			t.Errorf("%s defines %q, which the catalog does not list", d.File.Path, d.Pattern)
		}
	}
	// Records and observers are the only other top-level declarations, and
	// only std/lesson and std/look/basic have them.
	for _, f := range loadStd(t) {
		for _, n := range f.Tree.Body {
			l, ok := n.(*parse.Line)
			if !ok {
				continue
			}
			first := l.Sentence.Groups[0][0]
			switch {
			case parse.IsWord(first, "define"), parse.IsWord(first, "use"):
			case parse.IsWord(first, "when"):
				if f.pkgPath() != "look/basic" {
					t.Errorf("%s: only std/look/basic has observers", f.Path)
				}
			default:
				if f.pkgPath() != "lesson" {
					t.Errorf("%s:%d: unexpected declaration", f.Path, l.Pos.Line)
				}
			}
		}
	}
	// Every native of the table is in the catalog.
	known := map[string]bool{}
	for _, d := range defs {
		if seen[d] && d.Native != "" {
			known[d.Native] = true
		}
	}
	for id := range Natives {
		if !known[id] {
			t.Errorf("native %q is not in the catalog", id)
		}
	}
}

// readmeBlockFile reports whether the define stands in a file that is
// byte-identical to a catalog block (and so carries no comments).
func readmeBlockFile(d *def) bool {
	switch d.File.Path {
	case "time/every.synod", "lesson/lesson.synod", "look/motion.synod":
		return true
	}
	return false
}

func TestReadmeSnippets(t *testing.T) {
	dsl := siblingDir(t, "SYNOD_DSL", "SYNOD_REQUIRE_DSL", "dsl")
	data, err := os.ReadFile(filepath.Join(dsl, "std", "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	byContent := map[string][]string{}
	for _, f := range loadStd(t) {
		byContent[string(f.Data)] = append(byContent[string(f.Data)], f.Path)
	}
	blocks := readmeBlocks(string(data))
	complete := 0
	for _, b := range blocks {
		first, _, _ := strings.Cut(b, "\n")
		if !strings.HasPrefix(first, "# package ") {
			continue
		}
		complete++
		files := byContent[b]
		if len(files) != 1 {
			t.Errorf("block %q equals %d std files %v, want exactly 1", first, len(files), files)
			continue
		}
		if want := path.Base(path.Dir(files[0])); first != "# package "+want {
			t.Errorf("block %q is in folder %s", first, want)
		}
		t.Logf("%s == %s", first, files[0])
	}
	if complete != 3 {
		t.Errorf("README has %d package blocks, want 3 (time, lesson, look)", complete)
	}
}
