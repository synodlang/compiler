package trace

import (
	"sort"
	"strconv"

	"github.com/synodlang/compiler/internal/ir"
)

// Qualifiers returns the function that names the packages of p in traces and
// summaries. A package is named by its Name; when several packages of the
// program share a name, they are taken in import-path order, the first keeps
// the name and the others get "<name>2", "<name>3", … (skipping any name
// that another package already has). Packages not in p keep their Name.
func Qualifiers(p *ir.Program) func(*ir.Package) string {
	byName := map[string][]*ir.Package{}
	used := map[string]bool{}
	var names []string
	for _, pkg := range p.Packages {
		if _, ok := byName[pkg.Name]; !ok {
			names = append(names, pkg.Name)
		}
		byName[pkg.Name] = append(byName[pkg.Name], pkg)
		used[pkg.Name] = true
	}
	sort.Strings(names)
	assigned := map[*ir.Package]string{}
	for _, name := range names {
		group := byName[name]
		sort.SliceStable(group, func(i, j int) bool { return group[i].Path < group[j].Path })
		assigned[group[0]] = name
		next := 2
		for _, pkg := range group[1:] {
			for {
				q := name + strconv.Itoa(next)
				next++
				if !used[q] {
					used[q] = true
					assigned[pkg] = q
					break
				}
			}
		}
	}
	return func(pkg *ir.Package) string {
		if q, ok := assigned[pkg]; ok {
			return q
		}
		return pkg.Name
	}
}

// qualOf returns o.Qual, or the plain package name when it is nil.
func (o Options) qualOf() func(*ir.Package) string {
	if o.Qual != nil {
		return o.Qual
	}
	return func(p *ir.Package) string { return p.Name }
}
