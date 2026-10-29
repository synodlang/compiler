package main

import (
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/synodlang/compiler/internal/load"
)

// libFile is the library file `synod new` writes (decision 94): a node that
// counts once a second.
const libFile = `# package %[1]s
use std/time

Node has
  set Count to 0

# Count up once a second
define [n: Node] start
  time.Every 1s
    set n.Count to (n.Count plus 1)
`

// demoFile is the main file: it spawns the node, crashes it and stops.
const demoFile = `# package main
use %[1]s/%[2]s
use std/sim

define main
  set first to (sim.Spawn %[2]s)
  after 3s
    sim.Crash first
  sim.StopAt 6s
`

// runNew implements `synod new <dir> [--module path] [--lib]`.
func runNew(args []string, stdout, stderr io.Writer) int {
	paths, set, vals, err := parseFlags(args, []string{"lib"}, []string{"module"})
	if err != nil {
		fmt.Fprintf(stderr, "synod new: %v\n", err)
		return exitUsage
	}
	if len(paths) != 1 {
		fmt.Fprintln(stderr, "usage: synod new <dir> [--module path] [--lib]")
		return exitUsage
	}
	dir := paths[0]
	if err := checkNewDir(dir); err != nil {
		fmt.Fprintf(stderr, "synod new: %v\n", err)
		return exitUsage
	}
	module, hasModule := vals["module"]
	if !hasModule {
		abs, err := filepath.Abs(dir)
		if err != nil {
			fmt.Fprintf(stderr, "synod new: %v\n", err)
			return exitUsage
		}
		module = filepath.Base(abs)
	}
	if !load.ValidModulePath(module) {
		fmt.Fprintf(stderr, "synod new: %q is not a module path\n", module)
		return exitUsage
	}
	pkg := path.Base(module)
	if load.PackageNameProblem(pkg) != "" || pkg == "main" || pkg == "demo" || pkg == "time" || pkg == "sim" {
		fmt.Fprintf(stderr, "synod new: %q is not a package name; use a lower-case word\n", pkg)
		return exitUsage
	}
	files := []struct{ name, text string }{
		{"synod.mod", "module " + module + "\nsynod " + load.LanguageVersion + "\n"},
		{".gitignore", "vendor/\nout/\n"},
		{pkg + "/" + pkg + ".synod", fmt.Sprintf(libFile, pkg)},
	}
	if !set["lib"] {
		files = append(files, struct{ name, text string }{pkg + "/demo.synod", fmt.Sprintf(demoFile, module, pkg)})
	}
	var names []string
	for _, f := range files {
		p := filepath.Join(dir, filepath.FromSlash(f.name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			fmt.Fprintf(stderr, "synod new: %v\n", err)
			return exitUsage
		}
		if err := os.WriteFile(p, []byte(f.text), 0o644); err != nil {
			fmt.Fprintf(stderr, "synod new: %v\n", err)
			return exitUsage
		}
		names = append(names, f.name)
	}
	fmt.Fprintf(stdout, "created %s: %s\n", dir, strings.Join(names, ", "))
	return exitOK
}

// checkNewDir accepts a folder that does not exist or is empty.
func checkNewDir(dir string) error {
	info, err := os.Stat(dir)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("%s is not a folder", dir)
	}
	ents, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	if len(ents) > 0 {
		return fmt.Errorf("%s is not empty", dir)
	}
	return nil
}
