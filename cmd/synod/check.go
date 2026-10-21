package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/synodlang/compiler/internal/parse"
	"github.com/synodlang/compiler/internal/source"
)

func runCheck(args []string, stdout, stderr io.Writer) int {
	paths, flags, err := parseArgs(args, "json", "tokens")
	if err != nil {
		fmt.Fprintf(stderr, "synod check: %v\n", err)
		return exitUsage
	}
	if flags["tokens"] {
		fmt.Fprintln(stderr, "synod check: --tokens is not available until M2")
		return exitUsage
	}
	files, err := collect(paths)
	if err != nil {
		fmt.Fprintf(stderr, "synod check: %v\n", err)
		return exitUsage
	}
	var diags []source.Diagnostic
	ioErr := false
	for _, path := range files {
		src, err := os.ReadFile(path)
		if err != nil {
			fmt.Fprintf(stderr, "synod check: %v\n", err)
			ioErr = true
			continue
		}
		diags = append(diags, checkFile(source.NewFile(path, src))...)
	}
	source.Sort(diags)
	if flags["json"] {
		if err := source.WriteJSON(stdout, diags); err != nil {
			fmt.Fprintf(stderr, "synod check: %v\n", err)
			return exitUsage
		}
	} else {
		for _, d := range diags {
			fmt.Fprintln(stdout, d.String())
		}
	}
	switch {
	case ioErr:
		return exitUsage
	case len(diags) > 0:
		return exitDiags
	}
	return exitOK
}

// checkFile returns the diagnostics of one file: the first syntax or indent
// error, else the first non-canonical difference, plus a bad-header
// diagnostic when the file parses.
func checkFile(f *source.File) []source.Diagnostic {
	var out []source.Diagnostic
	tree, d := parse.Parse(f)
	if d != nil {
		return append(out, *d)
	}
	if d := checkFn(f); d != nil {
		out = append(out, *d)
	}
	folder := "."
	if abs, err := filepath.Abs(filepath.Dir(f.Path)); err == nil {
		folder = filepath.Base(abs)
	}
	if d := parse.HeaderDiagnostic(tree, folder); d != nil {
		out = append(out, *d)
	}
	return out
}
