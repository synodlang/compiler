// Package stdlib holds the Synod standard library as the compiler sees it:
// the embedded sources of github.com/synodlang/std, the table of native
// phrases with their typing rules, and the contexts of the phrases that std
// writes in Synod.
//
// The sources under std are a byte-for-byte copy of the std repository
// (TestStdDrift keeps them equal). The catalog they implement is
// dsl/std/README.md; TestCatalog checks that every pattern of the catalog has
// exactly one define here.
package stdlib

import (
	"embed"
	"io/fs"
)

// FS holds the vendored std repository: synod.mod at the root and one folder
// per package, such as std/net/net.synod. Paths are relative to the
// repository, so the root of FS is the directory "std"; use Root for the
// repository itself.
//
//go:embed std
var FS embed.FS

// Root returns the std repository: synod.mod at the root and one folder per
// package below it.
func Root() fs.FS {
	sub, err := fs.Sub(FS, "std")
	if err != nil {
		panic("stdlib: embedded std is missing: " + err.Error()) // unreachable: go:embed std guarantees the directory
	}
	return sub
}
