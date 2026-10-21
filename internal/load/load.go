// Package load finds the modules, packages and imports of a Synod program
// (spec/06). It reads synod.mod files, turns folders into packages, splits
// the main file from the library, resolves every import against the module
// itself, the embedded std, local overrides and the main module's vendor
// folder, and orders
// the packages with their dependencies first.
//
// The loader reads through the FS interface only, so it runs on the real
// disk (OSFS), in memory (MapFS), and under wasm. It lexes and parses every
// file but does not format-check them or resolve names; those are the
// resolver's and the driver's work.
//
// Load reports syntax, indent and bad-header (per file), syntax and
// non-canonical (synod.mod), extra-main, unknown-import and import-cycle.
package load

import (
	"io/fs"
	"path"
	"sort"
	"strings"

	"github.com/synodlang/compiler/internal/parse"
	"github.com/synodlang/compiler/internal/source"
)

// Config tells Load where to read from.
type Config struct {
	// FS is the file system; nil means OSFS.
	FS FS
	// Std is the std module: synod.mod at its root and one folder per
	// package ("net/net.synod", "look/basic/basic.synod"). nil means no std.
	Std fs.FS
	// Modules maps a module path to a directory that provides it, ahead of
	// the vendor folder. Tests use it for the examples and for spec snippets.
	Modules map[string]string
	// Display turns an FS path into the path shown in diagnostics. nil means
	// the FS's own Display method if it has one (OSFS shows paths relative
	// to the working directory), else the path as it is. Std files are
	// always shown as "std/<package>/<file>".
	Display func(path string) string
}

// Graph is the result of Load.
type Graph struct {
	// Packages holds every package reached from the targets, dependencies
	// first, in a deterministic order.
	Packages []*Package
	// Targets are the packages of the requested folders or files, in the
	// order requested: a folder gives its library and then its main file.
	Targets []*Package
	// Modules holds the modules that were read, sorted by path and version.
	Modules []*Module
}

// Package is one library package or one main file.
type Package struct {
	// Path is the import path ("std/net",
	// "github.com/synodlang/examples/raft"). A main package has the import
	// path of its folder plus "#main". A package of a folder without
	// synod.mod has the folder's path.
	Path string
	// Name is the qualifier: the last segment of Path, or "main".
	Name   string
	Dir    string // slash path of the folder; "$std/<package>" for std
	Module *Module
	IsMain bool
	IsStd  bool
	// Broken reports that a file failed to lex or parse; its Tree is nil.
	Broken  bool
	Files   []*File    // sorted by base name
	Imports []*Import  // in file order, then line order
	Deps    []*Package // resolved and distinct, sorted by Path
}

// File is a source file of a package.
type File struct {
	Src  *source.File
	Tree *parse.File // nil if the file failed to parse
	// Path is the slash path on the FS ("std/net/net.synod" for std).
	Path string
}

// Import is one "use" line.
type Import struct {
	File *File
	Line *parse.Line
	// Raw is the path as written, with any version.
	Raw     string
	Path    string // without the version
	Version string // "v1.2.0", or ""
	Alias   string // "" if there is no "as"
	PathPos source.Pos
	// AliasPos is the position of the alias, or the zero Pos.
	AliasPos source.Pos
	// Pkg is the imported library package, or nil if the import failed.
	Pkg *Package
}

// Load loads the packages of targets, which are slash paths of folders or
// of .synod files. A file selects the package of its folder that holds it.
// The diagnostics are sorted and free of duplicates.
func Load(cfg Config, targets []string) (*Graph, []source.Diagnostic) {
	l := newLoader(cfg)
	g := l.run(targets)
	source.Sort(l.diags)
	out := l.diags[:0]
	for i, d := range l.diags {
		if i == 0 || d.Key() != l.diags[i-1].Key() {
			out = append(out, d)
		}
	}
	return g, out
}

type folder struct {
	lib, main *Package
	libDiags  []source.Diagnostic // reported when lib joins the graph
	mainDiags []source.Diagnostic
}

type loader struct {
	fsys     FS
	std      fs.FS
	vendor   string // <main module>/vendor, or "" without a synod.mod
	over     map[string]string
	overKeys []string // longest first
	display  func(string) string

	mods     map[string]*Module // by directory
	stdMod   *Module
	selected map[string]string // build list: module → highest required version
	folders  map[string]*folder
	pkgs     map[string]*Package // by Path
	linked   map[*Package]bool
	diags    []source.Diagnostic
}

