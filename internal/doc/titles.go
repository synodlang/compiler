package doc

import "sort"

// titles are the titles of the std packages, as the headings of
// std/README.md write them ("## std/net — the simulated network"). A package
// that is not here has none. TestTitles keeps the table equal to the README.
var titles = map[string]string{
	"std/sim":    "nodes, runs, faults",
	"std/net":    "the simulated network",
	"std/random": "deterministic randomness",
	"std/disk":   "storage that survives crashes",
	"std/lesson": "Predict mode and captions",
	"std/theme":  "named colors",
	"std/look":   "shapes, motion, layout (look context only)",
}

// Title returns the title of a std package by import path, or "".
func Title(path string) string { return titles[path] }

// TitledPaths returns the import paths that have a title, sorted.
func TitledPaths() []string {
	out := make([]string, 0, len(titles))
	for p := range titles {
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}
