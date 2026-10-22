// Package explain holds the texts of `synod explain` (decision 93): one
// embedded Markdown entry per diagnostic code of spec/11, an example module
// and its fix. The diagnostic of an example is never stored; Render runs the
// example through the compiler and returns what it says right now.
package explain

import (
	"embed"
	"fmt"
	"regexp"
	"strings"
)

//go:embed codes/*.md
var codeFS embed.FS

// Kind of a code: reported while compiling or while running.
const (
	KindCompile = "compile"
	KindRun     = "run"
)

// compileCodes and runCodes are the codes of spec/11 sections 11.2 and 11.3,
// in the order of the tables. A test checks them against the spec.
var compileCodes = []string{
	"syntax", "indent", "non-canonical", "bad-header", "unknown-phrase",
	"ambiguous-phrase", "duplicate-name", "shadowing", "reserved-word",
	"bad-pattern", "bad-receiver", "bad-place", "type-mismatch",
	"ambiguous-empty", "missing-result", "value-as-statement",
	"action-as-value", "unexported", "unused-name", "unused-import",
	"unneeded-alias", "unknown-import", "import-cycle", "missing-main",
	"extra-main", "context", "foreign-state", "color-literal",
	"native-outside-std",
}

var runCodes = []string{
	"missing-key", "index-range", "disk-type", "division-by-zero", "nobody",
	"time-limit", "bad-argument", "number-range", "run-limit",
}

// Codes returns every code of spec/11 in table order: compile-time codes,
// then run-time codes.
func Codes() []string {
	return append(append([]string(nil), compileCodes...), runCodes...)
}

// KindOf returns the kind of a known code.
func KindOf(code string) string {
	for _, c := range runCodes {
		if c == code {
			return KindRun
		}
	}
	return KindCompile
}

// File is one source file of an example or of a fix.
type File struct {
	Path    string
	Content string
}

// Entry is one parsed entry file.
type Entry struct {
	Code    string
	Summary string
	// Text is the prose, paragraphs separated by one empty line, lines as
	// written in the entry.
	Text string
	// Program is the folder `synod run` runs, "" for an example that is
	// checked with `synod check .`.
	Program string
	// Example holds the files of the example, in entry order. synod.mod is
	// implied (see DefaultMod) unless listed.
	Example []File
	// FixNote is the one sentence of the fix.
	FixNote string
	// FixFiles replace example files of the same path (or add files).
	FixFiles []File
	// FixDeleted lists example files the fix removes.
	FixDeleted []string
}

// Command is "run" for an example with a program and "check" otherwise.
func (e *Entry) Command() string {
	if e.Program != "" {
		return "run"
	}
	return "check"
}

// Kind is KindRun or KindCompile.
func (e *Entry) Kind() string { return KindOf(e.Code) }

// DefaultMod is the synod.mod of an example that lists none.
const DefaultMod = "module example.com/explain\nsynod 0.2\n"

// ExampleFiles returns the example files with the implied synod.mod added.
func (e *Entry) ExampleFiles() []File { return withMod(e.Example) }

// FixedFiles returns the files after the fix: fix files replace or add files
// of the same path, deleted files are removed, the rest stay. The implied
// synod.mod is included.
func (e *Entry) FixedFiles() []File {
	files := withMod(e.Example)
	for _, f := range e.FixFiles {
		replaced := false
		for i := range files {
			if files[i].Path == f.Path {
				files[i] = f
				replaced = true
			}
		}
		if !replaced {
			files = append(files, f)
		}
	}
	var out []File
	for _, f := range files {
		deleted := false
		for _, d := range e.FixDeleted {
			if d == f.Path {
				deleted = true
			}
		}
		if !deleted {
			out = append(out, f)
		}
	}
	return out
}

func withMod(files []File) []File {
	out := append([]File(nil), files...)
	for _, f := range out {
		if f.Path == "synod.mod" {
			return out
		}
	}
	return append([]File{{"synod.mod", DefaultMod}}, out...)
}

var (
	titleRe = regexp.MustCompile(`^# ([a-z]+(-[a-z]+)*)$`)
	runRe   = regexp.MustCompile("^Run with `synod run ([^ `]+)`\\.$")
	pathRe  = regexp.MustCompile(`^[A-Za-z0-9_.-]+(/[A-Za-z0-9_.-]+)*$`)
)

// parser reads an entry line by line.
type parser struct {
	lines []string
	i     int
}

func (p *parser) done() bool { return p.i >= len(p.lines) }

func (p *parser) peek() string {
	if p.done() {
		return ""
	}
	return p.lines[p.i]
}

func (p *parser) next() string {
	l := p.peek()
	p.i++
	return l
}

func (p *parser) errf(format string, args ...any) error {
	return fmt.Errorf("line %d: %s", p.i+1, fmt.Sprintf(format, args...))
}

// blank consumes exactly one empty line.
func (p *parser) blank() error {
	if p.done() || p.peek() != "" {
		return p.errf("expected an empty line")
	}
	p.i++
	return nil
}