func newLoader(cfg Config) *loader {
	l := &loader{
		fsys:     cfg.FS,
		std:      cfg.Std,
		over:     cfg.Modules,
		display:  cfg.Display,
		mods:     map[string]*Module{},
		selected: map[string]string{},
		folders:  map[string]*folder{},
		pkgs:     map[string]*Package{},
		linked:   map[*Package]bool{},
	}
	if l.fsys == nil {
		l.fsys = OSFS{}
	}
	if l.display == nil {
		if d, ok := l.fsys.(interface{ Display(string) string }); ok {
			l.display = d.Display
		} else {
			l.display = func(p string) string { return p }
		}
	}
	for k := range l.over {
		l.overKeys = append(l.overKeys, k)
	}
	sort.Slice(l.overKeys, func(i, j int) bool {
		a, b := l.overKeys[i], l.overKeys[j]
		if len(a) != len(b) {
			return len(a) > len(b)
		}
		return a < b
	})
	l.stdMod = &Module{Path: StdModule, Dir: "$std", Synod: LanguageVersion, Requires: map[string]string{}, Std: true}
	return l
}

// diag records a diagnostic at pos of file f.
func (l *loader) diag(f *source.File, pos source.Pos, code source.Code, msg, hint string) {
	l.diags = append(l.diags, source.New(f, pos, code, msg, hint))
}

// plain records a diagnostic at 1:1 of a path that has no source.File.
func (l *loader) plain(display string, code source.Code, msg, hint string) {
	l.diag(source.NewFile(display, nil), source.Pos{Line: 1, Col: 1}, code, msg, hint)
}

type target struct {
	dir  string
	file string // "" for a folder
	mod  *Module
}

func (l *loader) abs(p string) string {
	if a, ok := l.fsys.(interface{ Abs(string) (string, error) }); ok {
		if r, err := a.Abs(p); err == nil {
			return r
		}
	}
	return path.Clean(p)
}

func (l *loader) run(args []string) *Graph {
	var ts []target
	for _, a := range args {
		p := l.abs(a)
		switch {
		case isDir(l.fsys, p):
			ts = append(ts, target{dir: p})
		default:
			if _, err := l.fsys.ReadFile(p); err != nil {
				l.plain(l.display(p), source.UnknownImport, `cannot read "`+l.display(p)+`"`, "")
				continue
			}
			ts = append(ts, target{dir: parentDir(p), file: p})
		}
	}
	for i := range ts {
		ts[i].mod = l.moduleFor(ts[i].dir)
	}
	// The build list comes first, so that the versions do not depend on
	// the order in which packages are loaded.
	var roots []*Module
	for _, t := range ts {
		roots = append(roots, t.mod)
		// Only the vendor folder of the main module (the first target's)
		// is read; a dependency's own vendor folder is ignored.
		if l.vendor == "" && t.mod.Path != "" {
			l.vendor = join(t.mod.Dir, "vendor")
		}
	}
	for _, k := range l.overKeys {
		if m := l.localModule(l.over[k], k); m != nil {
			roots = append(roots, m)
		}
	}
	l.buildList(roots)

	g := &Graph{}
	inTargets := map[*Package]bool{}
	for _, t := range ts {
		ipath, name := l.targetPath(t)
		fo := l.folder(t.dir, t.mod, ipath, name)
		if fo == nil {
			continue
		}
		var picked []*Package
		for _, p := range []*Package{fo.lib, fo.main} {
			if p == nil {
				continue
			}
			if t.file != "" {
				found := false
				for _, f := range p.Files {
					found = found || f.Path == t.file
				}
				if !found {
					continue
				}
			}
			picked = append(picked, p)
		}
		for _, p := range picked {
			if !inTargets[p] {
				inTargets[p] = true
				g.Targets = append(g.Targets, p)
				l.link(p)
			}
		}
	}
	l.order(g)
	return g
}

func isDir(fsys FS, p string) bool {
	_, err := fsys.ReadDir(p)
	return err == nil
}

func parentDir(p string) string {
	d := path.Dir(p)
	if d == "." && !strings.HasPrefix(p, "./") && !strings.Contains(p, "/") {
		return "."
	}
	return d
}

// targetPath returns the import path and the package name of a target folder.
func (l *loader) targetPath(t target) (ipath, name string) {
	if t.mod.Path == "" {
		return l.display(t.dir), path.Base(t.dir)
	}
	rel := strings.TrimPrefix(strings.TrimPrefix(t.dir, t.mod.Dir), "/")
	if rel == "" || rel == "." {
		ipath = t.mod.Path
	} else {
		ipath = t.mod.Path + "/" + rel
	}
	return ipath, lastSegment(ipath)
}

