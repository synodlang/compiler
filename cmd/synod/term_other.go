//go:build !windows

package main

import (
	"io"
	"os"
)

// enableVTReal: terminals other than the Windows console understand ANSI
// escapes. It reports whether w is a terminal.
func enableVTReal(w io.Writer) bool {
	f, ok := w.(*os.File)
	return ok && f != nil && fileIsTerminal(f)
}