// Parse reads an entry file strictly. name is the file name without .md and
// must equal the title. Format(Parse(b)) equals b for every accepted entry.
func Parse(name string, data []byte) (*Entry, error) {
	text := string(data)
	if !strings.HasSuffix(text, "\n") || strings.HasSuffix(text, "\n\n") {
		return nil, fmt.Errorf("must end with exactly one newline")
	}
	if strings.ContainsAny(text, "\r\t") {
		return nil, fmt.Errorf("must not contain CR or tab characters")
	}
	p := &parser{lines: strings.Split(strings.TrimSuffix(text, "\n"), "\n")}
	for i, l := range p.lines {
		if strings.HasSuffix(l, " ") {
			return nil, fmt.Errorf("line %d: trailing space", i+1)
		}
	}
	e := &Entry{}
	m := titleRe.FindStringSubmatch(p.peek())
	if m == nil {
		return nil, p.errf("the first line must be the title, # <code>")
	}
	e.Code = m[1]
	if e.Code != name {
		return nil, p.errf("title %q does not match the file name %q", e.Code, name)
	}
	p.next()
	if err := p.blank(); err != nil {
		return nil, err
	}
	e.Summary = p.next()
	if e.Summary == "" || strings.HasPrefix(e.Summary, "#") || strings.HasPrefix(e.Summary, "```") {
		return nil, p.errf("expected the one-line summary after the title")
	}
	if !strings.HasSuffix(e.Summary, ".") {
		return nil, p.errf("the summary must end with a period")
	}
	if err := p.blank(); err != nil {
		return nil, err
	}
	var prose []string
	for !p.done() && p.peek() != "## Example" {
		prose = append(prose, p.next())
	}
	if p.done() {
		return nil, fmt.Errorf("missing the ## Example section")
	}
	// The line before "## Example" is the empty separator.
	if len(prose) == 0 || prose[len(prose)-1] != "" {
		return nil, p.errf("expected an empty line before ## Example")
	}
	prose = prose[:len(prose)-1]
	e.Text = strings.Join(prose, "\n")
	if e.Text == "" || strings.HasPrefix(e.Text, "\n") || strings.Contains(e.Text, "\n\n\n") {
		return nil, fmt.Errorf("the prose must be 1 to 3 paragraphs separated by one empty line")
	}
	if n := strings.Count(e.Text, "\n\n") + 1; n > 3 {
		return nil, fmt.Errorf("the prose has %d paragraphs; at most 3", n)
	}
	for _, l := range prose {
		if strings.HasPrefix(l, "#") {
			return nil, fmt.Errorf("the prose must not contain headings")
		}
	}
	p.next() // ## Example
	if err := p.blank(); err != nil {
		return nil, err
	}
	if m := runRe.FindStringSubmatch(p.peek()); m != nil {
		e.Program = m[1]
		p.next()
		if err := p.blank(); err != nil {
			return nil, err
		}
	}
	files, _, err := p.files("## Fix", false)
	if err != nil {
		return nil, err
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("the example has no files")
	}
	e.Example = files
	p.next() // ## Fix
	if err := p.blank(); err != nil {
		return nil, err
	}
	e.FixNote = p.next()
	if e.FixNote == "" || strings.HasPrefix(e.FixNote, "#") || strings.HasPrefix(e.FixNote, "```") {
		return nil, p.errf("expected the one-sentence fix note after ## Fix")
	}
	if !strings.HasSuffix(e.FixNote, ".") {
		return nil, p.errf("the fix note must end with a period")
	}
	if !p.done() {
		if err := p.blank(); err != nil {
			return nil, err
		}
	}
	e.FixFiles, e.FixDeleted, err = p.files("", true)
	if err != nil {
		return nil, err
	}
	if len(e.FixFiles) == 0 && len(e.FixDeleted) == 0 {
		return nil, fmt.Errorf("the fix lists no file")
	}
	if err := e.validate(); err != nil {
		return nil, err
	}
	return e, nil
}

