package main

import (
	"fmt"
	"io"
	"strings"

	"github.com/synodlang/compiler/internal/get"
	"github.com/synodlang/compiler/internal/load"
)

// runGet implements `synod get [module@version]` (decision 95).
func runGet(args []string, stdout, stderr io.Writer) int {
	paths, _, _, err := parseFlags(args, nil, nil)
	if err != nil {
		fmt.Fprintf(stderr, "synod get: %v\n", err)
		return exitUsage
	}
	if len(paths) > 1 {
		fmt.Fprintln(stderr, "synod get: takes at most one argument, module@version")
		return exitUsage
	}
	cfg := get.Config{Dir: "."}
	if len(paths) == 1 {
		m, v, ok := strings.Cut(paths[0], "@")
		if !ok || !load.ValidModulePath(m) || !load.ValidVersion(v) {
			fmt.Fprintln(stderr, "synod get: write <module>@vX.Y.Z")
			return exitUsage
		}
		cfg.Module, cfg.Version = m, v
	}
	results, err := get.Run(cfg)
	if err != nil {
		fmt.Fprintf(stderr, "synod get: %v\n", err)
		return exitUsage
	}
	for _, r := range results {
		fmt.Fprintln(stdout, r)
	}
	return exitOK
}
