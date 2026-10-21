package load

import (
	"strconv"
	"strings"

	"github.com/synodlang/compiler/internal/source"
)

// LanguageVersion is the Synod version this compiler implements.
const LanguageVersion = "0.2"

// StdModule is the module path of the standard library.
const StdModule = "github.com/synodlang/std"

// Module is a parsed synod.mod.
type Module struct {
	Path     string            // module path; "" for a folder without synod.mod
	Dir      string            // slash path of the module root; "$std" for std
	Version  string            // "v1.2.0" for a vendored module, else ""
	Synod    string            // language version, "0.2"
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

// parseMod parses the synod.mod in src (implementation/04). The returned
// module holds whatever could be read. Errors are syntax diagnostics, and a
// non-canonical diagnostic for require lines that are not sorted.
func parseMod(display string, src []byte) (*Module, []source.Diagnostic) {
	f := source.NewFile(display, src)
	m := &Module{Requires: map[string]string{}, File: display, Src: f}
	var ds []source.Diagnostic
	add := func(line int, code source.Code, msg, hint string) {
		ds = append(ds, source.New(f, source.Pos{Line: line, Col: 1}, code, msg, hint))
	}
	text := strings.TrimPrefix(string(src), "\xef\xbb\xbf")
	var haveModule, haveSynod, unsorted bool
	lastReq := ""
	for i, line := range strings.Split(text, "\n") {
		n := i + 1
		fields := strings.Fields(line)
		if len(fields) == 0 || strings.HasPrefix(fields[0], "#") {
			continue
		}
		switch fields[0] {
		case "module":
			switch {
			case haveModule:
				add(n, source.Syntax, `a second "module" directive`, "")
			case len(fields) != 2:
				add(n, source.Syntax, `"module" takes one module path`, `write "module <path>"`)
			case !validModulePath(fields[1]):
				add(n, source.Syntax, `bad module path "`+fields[1]+`"`, "")
			default:
				m.Path = fields[1]
			}
			haveModule = true
		case "synod":
			switch {
			case haveSynod:
				add(n, source.Syntax, `a second "synod" directive`, "")
			case len(fields) != 2:
				add(n, source.Syntax, `"synod" takes one version`, `write "synod `+LanguageVersion+`"`)
			default:
				maj, min, ok := splitVersion(fields[1])
				if !ok {
					add(n, source.Syntax, `bad Synod version "`+fields[1]+`"`, `write "synod `+LanguageVersion+`"`)
					break
				}
				m.Synod = fields[1]
				wmaj, wmin, _ := splitVersion(LanguageVersion)
				if maj > wmaj || maj == wmaj && min > wmin {
					add(n, source.Syntax, "module needs Synod "+fields[1]+"; this compiler implements "+LanguageVersion, "")
				}
			}
			haveSynod = true
		case "require":
			switch {
			case len(fields) != 3:
				add(n, source.Syntax, `"require" takes a module path and a version`, `write "require <module> v1.2.0"`)
			case !validModulePath(fields[1]):
				add(n, source.Syntax, `bad module path "`+fields[1]+`"`, "")
			default:
				if _, ok := parseSemver(fields[2]); !ok {
					add(n, source.Syntax, `bad version "`+fields[2]+`"`, `a version looks like "v1.2.0"`)
					break
				}
				if _, dup := m.Requires[fields[1]]; dup {
					add(n, source.Syntax, `module "`+fields[1]+`" is required twice`, "")
					break
				}
				m.Requires[fields[1]] = fields[2]
				if fields[1] < lastReq && !unsorted {
					unsorted = true
					add(n, source.NonCanonical, "require lines are not sorted", "sort them by module path")
				}
				lastReq = fields[1]
			}
		default:
			add(n, source.Syntax, `unknown directive "`+fields[0]+`"`, `directives are "module", "synod" and "require"`)
		}
	}
	if !haveModule {
		add(1, source.Syntax, `missing "module" directive`, `write "module <path>"`)
	}
	if !haveSynod {
		add(1, source.Syntax, `missing "synod" directive`, `write "synod `+LanguageVersion+`"`)
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
