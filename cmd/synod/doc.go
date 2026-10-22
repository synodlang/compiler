package main

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/synodlang/compiler/internal/doc"
	"github.com/synodlang/compiler/internal/driver"
	"github.com/synodlang/compiler/internal/load"
	"github.com/synodlang/compiler/internal/pipeline"
	"github.com/synodlang/compiler/internal/resolve"
	"github.com/synodlang/compiler/internal/source"
)

// virtualDir is the folder of the one-line program that runDoc loads to
// resolve an import path the way a program would.
const virtualDir = "_synoddoc"

// docFS adds a folder with one file to a file system, so that the loader
// resolves "use <path>" with its own rules (the main module, vendor/, the
// build list).
type docFS struct {
	base load.FS
	dir  string // absolute slash path of the virtual folder
	data []byte
}

func (d docFS) abs(name string) string {
	if a, ok := d.base.(interface{ Abs(string) (string, error) }); ok {
		if r, err := a.Abs(name); err == nil {
			return r
		}
	}
	return path.Clean(name)
}

func (d docFS) ReadFile(name string) ([]byte, error) {
	if d.abs(name) == d.dir+"/main.synod" {
		return d.data, nil
	}
	return d.base.ReadFile(name)
}

type virtualFile struct{ size int }

func (virtualFile) Name() string       { return "main.synod" }
func (v virtualFile) Size() int64      { return int64(v.size) }
func (virtualFile) Mode() fs.FileMode  { return 0o644 }
func (virtualFile) ModTime() time.Time { return time.Time{} }
func (virtualFile) IsDir() bool        { return false }
func (virtualFile) Sys() any           { return nil }

func (d docFS) ReadDir(name string) ([]fs.DirEntry, error) {
	if d.abs(name) == d.dir {
		return []fs.DirEntry{fs.FileInfoToDirEntry(virtualFile{len(d.data)})}, nil
	}
	return d.base.ReadDir(name)
}

func (d docFS) Abs(name string) (string, error) { return d.abs(name), nil }

func (d docFS) Display(name string) string {
	if x, ok := d.base.(interface{ Display(string) string }); ok {
		return x.Display(name)
	}
	return name
}

// moduleRoot returns the absolute slash path of the folder with the nearest
// synod.mod at or above the working directory, and the module path; "" when
// there is none.
func moduleRoot(fsys load.FS) (dir, mod string) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", ""
	}
	for d := filepath.ToSlash(cwd); ; {
		if src, err := fsys.ReadFile(d + "/synod.mod"); err == nil {
			m, _ := load.ParseMod("", src)
			return d, m.Path
		}
		p := path.Dir(d)
		if p == d || p == "." {
			return "", ""
		}
		d = p
	}
}

func runDoc(args []string, stdout, stderr io.Writer) int {
	paths, flags, values, err := parseFlags(args, []string{"json", "catalog"}, []string{"scope", "overlay", "style", "color"})
	if err != nil {
		fmt.Fprintf(stderr, "synod doc: %v\n", err)
		return exitUsage
	}
	sf, err := styleOf(values)
	if err != nil {
		fmt.Fprintf(stderr, "synod doc: %v\n", err)
		return exitUsage
	}
	usage := func(format string, a ...any) int {
		fmt.Fprintf(stderr, "synod doc: "+format+"\n", a...)
		return exitUsage
	}
	scopeFile, hasScope := values["scope"]
	switch {
	case len(paths) > 1:
		return usage("takes at most one package")
	case flags["catalog"] && (len(paths) > 0 || flags["json"] || hasScope):
		return usage("--catalog takes no package and cannot be combined with --json or --scope")
	case hasScope && !flags["json"]:
		return usage("--scope needs --json")
	case hasScope && len(paths) > 0:
		return usage("--scope takes no package")
	}
	var fsys load.FS = load.OSFS{}
	if file, ok := values["overlay"]; ok {
		o, err := readOverlay(file)
		if err != nil {
			return usage("overlay: %v", err)
		}
		fsys = o
	}
	switch {
	case flags["catalog"]:
		return docCatalog(stdout, stderr)
	case hasScope:
		return docScope(scopeFile, fsys, stdout, stderr)
	case len(paths) == 0:
		return docList(fsys, stdout, stderr)
	}
	return docPackage(paths[0], fsys, flags["json"], sf, stdout, stderr)
}

func docCatalog(stdout, stderr io.Writer) int {
	all := doc.StdPackages()
	res := driver.Check(driver.Options{Load: load.Config{ExtraStd: all}}, nil)
	var pkgs []*doc.Package
	for _, p := range all {
		if rp := res.ByPath[p]; rp != nil {
			pkgs = append(pkgs, doc.Build(res, rp, doc.ViewOf(rp)))
		}
	}
	fmt.Fprint(stdout, doc.Catalog(pkgs))
	return exitOK
}

