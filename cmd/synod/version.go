package main

import (
	"fmt"
	"io"
)

// version is the compiler version, set with -ldflags "-X main.version=...".
var version = "dev"

func runVersion(args []string, stdout, stderr io.Writer) int {
	if len(args) > 0 {
		fmt.Fprintln(stderr, "synod: version takes no arguments")
		return exitUsage
	}
	fmt.Fprintf(stdout, "language 0.2\ncompiler %s\nformats synod-check/1 synod-trace/1 synod-scene/1\n", version)
	return exitOK
}
