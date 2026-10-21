package main

import (
	"fmt"
	"io"

	"github.com/synodlang/compiler/internal/driver"
	"github.com/synodlang/compiler/internal/pipeline"
)

func runCheck(args []string, stdout, stderr io.Writer) int {
	paths, flags, values, err := parseFlags(args, []string{"json", "tokens"}, []string{"style", "color"})
	if err != nil {
		fmt.Fprintf(stderr, "synod check: %v\n", err)
		return exitUsage
	}
	sf, err := styleOf(values)
	if err != nil {
		fmt.Fprintf(stderr, "synod check: %v\n", err)
		return exitUsage
	}
	if flags["tokens"] && !flags["json"] {
		fmt.Fprintln(stderr, "synod check: --tokens needs --json")
		return exitUsage
	}
	_, targets, err := collect(paths)
	if err != nil {
		fmt.Fprintf(stderr, "synod check: %v\n", err)
		return exitUsage
	}
	res := driver.Check(driver.Options{Tokens: flags["tokens"]}, targets)
	diags := pipeline.UserDiags(res)
	if flags["json"] {
		if werr := pipeline.WriteCheckJSON(stdout, diags, res, flags["tokens"]); werr != nil {
			fmt.Fprintf(stderr, "synod check: %v\n", werr)
			return exitUsage
		}
	} else {
		if werr := writeDiags(stdout, diags, sf, &sources{known: res.Sources}); werr != nil {
			fmt.Fprintf(stderr, "synod check: %v\n", werr)
			return exitUsage
		}
	}
	if len(diags) > 0 {
		return exitDiags
	}
	return exitOK
}
