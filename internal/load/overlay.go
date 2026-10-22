package load

import (
	"io/fs"
	"path"
)

// OverlayFS reads Base, except that the files named in Files hold the given
// content instead (decision 80). Keys are absolute slash paths as Base's Abs
// would give them. Nothing is written, listings are unchanged, and
// Display and Abs are Base's own.
type OverlayFS struct {
	Base  FS
	Files map[string][]byte
}

// ReadFile returns the replacement of name if there is one.
func (o OverlayFS) ReadFile(name string) ([]byte, error) {
	if b, ok := o.Files[o.abs(name)]; ok {
		return b, nil
	}
	return o.Base.ReadFile(name)
}

// ReadDir lists name in Base.
func (o OverlayFS) ReadDir(name string) ([]fs.DirEntry, error) { return o.Base.ReadDir(name) }

// Abs returns Base's Abs of name, or the cleaned name.
func (o OverlayFS) Abs(name string) (string, error) {
	if a, ok := o.Base.(interface{ Abs(string) (string, error) }); ok {
		return a.Abs(name)
	}
	return path.Clean(name), nil
}

// Display is Base's Display, or the name as it is.
func (o OverlayFS) Display(name string) string {
	if d, ok := o.Base.(interface{ Display(string) string }); ok {
		return d.Display(name)
	}
	return name
}

func (o OverlayFS) abs(name string) string {
	a, err := o.Abs(name)
	if err != nil {
		return path.Clean(name)
	}
	return a
}
