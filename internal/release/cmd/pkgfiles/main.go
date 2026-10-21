// Command pkgfiles writes the Homebrew formula and Scoop manifest for a release.
//
//	go run ./internal/release/cmd/pkgfiles -tag v0.1.0 -sums dist/SHA256SUMS \
//	    -formula tap/Formula/synod.rb -scoop bucket/synod.json
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/synodlang/compiler/internal/release"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "pkgfiles:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	fs := flag.NewFlagSet("pkgfiles", flag.ContinueOnError)
	tag := fs.String("tag", "", "release tag, e.g. v0.1.0")
	sumsPath := fs.String("sums", "", "path to SHA256SUMS")
	formula := fs.String("formula", "", "output path for the Homebrew formula")
	scoop := fs.String("scoop", "", "output path for the Scoop manifest")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *tag == "" || *sumsPath == "" || (*formula == "" && *scoop == "") {
		return fmt.Errorf("need -tag, -sums and at least one of -formula, -scoop")
	}
	f, err := os.Open(*sumsPath)
	if err != nil {
		return err
	}
	defer f.Close()
	sums, err := release.ParseSums(f)
	if err != nil {
		return err
	}
	if *formula != "" {
		s, err := release.Formula(*tag, sums)
		if err != nil {
			return err
		}
		if err := write(*formula, s); err != nil {
			return err
		}
	}
	if *scoop != "" {
		s, err := release.Manifest(*tag, sums)
		if err != nil {
			return err
		}
		if err := write(*scoop, s); err != nil {
			return err
		}
	}
	return nil
}

func write(path, s string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(s), 0o644)
}