func lastSegment(p string) string {
	if i := strings.LastIndexByte(p, '/'); i >= 0 {
		return p[i+1:]
	}
	return p
}

// moduleFor returns the module that holds dir: the one with the nearest
// synod.mod above it, or else a module without path rooted at dir.
func (l *loader) moduleFor(dir string) *Module {
	for d := dir; d != ""; d = parent(d) {
		if m := l.mods[d]; m != nil {
			return m
		}
		if _, err := l.fsys.ReadFile(join(d, "synod.mod")); err == nil {
			return l.moduleAt(d, "", "")
		}
	}
	m := &Module{Dir: dir, Requires: map[string]string{}}
	l.mods[dir] = m
	return m
}

// moduleAt reads the synod.mod of dir (once). When the file is missing the
// module has defPath, which a vendored module takes from its directory name.
func (l *loader) moduleAt(dir, defPath, version string) *Module {
	if m := l.mods[dir]; m != nil {
		return m
	}
	m := &Module{Path: defPath, Dir: dir, Version: version, Requires: map[string]string{}}
	if src, err := l.fsys.ReadFile(join(dir, "synod.mod")); err == nil {
		pm, ds := parseMod(l.display(join(dir, "synod.mod")), src)
		pm.Dir, pm.Version = dir, version
		if pm.Path == "" {
			pm.Path = defPath
		}
		m = pm
		l.diags = append(l.diags, ds...)
	}
	l.mods[dir] = m
	return m
}

// localModule returns the module an override directory provides, or nil if
// the directory is not readable.
func (l *loader) localModule(dir, modPath string) *Module {
	if !isDir(l.fsys, dir) {
		return nil
	}
	return l.moduleAt(dir, modPath, "")
}

// buildList computes the highest required version of every module reachable
// through require lines (minimal version selection), reading the synod.mod of
// every required version that is in vendor/.
func (l *loader) buildList(roots []*Module) {
	visited := map[string]bool{}
	var visit func(reqs map[string]string)
	visit = func(reqs map[string]string) {
		names := make([]string, 0, len(reqs))
		for m := range reqs {
			names = append(names, m)
		}
		sort.Strings(names)
		for _, m := range names {
			v := reqs[m]
			sv, _ := parseSemver(v)
			if cur, ok := l.selected[m]; !ok {
				l.selected[m] = v
			} else if cv, _ := parseSemver(cur); cv.less(sv) {
				l.selected[m] = v
			}
			key := m + "@" + v
			if visited[key] {
				continue
			}
			visited[key] = true
			if l.vendor == "" {
				continue
			}
			if src, err := l.fsys.ReadFile(join(join(l.vendor, key), "synod.mod")); err == nil {
				pm, _ := parseMod("", src)
				visit(pm.Requires)
			}
		}
	}
	for _, r := range roots {
		visit(r.Requires)
	}
}

// reserved are the reserved words of spec/01 section 1.10, except main,
// which names a folder's program.
var reserved = map[string]bool{}

func init() {
	for _, w := range strings.Fields(`use as define native start has set to when after deliver emit
		if else each in result always draw for message with of item list map
		is not at least most less more than and or plus minus times divided by
		true false`) {
		reserved[w] = true
	}
}

func isWord(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		letter := c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
		if !letter && (i == 0 || c < '0' || c > '9') {
			return false
		}
	}
	return true
}

// headerDiag checks the header of a parsed file: the M1 rule, and then that
// the package name is a lower-case word that is not reserved.
func headerDiag(t *parse.File, name string) *source.Diagnostic {
	if d := parse.HeaderDiagnostic(t, name); d != nil {
		return d
	}
	if t.Package == "main" {
		return nil
	}
	var msg string
	switch {
	case t.Package[0] < 'a' || t.Package[0] > 'z':
		msg = `package name "` + t.Package + `" must be a lower-case word`
	case reserved[t.Package]:
		msg = `package name "` + t.Package + `" is a reserved word`
	default:
		return nil
	}
	d := source.New(t.Source, t.Header.Pos, source.BadHeader, msg, "")
	return &d
}

// readDir lists dir for module m. Std folders come from the embedded tree.
func (l *loader) readDir(dir string, m *Module) ([]fs.DirEntry, error) {
	if m.Std {
		return fs.ReadDir(l.std, stdRel(dir))
	}
	return l.fsys.ReadDir(dir)
}

