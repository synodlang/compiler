package main

import (
	"fmt"
	"io"
	"os"

	"github.com/synodlang/compiler/internal/pipeline"
	"github.com/synodlang/compiler/internal/source"
)

// Tests replace these to simulate a terminal and an environment.
var (
	isTerminal = fileIsTerminal
	getenv     = os.Getenv
	// enableVT turns on ANSI escape processing for the console behind w and
	// reports whether it is on. It is a no-op that reports true on a
	// terminal on every system but Windows.
	enableVT = enableVTReal
)

// fileIsTerminal reports whether w is a character device: a terminal, a
// Windows console, or the null device (a harmless false positive). Pipes and
// regular files are not. MSYS and mintty pipes count as pipes, so they get
// the short form.
func fileIsTerminal(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok || f == nil {
		return false
	}
	st, err := f.Stat()
	if err != nil {
		return false
	}
	return st.Mode()&os.ModeCharDevice != 0
}

// styleFlags are the values of --style and --color, "" when not given.
type styleFlags struct {
	style, color string
}

// check validates the flag values.
func (s styleFlags) check() error {
	switch s.style {
	case "", "rich", "short":
	default:
		return fmt.Errorf("--style needs rich or short, not %q", s.style)
	}
	switch s.color {
	case "", "auto", "always", "never":
	default:
		return fmt.Errorf("--color needs auto, always or never, not %q", s.color)
	}
	return nil
}

// resolve decides how diagnostics are written to w. Rich is the default when
// w is a terminal. Color needs rich; auto also needs NO_COLOR unset or empty
// and TERM other than "dumb", and always and never beat the environment.
func (s styleFlags) resolve(w io.Writer) (rich, color bool) {
	tty := isTerminal(w)
	switch s.style {
	case "rich":
		rich = true
	case "short":
		rich = false
	default:
		rich = tty
	}
	if !rich {
		return false, false
	}
	switch s.color {
	case "always":
		color = true
	case "never":
		color = false
	default:
		color = tty && getenv("NO_COLOR") == "" && getenv("TERM") != "dumb"
	}
	if color && tty && !enableVT(w) {
		color = false
	}
	return rich, color
}

// sources looks files up for the renderer: first the sources of the graph,
// then the file system (cached). A nil result means no snippet.
type sources struct {
	known map[string]*source.File
	cache map[string]*source.File
}

func (s *sources) get(file string) *source.File {
	if f := s.known[file]; f != nil {
		return f
	}
	if f, ok := s.cache[file]; ok {
		return f
	}
	if s.cache == nil {
		s.cache = map[string]*source.File{}
	}
	var f *source.File
	if file != "" {
		if b, err := os.ReadFile(file); err == nil {
			f = source.NewFile(file, b)
		}
	}
	s.cache[file] = f
	return f
}

// writeDiags writes ds to w, in the one-line form or the rich form.
func writeDiags(w io.Writer, ds []source.Diagnostic, sf styleFlags, src *sources) error {
	rich, color := sf.resolve(w)
	return pipeline.WriteDiags(w, ds, pipeline.Style{Rich: rich, Color: color}, src.get)
}
