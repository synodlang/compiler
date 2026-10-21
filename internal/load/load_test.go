package load

import (
	"bytes"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/synodlang/compiler/internal/source"
)

var update = flag.Bool("update", false, "rewrite testdata/load/*/want.txt")

var testdata = filepath.Join("..", "..", "testdata", "load")

// A case is the directory testdata/load/<name>. case.txt holds directives:
//
//	root <dir>          the module tree, default "fs"
//	target <path>       a target below the root, default "."
//	discover            targets are all folders below the root
//	module <path>=<dir> a Config.Modules override
//
// Directories are relative to the case. The std tree is testdata/load/std.
type loadCase struct {
	name     string
	root     string // absolute slash path
	targets  []string
	discover bool
	modules  map[string]string
}

func readCase(t *testing.T, name string) loadCase {
	t.Helper()
	dir := filepath.Join(testdata, name)
	c := loadCase{name: name, modules: map[string]string{}}
	abs := func(rel string) string {
		a, err := filepath.Abs(filepath.Join(dir, rel))
		if err != nil {
			t.Fatal(err)
		}
		return filepath.ToSlash(a)
	}
	c.root = abs("fs")
	src, err := os.ReadFile(filepath.Join(dir, "case.txt"))
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(string(src), "\n") {
		key, val, _ := strings.Cut(strings.TrimSpace(line), " ")
		switch key {
		case "":
		case "root":
			c.root = abs(val)
		case "target":
			c.targets = append(c.targets, val)
		case "discover":
			c.discover = true
		case "module":
			mp, d, _ := strings.Cut(val, "=")
			c.modules[mp] = abs(d)
		default:
			t.Fatalf("%s: bad directive %q", name, line)
		}
	}
	return c
}

// readTree loads every file below dir into m under the virtual directory to.
func readTree(t *testing.T, m MapFS, dir, to string) {
	t.Helper()
	err := filepath.WalkDir(dir, func(p string, e fs.DirEntry, err error) error {
		if err != nil || e.IsDir() {
			return err
		}
		src, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(dir, p)
		m[to+"/"+filepath.ToSlash(rel)] = string(src)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func stdFS(t *testing.T) fs.FS {
	t.Helper()
	return os.DirFS(filepath.Join(testdata, "std"))
}

// shown maps a path below one of the mounts to a stable display path.
func shown(p string, mounts map[string]string) string {
	keys := make([]string, 0, len(mounts))
	for k := range mounts {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return len(keys[i]) > len(keys[j]) })
	for _, k := range keys {
		if p == k {
			return mounts[k]
		}
		if strings.HasPrefix(p, k+"/") {
			return path.Join(mounts[k], p[len(k)+1:])
		}
	}
	return p
}

// run loads the case through the real disk (mem false) or an in-memory copy
// (mem true) and returns the dump.
func (c loadCase) run(t *testing.T, mem bool) string {
	t.Helper()
	cfg := Config{Std: stdFS(t)}
	root := c.root
	mods := map[string]string{}
	for k, v := range c.modules {
		mods[k] = v
	}
	var fsys FS = OSFS{}
	if mem {
		m := MapFS{}
		readTree(t, m, root, "/m")
		for k, d := range mods {
			if strings.HasPrefix(d, root+"/") {
				mods[k] = "/m" + d[len(root):]
				continue
			}
			readTree(t, m, d, "/mod/"+k)
			mods[k] = "/mod/" + k
		}
		fsys, root = m, "/m"
	}
	mounts := map[string]string{root: "."}
	for k, d := range mods {
		if !strings.HasPrefix(d, root+"/") {
			mounts[d] = "$mod/" + k
		}
	}
	cfg.FS, cfg.Modules = fsys, mods
	cfg.Display = func(p string) string { return shown(p, mounts) }

	targets := c.targets
	if c.discover {
		dirs, err := Discover(fsys, root)
		if err != nil {
			t.Fatal(err)
		}
		targets = dirs
	} else {
		if len(targets) == 0 {
			targets = []string{"."}
		}
		var abs []string
		for _, tg := range targets {
			abs = append(abs, path.Join(root, tg))
		}
		targets = abs
	}
	g, ds := Load(cfg, targets)
	return dump(g, ds, mounts)
}

func dump(g *Graph, ds []source.Diagnostic, mounts map[string]string) string {
	var b strings.Builder
	showDir := func(d string) string {
		if strings.HasPrefix(d, "$std") {
			return "std" + strings.TrimPrefix(d, "$std")
		}
		return shown(d, mounts)
	}
	for _, p := range g.Packages {
		fmt.Fprintf(&b, "package %s name=%s dir=%s module=%s", p.Path, p.Name, showDir(p.Dir), p.Module.Path)
		if p.Module.Version != "" {
			fmt.Fprintf(&b, "@%s", p.Module.Version)
		}
		for _, f := range []struct {
			on   bool
			name string
		}{{p.IsMain, "main"}, {p.IsStd, "std"}, {p.Broken, "broken"}} {
			if f.on {
				b.WriteString(" " + f.name)
			}
		}
		b.WriteByte('\n')
		for _, f := range p.Files {
			state := ""
			if f.Tree == nil {
				state = " (unparsed)"
			}
			fmt.Fprintf(&b, "  file %s%s\n", f.Src.Path, state)
		}
		for _, imp := range p.Imports {
			to := "UNRESOLVED"
			if imp.Pkg != nil {
				to = imp.Pkg.Path
			}
			alias := ""
			if imp.Alias != "" {
				alias = " as " + imp.Alias
			}
			fmt.Fprintf(&b, "  import %s%s at %d:%d -> %s\n", imp.Raw, alias, imp.PathPos.Line, imp.PathPos.Col, to)
		}
		var deps []string
		for _, d := range p.Deps {
			deps = append(deps, d.Path)
		}
		fmt.Fprintf(&b, "  deps %s\n", strings.Join(deps, " "))
	}
	var tg []string
	for _, p := range g.Targets {
		tg = append(tg, p.Path)
	}
	fmt.Fprintf(&b, "targets %s\n", strings.Join(tg, " "))
	for _, m := range g.Modules {
		fmt.Fprintf(&b, "module %q dir=%s version=%s synod=%s requires=%d\n", m.Path, showDir(m.Dir), m.Version, m.Synod, len(m.Requires))
	}
	fmt.Fprintf(&b, "diagnostics %d\n", len(ds))
	for _, d := range ds {
		b.WriteString(d.String() + "\n")
	}
	return b.String()
}

func caseNames(t *testing.T) []string {
	t.Helper()
	ents, err := os.ReadDir(testdata)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, e := range ents {
		if e.IsDir() && e.Name() != "std" {
			out = append(out, e.Name())
		}
	}
	return out
}

func TestCases(t *testing.T) {
	names := caseNames(t)
	if len(names) < 20 {
		t.Fatalf("only %d cases", len(names))
	}
	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			c := readCase(t, name)
			got := c.run(t, false)
			if again := c.run(t, false); again != got {
				t.Errorf("two runs differ:\n%s\n---\n%s", got, again)
			}
			if mem := c.run(t, true); mem != got {
				t.Errorf("MapFS differs from OSFS:\n%s\n---\n%s", mem, got)
			}
			want := filepath.Join(testdata, name, "want.txt")
			if *update {
				if err := os.WriteFile(want, []byte(got), 0o644); err != nil {
					t.Fatal(err)
				}
				return
			}
			src, err := os.ReadFile(want)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(src, []byte(got)) {
				t.Errorf("want.txt differs (run with -update):\n%s", got)
			}
		})
	}
}
