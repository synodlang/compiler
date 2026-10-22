package doc

import (
	"github.com/synodlang/compiler/internal/driver"
	"github.com/synodlang/compiler/internal/resolve"
)

// BuildScope lists what file f of package rp can write (decision 92): the
// package's own names, all of them, with bare heads, then each import's
// exported names with heads spelled by the qualifier or alias that f uses,
// in import order. An import that failed to load is left out.
func BuildScope(res driver.Result, rp *resolve.Package, f *resolve.File) (*Scope, []*Package) {
	sc := &Scope{File: f.Path, Package: rp.Name, Imports: []ScopeImport{}}
	own := rp.Path
	qualOf := map[string]string{}
	for _, imp := range f.Imports {
		if imp.Pkg != nil {
			qualOf[imp.Pkg.Path] = imp.Qual
		}
	}
	qual := func(path string) string { return qualOf[path] }
	pkgs := []*Package{Build(res, rp, View{All: true, From: own, Qual: qual})}
	for _, imp := range f.Imports {
		if imp.Pkg == nil {
			continue
		}
		sc.Imports = append(sc.Imports, ScopeImport{Path: imp.Imp.Path, Qualifier: imp.Qual})
		pkgs = append(pkgs, Build(res, imp.Pkg, View{Qualifier: imp.Qual, From: own, Qual: qual}))
	}
	return sc, pkgs
}
