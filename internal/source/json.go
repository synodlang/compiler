package source

import (
	"encoding/json"
	"io"
)

// checkFormat is the value of the "format" key of check.json.
const checkFormat = "synod-check/1"

type checkJSON struct {
	Format      string       `json:"format"`
	OK          bool         `json:"ok"`
	Diagnostics []Diagnostic `json:"diagnostics"`
}

// WriteJSON writes ds as check.json: one compact line ending in a newline.
// An empty list is written as [], never null, and HTML characters are not
// escaped.
func WriteJSON(w io.Writer, ds []Diagnostic) error {
	if ds == nil {
		ds = []Diagnostic{}
	}
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	return enc.Encode(checkJSON{Format: checkFormat, OK: len(ds) == 0, Diagnostics: ds})
}
