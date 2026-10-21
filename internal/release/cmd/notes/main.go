// Command notes prints the CHANGELOG.md section of a release, for use as the
// GitHub release notes.
//
//	go run ./internal/release/cmd/notes -tag v0.1.0 -changelog CHANGELOG.md
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/synodlang/compiler/internal/release"
)

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "notes:", err)
		os.Exit(1)
	}
}

func run(args []string, out *os.File) error {
	fs := flag.NewFlagSet("notes", flag.ContinueOnError)
	tag := fs.String("tag", "", "release tag, e.g. v0.1.0")
	path := fs.String("changelog", "CHANGELOG.md", "path to the changelog")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *tag == "" {
		return fmt.Errorf("need -tag")
	}
	b, err := os.ReadFile(*path)
	if err != nil {
		return err
	}
	s, err := release.Notes(string(b), *tag)
	if err != nil {
		return err
	}
	_, err = fmt.Fprint(out, s)
	return err
}