// libraryFolders lists the library packages below root (a folder of a
// module whose path is mod) as import paths.
func libraryFolders(fsys load.FS, root, mod string) []string {
	dirs, err := load.Discover(fsys, root)
	if err != nil {
		return nil
	}
	var out []string
	for _, d := range dirs {
		if !hasLibraryFile(fsys, d) {
			continue
		}
		rel := strings.Trim(strings.TrimPrefix(d, root), "/")
		switch {
		case rel == "" || rel == ".":
			out = append(out, mod)
		default:
			out = append(out, mod+"/"+rel)
		}
	}
	return out
}

// hasLibraryFile reports whether the folder holds a file that is not a main
// file.
func hasLibraryFile(fsys load.FS, dir string) bool {
	ents, err := fsys.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, e := range ents {
		n := e.Name()
		if e.IsDir() || !strings.HasSuffix(n, ".synod") || strings.HasPrefix(n, ".") || strings.HasPrefix(n, "_") {
			continue
		}
		src, err := fsys.ReadFile(strings.TrimSuffix(dir, "/") + "/" + n)
		if err != nil {
			continue
		}
		first, _, _ := strings.Cut(string(src), "\n")
		if strings.TrimRight(first, " \r") != "# package main" {
			return true
		}
	}
	return false
}

func docList(fsys load.FS, stdout, stderr io.Writer) int {
	var es []doc.Entry
	for _, p := range doc.StdPackages() {
		es = append(es, doc.Entry{Path: p, Title: doc.Title(p)})
	}
	root, mod := moduleRoot(fsys)
	if root == "" || mod == "" {
		if cwd, err := os.Getwd(); err == nil && hasLibraryFile(fsys, filepath.ToSlash(cwd)) {
			es = append(es, doc.Entry{Path: "."})
		}
	} else {
		own := libraryFolders(fsys, root, mod)
		sort.Strings(own)
		for _, p := range own {
			es = append(es, doc.Entry{Path: p})
		}
		es = append(es, vendoredPackages(fsys, root)...)
	}
	if err := doc.WriteList(stdout, es); err != nil {
		fmt.Fprintf(stderr, "synod doc: %v\n", err)
		return exitUsage
	}
	return exitOK
}

// vendoredPackages lists the library packages of the modules in vendor/,
// the highest version of each module.
func vendoredPackages(fsys load.FS, root string) []doc.Entry {
	type mv struct{ mod, ver, dir string }
	best := map[string]mv{}
	var walk func(dir, prefix string)
	walk = func(dir, prefix string) {
		ents, err := fsys.ReadDir(dir)
		if err != nil {
			return
		}
		for _, e := range ents {
			if !e.IsDir() {
				continue
			}
			name := e.Name()
			if m, v, ok := strings.Cut(name, "@"); ok {
				mod := prefix + m
				if cur, have := best[mod]; !have || load.VersionLess(cur.ver, v) {
					best[mod] = mv{mod, v, dir + "/" + name}
				}
				continue
			}
			walk(dir+"/"+name, prefix+name+"/")
		}
	}
	walk(root+"/vendor", "")
	mods := make([]string, 0, len(best))
	for m := range best {
		mods = append(mods, m)
	}
	sort.Strings(mods)
	var out []doc.Entry
	for _, m := range mods {
		ps := libraryFolders(fsys, best[m].dir, m)
		sort.Strings(ps)
		for _, p := range ps {
			out = append(out, doc.Entry{Path: p})
		}
	}
	return out
}

// noPackage reports an unknown package argument.
func noPackage(stderr io.Writer, arg, detail string) int {
	fmt.Fprintf(stderr, "synod doc: no package %q\n", arg)
	if detail != "" {
		fmt.Fprintf(stderr, "  %s\n", detail)
	}
	return exitUsage
}

