package trace

import (
	"path"
	"strconv"
	"strings"

	"github.com/synodlang/compiler/internal/ir"
	"github.com/synodlang/compiler/internal/load"
)

// FilePaths maps the display path of every source file of g (ir.Pos.File,
// which depends on the working directory) to its trace path, which does not:
//
//   - a file of the main module (or of a module given by an override):
//     its slash path relative to the module root, "paxos/paxos.synod";
//   - a std file: "std/<package>/<file>";
//   - a file of a vendored module: "<module path>@<version>/<path in the
//     module>";
//   - a file of a folder without synod.mod: "<folder base>/<file>".
func FilePaths(g *load.Graph) map[string]string {
	out := map[string]string{}
	for _, p := range g.Packages {
		for _, f := range p.Files {
			if f.Src == nil {
				continue
			}
			out[f.Src.Path] = tracePath(p, f)
		}
	}
	return out
}

func tracePath(p *load.Package, f *load.File) string {
	m := p.Module
	switch {
	case m == nil || m.Std || p.IsStd:
		return f.Path
	case m.Path == "":
		return path.Base(path.Clean(p.Dir)) + "/" + path.Base(f.Path)
	}
	rel := relTo(f.Path, m.Dir)
	if m.Version != "" {
		return m.Path + "@" + m.Version + "/" + rel
	}
	return rel
}

// relTo returns file relative to dir (both slash paths).
func relTo(file, dir string) string {
	file, dir = path.Clean(file), path.Clean(dir)
	if dir == "." {
		return file
	}
	if dir == "/" {
		return strings.TrimPrefix(file, "/")
	}
	return strings.TrimPrefix(file, dir+"/")
}

// file returns the trace path of a display path.
func (o Options) file(display string) string {
	if t, ok := o.Paths[display]; ok {
		return t
	}
	return display
}

// pos is "file:line:col" with the trace path.
func (o Options) pos(p ir.Pos) string {
	return o.file(p.File) + ":" + strconv.Itoa(p.Line) + ":" + strconv.Itoa(p.Col)
}

// rule is a rule text with its leading display path replaced by the trace
// path of pos.File.
func (o Options) rule(rule string, pos ir.Pos) string {
	if pos.File != "" && strings.HasPrefix(rule, pos.File+":") {
		return o.file(pos.File) + rule[len(pos.File):]
	}
	return rule
}
