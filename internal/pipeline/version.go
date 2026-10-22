package pipeline

import (
	"fmt"
	"regexp"
	"runtime/debug"
	"strings"
)

// Language is the language version the compiler implements.
const Language = "0.2"

// Formats are the document formats the compiler writes.
var Formats = [...]string{"synod-trace/1", "synod-scene/1", "synod-check/1", "synod-doc/1", "synod-explain/1"}

// pseudoVersion matches Go pseudo-version suffixes: vX.Y.Z-yyyymmddhhmmss-abcdefabcdef,
// vX.Y.Z-pre.0.yyyymmddhhmmss-abcdefabcdef and vX.Y.(Z+1)-0.yyyymmddhhmmss-abcdefabcdef.
var pseudoVersion = regexp.MustCompile(`^v\d+\.\d+\.\d+-(?:[0-9A-Za-z.-]*\.)?\d{14}-[0-9a-f]{12}$`)

// realVersion reports whether v is a tagged module version worth showing.
func realVersion(v string) bool {
	return v != "" && v != "(devel)" && !strings.Contains(v, "+") && !pseudoVersion.MatchString(v)
}

// CompilerVersion renders the compiler description for `synod version`.
// injected is the -ldflags version; when it is still "dev" the module version
// from info is used if it is a real one. VCS settings add "(rev[-dirty], date)".
// info may be nil.
func CompilerVersion(injected string, info *debug.BuildInfo) string {
	v := injected
	if info == nil {
		return v
	}
	if v == "dev" && realVersion(info.Main.Version) {
		v = info.Main.Version
	}
	var rev, date string
	modified := false
	for _, s := range info.Settings {
		switch s.Key {
		case "vcs.revision":
			rev = s.Value
		case "vcs.time":
			date = s.Value
		case "vcs.modified":
			modified = s.Value == "true"
		}
	}
	if len(rev) > 7 {
		rev = rev[:7]
	}
	if len(date) > 10 {
		date = date[:10]
	}
	if rev == "" {
		return v
	}
	if modified {
		rev += "-dirty"
	}
	if date != "" {
		return fmt.Sprintf("%s (%s, %s)", v, rev, date)
	}
	return fmt.Sprintf("%s (%s)", v, rev)
}

// VersionText is the output of `synod version` for the compiler description
// compiler (see CompilerVersion).
func VersionText(compiler string) string {
	return fmt.Sprintf("language %s\ncompiler %s\nformats %s\n", Language, compiler, strings.Join(Formats[:], " "))
}
