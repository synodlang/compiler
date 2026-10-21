// Package pipeline assembles the output of check, run and fmt: the
// documents, status lines and diagnostics that the command line prints and
// the WebAssembly API returns. Both call this package, so they cannot drift
// apart. It does no file I/O and has no flags.
package pipeline

import (
	"fmt"
	"io"

	"github.com/synodlang/compiler/internal/render"
	"github.com/synodlang/compiler/internal/source"
)

// Style says how diagnostics are written: the rich form with snippets or the
// one-line form, with or without ANSI color. Color needs Rich.
type Style struct {
	Rich, Color bool
}

// WriteDiags writes ds to w, in the one-line form or the rich form. lookup
// finds the source of a file for the snippets; nil or a nil result means no
// snippet.
func WriteDiags(w io.Writer, ds []source.Diagnostic, st Style, lookup func(string) *source.File) error {
	if !st.Rich {
		for _, d := range ds {
			if _, err := fmt.Fprintln(w, d.String()); err != nil {
				return err
			}
		}
		return nil
	}
	return render.All(w, ds, render.Options{Color: st.Color, Source: lookup})
}