func (l *loader) readFile(dir, name string, m *Module) ([]byte, error) {
	if m.Std {
		return fs.ReadFile(l.std, join(stdRel(dir), name))
	}
	return l.fsys.ReadFile(join(dir, name))
}

// stdRel turns "$std/net" into "net", and "$std" into ".".
func stdRel(dir string) string {
	r := strings.TrimPrefix(strings.TrimPrefix(dir, "$std"), "/")
	if r == "" {
		return "."
	}
	return r
}

func (l *loader) showFile(dir, name string, m *Module) string {
	if m.Std {
		return join("std", join(stdRel(dir), name))
	}
	return l.display(join(dir, name))
}

// folder loads the source files of dir (once) into a library package and a
// main package. It returns nil if the folder cannot be read or holds no
// source file.
func (l *loader) folder(dir string, mod *Module, ipath, name string) *folder {
	if fo, ok := l.folders[dir]; ok {
		return fo
	}
	l.folders[dir] = nil
	if mod.Std && l.std == nil {
		return nil
	}
	ents, err := l.readDir(dir, mod)
	if err != nil {
		return nil
	}
	fo := &folder{}
	var libFiles, mainFiles []*File
	var libBroken bool
	for _, e := range ents {
		if e.IsDir() || !isSynodName(e.Name()) {
			continue
		}
		show := l.showFile(dir, e.Name(), mod)
		src, err := l.readFile(dir, e.Name(), mod)
		sf := source.NewFile(show, src)
		file := &File{Src: sf, Path: join(dir, e.Name())}
		if mod.Std {
			file.Path = show
		}
		if err != nil {
			fo.libDiags = append(fo.libDiags, source.New(sf, source.Pos{Line: 1, Col: 1}, source.Syntax, "cannot read file: "+err.Error(), ""))
			libBroken = true
			libFiles = append(libFiles, file)
			continue
		}
		tree, d := parse.Parse(sf)
		if d != nil {
			fo.libDiags = append(fo.libDiags, *d)
			libBroken = true
			libFiles = append(libFiles, file)
			continue
		}
		file.Tree = tree
		if hd := headerDiag(tree, name); hd != nil {
			fo.libDiags = append(fo.libDiags, *hd)
		}
		if tree.Package == "main" {
			mainFiles = append(mainFiles, file)
		} else {
			libFiles = append(libFiles, file)
		}
	}
	if len(libFiles) == 0 && len(mainFiles) == 0 {
		return nil
	}
	if len(libFiles) > 0 {
		fo.lib = &Package{Path: ipath, Name: name, Dir: dir, Module: mod, IsStd: mod.Std, Broken: libBroken, Files: libFiles}
	}
	if len(mainFiles) > 0 {
		first := mainFiles[0]
		fo.main = &Package{Path: ipath + "#main", Name: "main", Dir: dir, Module: mod, IsMain: true, IsStd: mod.Std, Files: []*File{first}}
		for _, extra := range mainFiles[1:] {
			pos := extra.Tree.Header.Pos
			fo.mainDiags = append(fo.mainDiags, source.New(extra.Src, pos, source.ExtraMain,
				"a second main file in "+l.dirShown(dir, mod)+"; the first is "+first.Src.Path, ""))
		}
	}
	for _, p := range []*Package{fo.lib, fo.main} {
		if p != nil {
			p.Imports = importsOf(p)
		}
	}
	l.folders[dir] = fo
	return fo
}

func (l *loader) dirShown(dir string, m *Module) string {
	if m.Std {
		return join("std", stdRel(dir))
	}
	return l.display(dir)
}

// importsOf lists the use lines of the files of p.
func importsOf(p *Package) []*Import {
	var out []*Import
	for _, f := range p.Files {
		if f.Tree == nil {
			continue
		}
		for _, n := range f.Tree.Body {
			ln, ok := n.(*parse.Line)
			if !ok || len(ln.Sentence.Groups) != 1 {
				continue
			}
			g := ln.Sentence.Groups[0]
			if len(g) < 2 || !parse.IsWord(g[0], "use") {
				continue
			}
			pa, ok := g[1].(*parse.Path)
			if !ok {
				continue
			}
			imp := &Import{File: f, Line: ln, Raw: pa.Text, Path: pa.Text, PathPos: pa.P}
			if i := strings.LastIndexByte(pa.Text, '@'); i >= 0 {
				imp.Path, imp.Version = pa.Text[:i], pa.Text[i+1:]
			}
			if len(g) >= 4 && parse.IsWord(g[2], "as") {
				if w, ok := g[3].(*parse.Word); ok {
					imp.Alias, imp.AliasPos = w.Name, w.P
				}
			}
			out = append(out, imp)
		}
	}
	return out
}

