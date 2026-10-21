package main

import (
	"bytes"
	"fmt"
	"io"
	"os"

	"github.com/synodlang/compiler/internal/driver"
	"github.com/synodlang/compiler/internal/load"
	"github.com/synodlang/compiler/internal/pipeline"
	"github.com/synodlang/compiler/internal/source"
)

// formatWith formats f with the fixes the driver found for the file at path,
// or with the M1 formatter when the file's package did not load.
func formatWith(res driver.Result, path string, f *source.File) ([]byte, *source.Diagnostic) {
	fsys := load.OSFS{}
	display := ""
	if abs, err := fsys.Abs(path); err == nil {
		display = fsys.Display(abs)
	}
	return pipeline.FormatFile(res, display, f, formatFn)
}

func runFmt(args []string, stdout, stderr io.Writer) int {
	paths, flags, values, err := parseFlags(args, []string{"check"}, []string{"style", "color"})
	if err != nil {
		fmt.Fprintf(stderr, "synod fmt: %v\n", err)
		return exitUsage
	}
	sf, err := styleOf(values)
	if err != nil {
		fmt.Fprintf(stderr, "synod fmt: %v\n", err)
		return exitUsage
	}
	files, targets, err := collect(paths)
	if err != nil {
		fmt.Fprintf(stderr, "synod fmt: %v\n", err)
		return exitUsage
	}
	check := flags["check"]
	// The fixes that need declarations come from the driver. A file whose
	// package does not load has none and gets the M1 behaviour.
	res := driver.Check(driver.Options{}, targets)
	ioErr, diags, changed := false, false, false
	for _, path := range files {
		src, err := os.ReadFile(path)
		if err != nil {
			fmt.Fprintf(stderr, "synod fmt: %v\n", err)
			ioErr = true
			continue
		}
		f := source.NewFile(path, src)
		out, d := formatWith(res, path, f)
		if d != nil {
			// Diagnostics go to stderr, so the style is detected there.
			src := &sources{known: map[string]*source.File{f.Path: f}}
			if werr := writeDiags(stderr, []source.Diagnostic{*d}, sf, src); werr != nil {
				fmt.Fprintf(stderr, "synod fmt: %v\n", werr)
			}
			diags = true
			continue
		}
		if bytes.Equal(out, src) {
			continue
		}
		changed = true
		if check {
			fmt.Fprintln(stdout, path)
			continue
		}
		if err := writeFileAtomic(path, out); err != nil {
			fmt.Fprintf(stderr, "synod fmt: %v\n", err)
			ioErr = true
		}
	}
	switch {
	case ioErr:
		return exitUsage
	case diags || (check && changed):
		return exitDiags
	}
	return exitOK
}
