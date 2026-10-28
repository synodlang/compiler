package load

import (
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/synodlang/compiler/internal/source"
)

// LanguageVersion is the Synod version this compiler implements.
const LanguageVersion = "0.3"

// StdModule is the module path of the standard library.
const StdModule = "github.com/synodlang/std"

// Module is a parsed synod.mod.
type Module struct {
	Path     string            // module path; "" for a folder without synod.mod
	Dir      string            // slash path of the module root; "$std" for std
	Version  string            // "v1.2.0" for a vendored module, else ""
	Synod    string            // language version, "0.3"
	Requires map[string]string // module path → version
	Std      bool
	// File is the display path of synod.mod ("" if there is none).
	File string
	// Src is the source of synod.mod, nil if there is none. Diagnostics
	// about the file are rendered from it.
	Src *source.File
}

// semver is a parsed "vX.Y.Z".
type semver [3]int

func parseSemver(s string) (semver, bool) {
	var v semver
	if !strings.HasPrefix(s, "v") {
		return v, false
	}
	parts := strings.Split(s[1:], ".")
	if len(parts) != 3 {
		return v, false
	}
	for i, p := range parts {
		n, ok := atoi(p)
		if !ok {
			return v, false
		}
		v[i] = n
	}
	return v, true
}

// atoi parses a short run of digits.
func atoi(s string) (int, bool) {
	if s == "" || len(s) > 9 {
		return 0, false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return 0, false
		}
	}
	n, err := strconv.Atoi(s)
	return n, err == nil
}

func (a semver) less(b semver) bool {
	for i := range a {
		if a[i] != b[i] {
			return a[i] < b[i]
		}
	}
	return false
}

// validModulePath checks a module path against the import path grammar of
// spec/01 section 1.9, without a version.
func validModulePath(p string) bool {
	if p == "" {
		return false
	}
	for _, seg := range strings.Split(p, "/") {
		if seg == "" {
			return false
		}
		for i := 0; i < len(seg); i++ {
			c := seg[i]
			alnum := c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9'
			if !alnum && (i == 0 || c != '.' && c != '-') {
				return false
			}
		}
	}
	return true
}

// field is one whitespace separated word of a line of synod.mod: its text and
// its 1-based code point columns, End exclusive.
type field struct {
	Text       string
	Start, End int
}

// fieldsOf splits line like strings.Fields, with the columns of the fields.
func fieldsOf(line string) []field {
	var out []field
	col, start := 1, -1
	word := strings.Builder{}
	flush := func() {
		if start >= 0 {
			out = append(out, field{word.String(), start, col})
			word.Reset()
			start = -1
		}
	}
	for _, r := range line {
		if unicode.IsSpace(r) {
			flush()
		} else {
			if start < 0 {
				start = col
			}
			word.WriteRune(r)
		}
		col++
	}
	flush()
	return out
}