// files reads "### path" blocks until the line stop (or the end when stop is
// ""). Blocks are separated by one empty line; the empty line before stop is
// consumed.
func (p *parser) files(stop string, allowDelete bool) (files []File, deleted []string, err error) {
	for n := 0; ; n++ {
		if n > 0 {
			// The previous block ended; one empty line separates it from
			// what follows.
			if p.done() {
				if stop != "" {
					return nil, nil, fmt.Errorf("missing %s", stop)
				}
				return files, deleted, nil
			}
			if err := p.blank(); err != nil {
				return nil, nil, err
			}
			if stop != "" && p.peek() == stop {
				return files, deleted, nil
			}
			if p.done() {
				return nil, nil, p.errf("unexpected empty line at the end")
			}
		}
		head := p.next()
		if !strings.HasPrefix(head, "### ") {
			return nil, nil, p.errf("expected ### <path>, found %q", head)
		}
		path := strings.TrimPrefix(head, "### ")
		isDel := false
		if strings.HasSuffix(path, " (deleted)") {
			isDel = true
			path = strings.TrimSuffix(path, " (deleted)")
		}
		if !pathRe.MatchString(path) || strings.Contains(path, "..") {
			return nil, nil, p.errf("bad path %q", path)
		}
		for _, f := range files {
			if f.Path == path {
				return nil, nil, p.errf("file %s listed twice", path)
			}
		}
		for _, d := range deleted {
			if d == path {
				return nil, nil, p.errf("file %s listed twice", path)
			}
		}
		if isDel {
			if !allowDelete {
				return nil, nil, p.errf("only the fix can delete files")
			}
			deleted = append(deleted, path)
			continue
		}
		fence := "```"
		if strings.HasSuffix(path, ".synod") {
			fence = "```synod"
		}
		if p.next() != fence {
			return nil, nil, p.errf("expected the fence %s after the file heading", fence)
		}
		var body []string
		for {
			if p.done() {
				return nil, nil, fmt.Errorf("unterminated code block for %s", path)
			}
			l := p.next()
			if l == "```" {
				break
			}
			body = append(body, l)
		}
		if len(body) == 0 {
			return nil, nil, fmt.Errorf("empty code block for %s", path)
		}
		files = append(files, File{path, strings.Join(body, "\n") + "\n"})
	}
}

func (e *Entry) validate() error {
	if e.Program != "" && e.Kind() != KindRun {
		return fmt.Errorf("only a run-time code has a Run line")
	}
	if e.Program == "" && e.Kind() == KindRun {
		return fmt.Errorf("a run-time code needs a Run line")
	}
	return nil
}

// Format writes the entry in canonical form.
func Format(e *Entry) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n%s\n\n%s\n\n## Example\n\n", e.Code, e.Summary, e.Text)
	if e.Program != "" {
		fmt.Fprintf(&b, "Run with `synod run %s`.\n\n", e.Program)
	}
	writeFiles(&b, e.Example, nil)
	fmt.Fprintf(&b, "\n## Fix\n\n%s\n", e.FixNote)
	if len(e.FixFiles)+len(e.FixDeleted) > 0 {
		b.WriteString("\n")
		writeFiles(&b, e.FixFiles, e.FixDeleted)
	}
	return b.String()
}

func writeFiles(b *strings.Builder, files []File, deleted []string) {
	n := 0
	sep := func() {
		if n > 0 {
			b.WriteString("\n")
		}
		n++
	}
	for _, f := range files {
		sep()
		fence := "```"
		if strings.HasSuffix(f.Path, ".synod") {
			fence = "```synod"
		}
		fmt.Fprintf(b, "### %s\n%s\n%s```\n", f.Path, fence, f.Content)
	}
	for _, d := range deleted {
		sep()
		fmt.Fprintf(b, "### %s (deleted)\n", d)
	}
}

// loadEntries parses every embedded entry.
func loadEntries() (map[string]*Entry, map[string]error) {
	entries := map[string]*Entry{}
	errs := map[string]error{}
	list, err := codeFS.ReadDir("codes")
	if err != nil {
		return entries, errs
	}
	for _, de := range list {
		name := strings.TrimSuffix(de.Name(), ".md")
		data, err := codeFS.ReadFile("codes/" + de.Name())
		if err != nil {
			errs[name] = err
			continue
		}
		e, err := Parse(name, data)
		if err != nil {
			errs[name] = err
			continue
		}
		entries[name] = e
	}
	return entries, errs
}

// Entries returns the entries that exist and parse, in the order of spec/11.
func Entries() []*Entry {
	m, _ := loadEntries()
	var out []*Entry
	for _, c := range Codes() {
		if e := m[c]; e != nil {
			out = append(out, e)
		}
	}
	return out
}

// Lookup returns the entry of a code.
func Lookup(code string) (*Entry, bool) {
	m, _ := loadEntries()
	e, ok := m[code]
	return e, ok
}

// Missing returns the codes of spec/11 that have no entry file.
func Missing() []string {
	m, errs := loadEntries()
	var out []string
	for _, c := range Codes() {
		if m[c] == nil && errs[c] == nil {
			out = append(out, c)
		}
	}
	return out
}

// Problems returns the embedded files that do not parse or that name no code
// of spec/11, keyed by code (file name).
func Problems() map[string]error {
	m, errs := loadEntries()
	known := map[string]bool{}
	for _, c := range Codes() {
		known[c] = true
	}
	for name := range m {
		if !known[name] {
			errs[name] = fmt.Errorf("not a code of spec/11")
		}
	}
	return errs
}

// Suggest returns the known code closest to s when it is within edit
// distance 2 (the first in spec order on a tie), or "".
func Suggest(s string) string {
	best, bestD := "", 3
	for _, c := range Codes() {
		if d := editDistance(s, c); d < bestD {
			best, bestD = c, d
		}
	}
	return best
}

func editDistance(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	prev := make([]int, len(rb)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(ra); i++ {
		cur := make([]int, len(rb)+1)
		cur[0] = i
		for j := 1; j <= len(rb); j++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}
		prev = cur
	}
	return prev[len(rb)]
}
