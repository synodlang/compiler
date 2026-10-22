package load

import (
	"strings"

	"github.com/synodlang/compiler/internal/source"
)

// ParseMod parses the text of a synod.mod. It returns whatever could be read
// and the syntax diagnostics, including the non-canonical one for require
// lines that are not sorted. display is the path shown in diagnostics.
func ParseMod(display string, src []byte) (*Module, []source.Diagnostic) {
	return parseMod(display, src)
}

// ValidModulePath reports whether p is a module path without a version.
func ValidModulePath(p string) bool { return validModulePath(p) }

// ValidVersion reports whether v looks like "v1.2.0".
func ValidVersion(v string) bool {
	_, ok := parseSemver(v)
	return ok
}

// VersionLess reports whether version a is lower than version b. Both must
// be valid versions; an invalid one counts as v0.0.0.
func VersionLess(a, b string) bool {
	sa, _ := parseSemver(a)
	sb, _ := parseSemver(b)
	return sa.less(sb)
}

// PackageNameProblem returns why s cannot name a package, or "" when it can:
// a package name is a lower-case word (letters and digits, starting with a
// letter) that is not a reserved word.
func PackageNameProblem(s string) string {
	switch {
	case !isWord(s) || s != strings.ToLower(s):
		return "not a lower-case word"
	case reserved[s]:
		return "a reserved word"
	}
	return ""
}