func docPackage(arg string, fsys load.FS, asJSON bool, sf styleFlags, stdout, stderr io.Writer) int {
	var (
		res    driver.Result
		rp     *resolve.Package
		hidden string
	)
	std := doc.StdPackages()
	isStd := func(p string) bool {
		i := sort.SearchStrings(std, p)
		return i < len(std) && std[i] == p
	}
	cfg := load.Config{FS: fsys}
	info, statErr := os.Stat(arg)
	switch {
	case strings.HasPrefix(arg, "std/") || arg == "std":
		if !isStd(arg) {
			return noPackage(stderr, arg, "")
		}
		cfg.ExtraStd = []string{arg}
		res = driver.Check(driver.Options{Load: cfg}, nil)
		rp = res.ByPath[arg]
	case statErr == nil && info.IsDir():
		res = driver.Check(driver.Options{Load: cfg}, []string{filepath.ToSlash(filepath.Clean(arg))})
		broken := false
		for _, t := range res.Graph.Targets {
			if t.Broken {
				broken = true
			}
			if !t.IsMain {
				rp = res.ByPath[t.Path]
			}
		}
		if rp == nil && !broken {
			fmt.Fprintf(stderr, "synod doc: %s has no library package\n", arg)
			return exitUsage
		}
	default:
		if !strings.Contains(arg, "/") {
			hint := ""
			for _, p := range std {
				if p == "std/"+arg {
					hint = `hint: did you mean "` + p + `"?`
				}
			}
			return noPackage(stderr, arg, hint)
		}
		root, _ := moduleRoot(fsys)
		if root == "" {
			return noPackage(stderr, arg, "hint: no synod.mod found here or above")
		}
		dir := root + "/" + virtualDir
		vfs := docFS{base: fsys, dir: dir, data: []byte("# package main\nuse " + arg + "\n")}
		cfg.FS = vfs
		hidden = vfs.Display(dir + "/main.synod")
		res = driver.Check(driver.Options{Load: cfg}, []string{dir})
		for _, t := range res.Graph.Targets {
			for _, imp := range t.Imports {
				if imp.Pkg != nil {
					rp = res.ByPath[imp.Pkg.Path]
				}
			}
		}
		if rp == nil {
			detail := ""
			for _, d := range res.Diags {
				if d.File == hidden {
					detail = d.Message
					break
				}
			}
			return noPackage(stderr, arg, detail)
		}
	}
	diags := dropFile(pipeline.UserDiags(res), hidden)
	if rp == nil {
		// The package is broken: only diagnostics to show.
		if len(diags) == 0 {
			return noPackage(stderr, arg, "")
		}
		return docDiags(diags, res, asJSON, nil, sf, stdout, stderr)
	}
	pkg := doc.Build(res, rp, doc.ViewOf(rp))
	if len(diags) > 0 {
		return docDiags(diags, res, asJSON, []*doc.Package{pkg}, sf, stdout, stderr)
	}
	if asJSON {
		if err := doc.WriteJSON(stdout, nil, nil, []*doc.Package{pkg}); err != nil {
			fmt.Fprintf(stderr, "synod doc: %v\n", err)
			return exitUsage
		}
		return exitOK
	}
	rich, color := sf.resolve(stdout)
	if err := doc.Write(stdout, pkg, doc.Style{Rich: rich, Color: color}); err != nil {
		fmt.Fprintf(stderr, "synod doc: %v\n", err)
		return exitUsage
	}
	return exitOK
}

func dropFile(ds []source.Diagnostic, file string) []source.Diagnostic {
	if file == "" {
		return ds
	}
	var out []source.Diagnostic
	for _, d := range ds {
		if d.File != file {
			out = append(out, d)
		}
	}
	return out
}

// docDiags prints the diagnostics as synod check does (the JSON document
// also carries the packages that could be built) and returns exit 1.
func docDiags(diags []source.Diagnostic, res driver.Result, asJSON bool, pkgs []*doc.Package, sf styleFlags, stdout, stderr io.Writer) int {
	if asJSON {
		if err := doc.WriteJSON(stdout, diags, nil, pkgs); err != nil {
			fmt.Fprintf(stderr, "synod doc: %v\n", err)
			return exitUsage
		}
		return exitDiags
	}
	if err := writeDiags(stdout, diags, sf, &sources{known: res.Sources}); err != nil {
		fmt.Fprintf(stderr, "synod doc: %v\n", err)
		return exitUsage
	}
	return exitDiags
}

func docScope(file string, fsys load.FS, stdout, stderr io.Writer) int {
	info, err := os.Stat(file)
	if err != nil || info.IsDir() || !strings.HasSuffix(file, ".synod") {
		fmt.Fprintf(stderr, "synod doc: --scope %s: no such .synod file\n", file)
		return exitUsage
	}
	target := filepath.ToSlash(filepath.Clean(file))
	res := driver.Check(driver.Options{Load: load.Config{FS: fsys}}, []string{target})
	diags := pipeline.UserDiags(res)
	abs, _ := filepath.Abs(file)
	var (
		rp *resolve.Package
		rf *resolve.File
	)
	for _, p := range res.Packages {
		for _, f := range p.Files {
			if fa := absOf(fsys, f.Load.Path); fa == filepath.ToSlash(abs) || f.Path == target {
				rp, rf = p, f
			}
		}
	}
	var sc *doc.Scope
	var pkgs []*doc.Package
	if rp != nil {
		sc, pkgs = doc.BuildScope(res, rp, rf)
	}
	if err := doc.WriteJSON(stdout, diags, sc, pkgs); err != nil {
		fmt.Fprintf(stderr, "synod doc: %v\n", err)
		return exitUsage
	}
	if len(diags) > 0 {
		return exitDiags
	}
	return exitOK
}

func absOf(fsys load.FS, name string) string {
	if a, ok := fsys.(interface{ Abs(string) (string, error) }); ok {
		if r, err := a.Abs(name); err == nil {
			return r
		}
	}
	return path.Clean(name)
}
