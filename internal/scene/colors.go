package scene

import "github.com/synodlang/compiler/internal/ir"

// themePath is the import path of std/theme, whose colors keep their bare
// name in the scene ("Accent").
const themePath = "std/theme"

// colorName names the color constant hex, which is being evaluated inside the
// phrase p (nil in a handler body). A zero-hole phrase names its color: the
// bare phrase name for std/theme ("Accent"), otherwise
// "<trace qualifier>.<Name>" ("raftlook.Hot"). A color written anywhere else
// is named by its hex text. The first hex seen for a name is kept; emit asks
// for it by name (colorHex).
func (b *builder) colorName(hex string) string {
	name := hex
	if p := b.cur; p != nil && p.Kind == ir.ProcPhrase && p.Pkg != nil &&
		p.Receiver == nil && len(p.Params) == 0 && len(p.BlockParams) == 0 {
		name = p.Name
		if p.Pkg.Path != themePath {
			name = b.qual(p.Pkg) + "." + p.Name
		}
	}
	if _, ok := b.hexes[name]; !ok {
		b.hexes[name] = hex
	}
	return name
}

// colorHex gives the hex value of a color name used by the scene.
func (b *builder) colorHex(name string) (string, bool) {
	h, ok := b.hexes[name]
	return h, ok
}