// link resolves the imports of p and of everything it reaches.
func (l *loader) link(p *Package) {
	if l.linked[p] {
		return
	}
	l.linked[p] = true
	l.pkgs[p.Path] = p
	for _, imp := range p.Imports {
		imp.Pkg = l.resolve(p, imp)
		if imp.Pkg != nil {
			l.link(imp.Pkg)
		}
	}
	seen := map[*Package]bool{}
	for _, imp := range p.Imports {
		if imp.Pkg != nil && !seen[imp.Pkg] {
			seen[imp.Pkg] = true
			p.Deps = append(p.Deps, imp.Pkg)
		}
	}
	sort.Slice(p.Deps, func(i, j int) bool { return p.Deps[i].Path < p.Deps[j].Path })
}

func hasPathPrefix(p, prefix string) bool {
	return p == prefix || strings.HasPrefix(p, prefix+"/")
}

// resolve finds the library package that imp names, or reports why not.
func (l *loader) resolve(from *Package, imp *Import) *Package {
	p := imp.Path
	fail := func(msg, hint string) *Package {
		l.diag(imp.File.Src, imp.PathPos, source.UnknownImport, msg, hint)
		return nil
	}
	if last := lastSegment(p); !isWord(last) {
		return fail(`the last segment of "`+imp.Raw+`" is not a word`, "")
	}
	if p == "std" || strings.HasPrefix(p, "std/") {
		rest := strings.TrimPrefix(strings.TrimPrefix(p, "std"), "/")
		if imp.Version != "" {
			return fail(`"`+imp.Raw+`" takes no version: std is versioned with the language`, `write "use `+p+`"`)
		}
		if rest == "" {
			return fail(`no library package at "std"`, `write "use std/<package>"`)
		}
		if fo := l.folder("$std/"+rest, l.stdMod, p, lastSegment(p)); fo != nil && fo.lib != nil {
			return fo.lib
		}
		return fail(`no std package "`+rest+`"`, "")
	}
	if hasPathPrefix(p, StdModule) {
		hint := ""
		if rest := strings.TrimPrefix(strings.TrimPrefix(p, StdModule), "/"); rest != "" {
			hint = `write "use std/` + rest + `"`
		}
		return fail(`"`+imp.Raw+`" cannot be imported by the std module path`, hint)
	}

	mod, rel, ok := l.moduleOf(from, p)
	if !ok {
		hint := `add "require <module> <version>" to synod.mod`
		if from.Module.Path == "" && !from.IsStd {
			hint = "no synod.mod found above " + l.dirShown(from.Dir, from.Module)
		}
		return fail(`no module provides "`+imp.Raw+`"`, hint)
	}
	if mod.Version == "" && imp.Version != "" {
		return fail(`"`+imp.Raw+`" is a local package and takes no version`, `write "use `+p+`"`)
	}
	if mod.Version != "" && imp.Version != "" && imp.Version != mod.Version {
		return fail(`"`+p+`" asks for `+imp.Version+` but synod.mod selects `+mod.Version, "")
	}
	if mod.Dir == "" {
		return fail(`module "`+mod.Path+"@"+mod.Version+`" is not in vendor/`,
			`run "synod get `+mod.Path+"@"+mod.Version+`"`)
	}
	dir := mod.Dir
	if rel != "" {
		for _, seg := range strings.Split(rel, "/") {
			dir = join(dir, seg)
			if _, err := l.fsys.ReadFile(join(dir, "synod.mod")); err == nil {
				return fail(`"`+imp.Raw+`" lies in a nested module`, "import it by the path of its own module")
			}
		}
	}
	if fo := l.folder(dir, mod, p, lastSegment(p)); fo != nil {
		if fo.lib != nil {
			return fo.lib
		}
		return fail(`"`+p+`" holds only a main file`, "")
	}
	return fail(`no library package at "`+p+`"`, "")
}

