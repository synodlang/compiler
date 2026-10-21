// Package driver runs the compiler pipeline: load the packages, resolve
// each one, check that its files are canonical (with the fixes the resolver
// found), and, once the checker exists, type-check them.
package driver

import (
	"github.com/synodlang/compiler/internal/check"
	"github.com/synodlang/compiler/internal/format"
	"github.com/synodlang/compiler/internal/ir"
	"github.com/synodlang/compiler/internal/load"
	"github.com/synodlang/compiler/internal/resolve"
	"github.com/synodlang/compiler/internal/source"
	"github.com/synodlang/compiler/internal/stdlib"
)

// Options configures Check.
type Options struct {
	// Load tells the loader where to read from. A nil Std is replaced by the
	// embedded std.
	Load load.Config
	// Tokens asks for the token lists of the files (Result.Tokens).
	Tokens bool
}

// Result is the outcome of Check.
type Result struct {
	// Diags are all diagnostics, std files included, sorted and free of
	// duplicates.
	Diags []source.Diagnostic
	// Graph is the loaded graph.
	Graph *load.Graph
	// Packages are the resolved packages in graph order (dependencies
	// first); a broken package is absent.
	Packages []*resolve.Package
	// ByPath indexes Packages by load path.
	ByPath map[string]*resolve.Package
	// Fixes holds the fixes per file, by the path shown in diagnostics.
	Fixes map[string][]format.Fix
	// Infos holds what the checker learned, by import path.
	Infos map[string]*check.Info
	// Programs holds the lowered IR, by the import path of the entry package
	// (a main package has "#main" appended). It is nil when there are
	// diagnostics.
	Programs map[string]*ir.Program
	// Tokens holds the token lists per file, by the path shown in
	// diagnostics; only with Options.Tokens.
	Tokens map[string][]resolve.Token
}

// Check loads, resolves and checks the packages of targets (slash paths of
// folders or files).
func Check(opts Options, targets []string) Result {
	cfg := opts.Load
	if cfg.Std == nil {
		cfg.Std = stdlib.Root()
	}
	g, diags := load.Load(cfg, targets)
	res := Result{Graph: g, ByPath: map[string]*resolve.Package{}, Fixes: map[string][]format.Fix{}}
	if opts.Tokens {
		res.Tokens = map[string][]resolve.Token{}
	}
	for _, lp := range g.Packages {
		if lp.Broken {
			continue
		}
		deps := map[string]*resolve.Package{}
		for _, d := range lp.Deps {
			if rp := res.ByPath[d.Path]; rp != nil {
				deps[d.Path] = rp
			}
		}
		rp, ds := resolve.Resolve(lp, deps)
		diags = append(diags, ds...)
		res.Packages = append(res.Packages, rp)
		res.ByPath[lp.Path] = rp
		for _, f := range rp.Files {
			fx := rp.Fixes[f.Load]
			res.Fixes[f.Path] = fx
			if d := format.CheckWith(f.Load.Src, fx); d != nil {
				diags = append(diags, *d)
			}
			if opts.Tokens {
				res.Tokens[f.Path] = rp.Tokens[f.Load]
			}
		}
	}
	diags = append(diags, runCheck(&res, diags)...)
	res.Diags = tidy(diags)
	return res
}

// runCheck type-checks the resolved packages, dependencies first, then
// checks contexts across the whole graph, and lowers the program to the IR
// when nothing has been reported. prior are the diagnostics of the earlier
// stages.
func runCheck(res *Result, prior []source.Diagnostic) []source.Diagnostic {
	var out []source.Diagnostic
	res.Infos = map[string]*check.Info{}
	for _, rp := range res.Packages {
		info, ds := check.Package(rp, res.Infos)
		res.Infos[rp.Path] = info
		check.AdjustHints(rp, prior)
		out = append(out, ds...)
	}
	out = append(out, check.Contexts(res.Packages, res.Infos)...)
	if len(prior) == 0 && len(out) == 0 {
		res.Programs = check.Lower(res.Packages, res.Infos)
	}
	return out
}

// tidy sorts diagnostics, drops duplicates, and keeps at most one
// non-canonical diagnostic per file: the first.
func tidy(ds []source.Diagnostic) []source.Diagnostic {
	source.Sort(ds)
	out := ds[:0]
	nc := map[string]bool{}
	for i, d := range ds {
		if i > 0 && d.Key() == ds[i-1].Key() {
			continue
		}
		if d.Code == source.NonCanonical {
			if nc[d.File] {
				continue
			}
			nc[d.File] = true
		}
		out = append(out, d)
	}
	return out
}