// parseMod parses the synod.mod in src (implementation/04). The returned
// module holds whatever could be read. Errors are syntax diagnostics, and a
// non-canonical diagnostic for require lines that are not sorted. The
// primary span of a diagnostic starts at column 1 of its line, as the
// position always has, and runs to the end of the word it is about.
func parseMod(display string, src []byte) (*Module, []source.Diagnostic) {
	f := source.NewFile(display, src)
	m := &Module{Requires: map[string]string{}, File: display, Src: f}
	var ds []source.Diagnostic
	// upto returns the span from column 1 of line n to the end of fields[i].
	upto := func(n int, fields []field, i int) source.Span {
		return source.Span{Start: source.Pos{Line: n, Col: 1}, End: source.Pos{Line: n, Col: fields[i].End}}
	}
	add := func(sp source.Span, code source.Code, msg, hint string, more ...source.Extra) {
		ds = append(ds, source.NewSpan(f, sp, code, msg, hint, more...))
	}
	text := strings.TrimPrefix(string(src), "\xef\xbb\xbf")
	var haveModule, haveSynod, unsorted bool
	var firstModule, firstSynod source.Span
	lastReq := ""
	for i, line := range strings.Split(text, "\n") {
		n := i + 1
		fields := fieldsOf(line)
		if len(fields) == 0 || strings.HasPrefix(fields[0].Text, "#") {
			continue
		}
		last := len(fields) - 1
		word := upto(n, fields, 0)
		switch fields[0].Text {
		case "module":
			switch {
			case haveModule:
				add(word, source.Syntax, `a second "module" directive`, "",
					source.Also(display, firstModule, "first here"))
			case len(fields) != 2:
				add(upto(n, fields, last), source.Syntax, `"module" takes one module path`, `write "module <path>"`,
					source.HelpText(`write "module <path>"`))
			case !validModulePath(fields[1].Text):
				add(upto(n, fields, 1), source.Syntax, `bad module path "`+fields[1].Text+`"`, "")
			default:
				m.Path = fields[1].Text
			}
			if !haveModule {
				firstModule = word
			}
			haveModule = true
		case "synod":
			switch {
			case haveSynod:
				add(word, source.Syntax, `a second "synod" directive`, "",
					source.Also(display, firstSynod, "first here"))
			case len(fields) != 2:
				add(upto(n, fields, last), source.Syntax, `"synod" takes one version`, `write "synod `+LanguageVersion+`"`,
					source.HelpText(`write "synod `+LanguageVersion+`"`))
			default:
				maj, min, ok := splitVersion(fields[1].Text)
				if !ok {
					add(upto(n, fields, 1), source.Syntax, `bad Synod version "`+fields[1].Text+`"`, `write "synod `+LanguageVersion+`"`,
						source.HelpText(`write "synod `+LanguageVersion+`"`))
					break
				}
				m.Synod = fields[1].Text
				wmaj, wmin, _ := splitVersion(LanguageVersion)
				if maj > wmaj || maj == wmaj && min > wmin {
					add(upto(n, fields, 1), source.Syntax, "module needs Synod "+fields[1].Text+"; this compiler implements "+LanguageVersion, "",
						source.Note("this compiler reads synod "+LanguageVersion))
				}
			}
			if !haveSynod {
				firstSynod = word
			}
			haveSynod = true
		case "require":
			switch {
			case len(fields) != 3:
				add(upto(n, fields, last), source.Syntax, `"require" takes a module path and a version`, `write "require <module> v1.2.0"`,
					source.HelpText(`write "require <module> v1.2.0"`))
			case !validModulePath(fields[1].Text):
				add(upto(n, fields, 1), source.Syntax, `bad module path "`+fields[1].Text+`"`, "")
			default:
				if _, ok := parseSemver(fields[2].Text); !ok {
					add(upto(n, fields, 2), source.Syntax, `bad version "`+fields[2].Text+`"`, `a version looks like "v1.2.0"`,
						source.HelpText(`write a version like "v1.2.0"`))
					break
				}
				if _, dup := m.Requires[fields[1].Text]; dup {
					add(upto(n, fields, 1), source.Syntax, `module "`+fields[1].Text+`" is required twice`, "")
					break
				}
				m.Requires[fields[1].Text] = fields[2].Text
				if fields[1].Text < lastReq && !unsorted {
					unsorted = true
					add(upto(n, fields, 2), source.NonCanonical, "require lines are not sorted", "sort them by module path",
						source.HelpText("sort the require lines by module path"))
				}
				lastReq = fields[1].Text
			}
		default:
			add(word, source.Syntax, `unknown directive "`+fields[0].Text+`"`, `directives are "module", "synod" and "require"`,
				source.HelpText(`write "module", "synod" or "require"`))
		}
	}
	// A missing directive is reported on the first line, which is shown
	// whole.
	first := source.Span{Start: source.Pos{Line: 1, Col: 1}, End: source.Pos{Line: 1, Col: 1 + utf8.RuneCountInString(strings.TrimRight(f.Line(1), " \t\r"))}}
	if !haveModule {
		add(first, source.Syntax, `missing "module" directive`, `write "module <path>"`,
			source.HelpText(`write "module <path>"`))
	}
	if !haveSynod {
		add(first, source.Syntax, `missing "synod" directive`, `write "synod `+LanguageVersion+`"`,
			source.HelpText(`write "synod `+LanguageVersion+`"`))
	}
	return m, ds
}

// splitVersion parses "N.M".
func splitVersion(s string) (major, minor int, ok bool) {
	a, b, found := strings.Cut(s, ".")
	if !found {
		return 0, 0, false
	}
	major, ok1 := atoi(a)
	minor, ok2 := atoi(b)
	return major, minor, ok1 && ok2
}