// moduleOf finds the module that provides import path p for a package of
// from, and the folder of p below the module root. The module's own path
// comes first, then the overrides, then the build list. A vendored
// module that is missing gets an empty Dir.
func (l *loader) moduleOf(from *Package, p string) (*Module, string, bool) {
	rel := func(mp string) string { return strings.TrimPrefix(strings.TrimPrefix(p, mp), "/") }
	if m := from.Module; !m.Std && m.Path != "" && hasPathPrefix(p, m.Path) {
		return m, rel(m.Path), true
	}
	for _, k := range l.overKeys {
		if hasPathPrefix(p, k) {
			if m := l.localModule(l.over[k], k); m != nil {
				return m, rel(k), true
			}
		}
	}
	best := ""
	for m := range l.selected {
		if hasPathPrefix(p, m) && len(m) > len(best) {
			best = m
		}
	}
	if best == "" {
		return nil, "", false
	}
	v := l.selected[best]
	dir := ""
	if l.vendor != "" {
		dir = join(l.vendor, best+"@"+v)
		if !isDir(l.fsys, dir) {
			dir = ""
		}
	}
	if dir == "" {
		return &Module{Path: best, Version: v}, rel(best), true
	}
	return l.moduleAt(dir, best, v), rel(best), true
}

// order fills g.Packages (dependencies first), reports import cycles and
// adds the diagnostics of the packages that joined the graph.
func (l *loader) order(g *Graph) {
	seen := map[*Package]bool{}
	var visit func(p *Package)
	visit = func(p *Package) {
		if seen[p] {
			return
		}
		seen[p] = true
		for _, d := range p.Deps {
			visit(d)
		}
		g.Packages = append(g.Packages, p)
	}
	for _, t := range g.Targets {
		visit(t)
	}
	for _, p := range g.Packages {
		fo := l.folders[p.Dir]
		if fo == nil {
			continue
		}
		if p.IsMain {
			l.diags = append(l.diags, fo.mainDiags...)
		} else {
			l.diags = append(l.diags, fo.libDiags...)
		}
	}
	l.cycles(g.Packages)

	mods := map[*Module]bool{}
	for _, m := range l.mods {
		mods[m] = true
	}
	for m := range mods {
		g.Modules = append(g.Modules, m)
	}
	sort.Slice(g.Modules, func(i, j int) bool {
		a, b := g.Modules[i], g.Modules[j]
		if a.Path != b.Path {
			return a.Path < b.Path
		}
		if a.Version != b.Version {
			return a.Version < b.Version
		}
		return a.Dir < b.Dir
	})
}

// cycles reports every import cycle once.
func (l *loader) cycles(pkgs []*Package) {
	sorted := append([]*Package(nil), pkgs...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Path < sorted[j].Path })
	state := map[*Package]int{} // 1 on the stack, 2 done
	var stack []*Package
	reported := map[string]bool{}
	var dfs func(p *Package)
	dfs = func(p *Package) {
		state[p] = 1
		stack = append(stack, p)
		for _, d := range p.Deps {
			switch state[d] {
			case 0:
				dfs(d)
			case 1:
				i := len(stack) - 1
				for stack[i] != d {
					i--
				}
				l.reportCycle(append([]*Package(nil), stack[i:]...), reported)
			}
		}
		stack = stack[:len(stack)-1]
		state[p] = 2
	}
	for _, p := range sorted {
		if state[p] == 0 {
			dfs(p)
		}
	}
}

func (l *loader) reportCycle(cyc []*Package, reported map[string]bool) {
	min := 0
	for i, p := range cyc {
		if p.Path < cyc[min].Path {
			min = i
		}
	}
	cyc = append(append([]*Package(nil), cyc[min:]...), cyc[:min]...)
	var key []string
	short := map[string]bool{}
	full := false
	for _, p := range cyc {
		key = append(key, p.Path)
		s := lastSegment(p.Path)
		if short[s] {
			full = true
		}
		short[s] = true
	}
	k := strings.Join(key, "\n")
	if reported[k] {
		return
	}
	reported[k] = true
	names := make([]string, 0, len(cyc)+1)
	for _, p := range append(append([]*Package(nil), cyc...), cyc[0]) {
		if full {
			names = append(names, p.Path)
		} else {
			names = append(names, lastSegment(p.Path))
		}
	}
	next := cyc[0]
	if len(cyc) > 1 {
		next = cyc[1]
	}
	for _, imp := range cyc[0].Imports {
		if imp.Pkg == next {
			l.diag(imp.File.Src, imp.PathPos, source.ImportCycle, "import cycle: "+strings.Join(names, " → "), "")
			return
		}
	}
}
