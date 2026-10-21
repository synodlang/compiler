package main

import (
	"bytes"
	"fmt"
	"io"
	"os"

	"github.com/synodlang/compiler/internal/source"
)

func runFmt(args []string, stdout, stderr io.Writer) int {
	paths, flags, err := parseArgs(args, "check")
	if err != nil {
		fmt.Fprintf(stderr, "synod fmt: %v\n", err)
		return exitUsage
	}
	files, err := collect(paths)
	if err != nil {
		fmt.Fprintf(stderr, "synod fmt: %v\n", err)
		return exitUsage
	}
	check := flags["check"]
	ioErr, diags, changed := false, false, false
	for _, path := range files {
		src, err := os.ReadFile(path)
		if err != nil {
			fmt.Fprintf(stderr, "synod fmt: %v\n", err)
			ioErr = true
			continue
		}
		out, d := formatFn(source.NewFile(path, src))
		if d != nil {
			fmt.Fprintln(stderr, d.String())
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
