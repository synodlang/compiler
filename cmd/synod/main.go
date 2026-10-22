// Command synod is the Synod compiler command line (implementation/05-cli.md).
// It implements new, fmt, check, run, get, doc, explain, mcp and version.
package main

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/synodlang/compiler/internal/source"
)

// formatFn is the formatter without fixes. It is bound to internal/format in
// binding.go and replaced by a fake in unit tests.
var formatFn func(f *source.File) ([]byte, *source.Diagnostic)

const usage = `usage: synod <command> [arguments]

commands:
  new <dir> [--module path] [--lib]
  fmt [paths...] [--check] [--style rich|short] [--color auto|always|never]
  check [paths...] [--json] [--tokens] [--overlay file] [--style rich|short] [--color auto|always|never]
  run <folder> [--seed n] [--trace file] [--scene file] [--pace x] [--mode ordered|proportional] [--summary] [--style rich|short] [--color auto|always|never]
  get [module@version]
  doc [package] [--json] [--catalog] [--scope file] [--overlay file] [--style rich|short] [--color auto|always|never]
  explain [code] [--json] [--catalog] [--style rich|short] [--color auto|always|never]
  mcp [--root dir]
  version
`

// Exit codes (implementation/05-cli.md).
const (
	exitOK    = 0
	exitDiags = 1
	exitUsage = 2
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

// run executes one command line and returns the exit code.
func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, usage)
		return exitUsage
	}
	cmd, rest := args[0], args[1:]
	switch cmd {
	case "fmt":
		return runFmt(rest, stdout, stderr)
	case "check":
		return runCheck(rest, stdout, stderr)
	case "run":
		return runRun(rest, stdout, stderr)
	case "version":
		return runVersion(rest, stdout, stderr)
	case "new":
		return runNew(rest, stdout, stderr)
	case "get":
		return runGet(rest, stdout, stderr)
	case "doc":
		return runDoc(rest, stdout, stderr)
	case "explain":
		return runExplain(rest, stdout, stderr)
	case "mcp":
		return runMCP(rest, stdin, stdout, stderr)
	case "help", "-h", "--help":
		fmt.Fprint(stdout, usage)
		return exitOK
	}
	fmt.Fprintf(stderr, "synod: unknown command %q\n%s", cmd, usage)
	return exitUsage
}

// parseArgs separates args into positional paths and the known boolean flags.
// Flags may appear anywhere; "--" ends flag parsing. An unknown flag is an
// error.
func parseArgs(args []string, known ...string) (paths []string, set map[string]bool, err error) {
	paths, set, _, err = parseFlags(args, known, nil)
	return paths, set, err
}

// parseFlags is parseArgs with value flags too: "--name v" and "--name=v".
// A boolean flag takes no value; a value flag given twice keeps the last.
func parseFlags(args []string, bools, vals []string) (paths []string, set map[string]bool, values map[string]string, err error) {
	set = map[string]bool{}
	values = map[string]string{}
	has := func(list []string, n string) bool {
		for _, k := range list {
			if k == n {
				return true
			}
		}
		return false
	}
	flags := true
	for i := 0; i < len(args); i++ {
		a := args[i]
		if !flags || !strings.HasPrefix(a, "-") || a == "-" {
			paths = append(paths, a)
			continue
		}
		if a == "--" {
			flags = false
			continue
		}
		name := strings.TrimLeft(a, "-")
		val, hasVal := "", false
		if k := strings.IndexByte(name, '='); k >= 0 {
			name, val, hasVal = name[:k], name[k+1:], true
		}
		switch {
		case has(bools, name):
			if hasVal {
				return nil, nil, nil, fmt.Errorf("--%s takes no value", name)
			}
			set[name] = true
		case has(vals, name):
			if !hasVal {
				if i+1 >= len(args) {
					return nil, nil, nil, fmt.Errorf("--%s needs a value", name)
				}
				i++
				val = args[i]
			}
			values[name] = val
		default:
			return nil, nil, nil, fmt.Errorf("unknown flag %q", a)
		}
	}
	return paths, set, values, nil
}

// styleOf validates the --style and --color values of a command line.
func styleOf(values map[string]string) (styleFlags, error) {
	sf := styleFlags{style: values["style"], color: values["color"]}
	return sf, sf.check()
}
