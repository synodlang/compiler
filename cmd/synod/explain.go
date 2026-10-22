package main

import (
	"fmt"
	"io"

	"github.com/synodlang/compiler/internal/explain"
	"github.com/synodlang/compiler/internal/pipeline"
)

// runExplain implements `synod explain [code] [--json] [--catalog]
// [--style rich|short] [--color auto|always|never]` (implementation/05-cli.md,
// decision 93). The diagnostic shown for an example is made when the command
// runs, by checking or running the example in memory.
func runExplain(args []string, stdout, stderr io.Writer) int {
	paths, flags, values, err := parseFlags(args, []string{"json", "catalog"}, []string{"style", "color"})
	if err != nil {
		fmt.Fprintf(stderr, "synod explain: %v\n", err)
		return exitUsage
	}
	sf, err := styleOf(values)
	if err != nil {
		fmt.Fprintf(stderr, "synod explain: %v\n", err)
		return exitUsage
	}
	if len(paths) > 1 {
		fmt.Fprintf(stderr, "synod explain: takes at most one code, got %d\n", len(paths))
		return exitUsage
	}
	if flags["json"] && flags["catalog"] {
		fmt.Fprintln(stderr, "synod explain: --json and --catalog cannot be combined")
		return exitUsage
	}
	if flags["catalog"] && len(paths) > 0 {
		fmt.Fprintln(stderr, "synod explain: --catalog takes no code")
		return exitUsage
	}

	entries := explain.Entries()
	if len(paths) == 1 {
		e, ok := explain.Lookup(paths[0])
		if !ok {
			fmt.Fprintf(stderr, "synod explain: unknown code %q\n", paths[0])
			if s := explain.Suggest(paths[0]); s != "" {
				fmt.Fprintf(stderr, "  hint: did you mean %q?\n", s)
			}
			return exitUsage
		}
		entries = []*explain.Entry{e}
	}

	fail := func(err error) int {
		fmt.Fprintf(stderr, "synod explain: %v\n", err)
		return exitUsage
	}
	switch {
	case flags["catalog"]:
		if err := explain.WriteCatalog(stdout, entries); err != nil {
			return fail(err)
		}
	case flags["json"]:
		docs, err := render(entries)
		if err != nil {
			return fail(err)
		}
		if err := explain.WriteJSON(stdout, docs); err != nil {
			return fail(err)
		}
	case len(paths) == 0:
		if err := explain.List(stdout, entries); err != nil {
			return fail(err)
		}
	default:
		docs, err := render(entries)
		if err != nil {
			return fail(err)
		}
		rich, color := sf.resolve(stdout)
		if err := explain.Text(stdout, docs[0], pipeline.Style{Rich: rich, Color: color}); err != nil {
			return fail(err)
		}
	}
	return exitOK
}

// render makes the diagnostic of every entry's example.
func render(entries []*explain.Entry) ([]*explain.Doc, error) {
	docs := make([]*explain.Doc, 0, len(entries))
	for _, e := range entries {
		d, err := explain.Render(e)
		if err != nil {
			return nil, fmt.Errorf("internal error: %s: %v", e.Code, err)
		}
		docs = append(docs, d)
	}
	return docs, nil
}
