package pipeline

import (
	"github.com/synodlang/compiler/internal/driver"
	"github.com/synodlang/compiler/internal/format"
	"github.com/synodlang/compiler/internal/source"
)

// FormatFile formats f with the fixes the driver found for the file shown as
// display, or with fallback (the formatter without fixes) when the file's
// package did not load and so has none.
func FormatFile(res driver.Result, display string, f *source.File, fallback func(*source.File) ([]byte, *source.Diagnostic)) ([]byte, *source.Diagnostic) {
	if fixes, ok := res.Fixes[display]; ok && len(fixes) > 0 {
		return format.FormatWith(f, fixes)
	}
	return fallback(f)
}
