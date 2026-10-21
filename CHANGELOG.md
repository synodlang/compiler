# Changelog

All notable changes to the `synod` command. Each released version's
section is the text of its GitHub release page.

## Unreleased

### Added

- Language types and their acceptance rules (`list of T`, `map of K to V`,
  node types `Node` and `p.Node`).
- The standard library, embedded in the binary: every phrase of the std
  catalog, with native declarations and the phrases written in Synod.
- Module loader: `synod.mod`, package graph, main files next to their
  library, `std/` imports, remote modules from the cache
  (`$SYNOD_CACHE` or `~/.synod/cache`), import cycles.
- Fixes that need the package loaded, for `synod fmt`: record settings
  order, name case, and a dropped own-package qualifier.
- `synod check` now checks whole programs, not just their form. It finds
  the phrase every sentence calls, checks the types of values (arguments,
  operands, list and map items, node types such as `raft.Node`, and
  results), infers the type of `{}` from how it is used, and reports
  every error code of the specification: `type-mismatch`, `ambiguous-empty`,
  `missing-result`, `value-as-statement`, `action-as-value`, `context`
  (what may run in node, main, check and look code) and `foreign-state`
  (node code that touches another node's state). Every example and every
  std package checks clean.

### Changed

- CI and release workflows use the Node 24 versions of GitHub Actions.

## v0.1.0 — 2026-10-21

The first release: the front end of the Synod compiler. It reads, checks
the form of, and formats Synod source for specification v0.2.0. Running
programs (`synod run`) comes in a later release.

### Added

- `synod fmt [paths…]` rewrites `.synod` files into canonical form;
  `--check` lists the files that would change and exits 1.
- `synod check [paths…]` reports syntax, indentation, header and
  canonical-form errors, with a hint showing the canonical line;
  `--json` prints the `synod-check/1` format.
- `synod version` prints the language, compiler and format versions; the
  compiler line shows the release tag and the commit it was built from.
- Lexer and parser for the whole structure grammar: indentation blocks,
  comments, continuation lines (`else`, `otherwise`), holes, typed
  collections, members, import paths.
- Canonical formatting: numbers, durations and colors respelled; single-item
  parentheses removed; `not (…)` rewritten to the direct operator; imports
  hoisted and sorted; blank lines and comments laid out.
- Binaries for macOS (arm64, amd64, universal), Linux (amd64, arm64) and
  Windows (amd64, arm64); install with Homebrew
  (`brew install synodlang/tap/synod`), Scoop, `install.sh` or
  `go install`. See https://synodlang.github.io/install/.

### Known limitations

- Binaries are not code-signed: macOS and Windows warn when they are
  downloaded with a browser. Package managers and `install.sh` avoid this.
- Phrase resolution, types and contexts are not checked yet.
