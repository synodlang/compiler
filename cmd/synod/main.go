// Command synod is the Synod compiler command line (implementation/05-cli.md).
// This milestone implements fmt, check, run and version.
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
  fmt [paths...] [--check]
  check [paths...] [--json] [--tokens]
  run <folder> [--seed n] [--trace file] [--summary]
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
	case "new", "get", "doc", "explain":
		fmt.Fprintf(stderr, "synod: %s: not implemented yet\n", cmd)
		return exitUsage
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
	set = map[string]bool{}
	flags := true
	for _, a := range args {
		switch {
		case flags && a == "--":
			flags = false
		case flags && strings.HasPrefix(a, "-") && a != "-":
			name := strings.TrimLeft(a, "-")
			ok := false
			for _, k := range known {
				if k == name {
					ok = true
				}
			}
			if !ok {
				return nil, nil, fmt.Errorf("unknown flag %q", a)
			}
			set[name] = true
		default:
			paths = append(paths, a)
		}
	}
	return paths, set, nil
}
