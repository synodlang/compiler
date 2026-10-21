package main

import (
	"fmt"
	"io"
	"runtime/debug"

	"github.com/synodlang/compiler/internal/pipeline"
)

// version is the compiler version, set with -ldflags "-X main.version=...".
var version = "dev"

// compilerVersion renders the compiler description for `synod version`.
func compilerVersion(injected string, info *debug.BuildInfo) string {
	return pipeline.CompilerVersion(injected, info)
}

func runVersion(args []string, stdout, stderr io.Writer) int {
	if len(args) > 0 {
		fmt.Fprintln(stderr, "synod: version takes no arguments")
		return exitUsage
	}
	info, _ := debug.ReadBuildInfo()
	fmt.Fprint(stdout, pipeline.VersionText(compilerVersion(version, info)))
	return exitOK
}
