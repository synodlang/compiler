# Changelog

All notable changes to the `synod` command. Each released version's
section is the text of its GitHub release page.

## Unreleased

`synod run` simulates a program deterministically and writes its trace.

### Added

- `synod run <folder>` checks the folder's main file and runs it in the
  deterministic simulator: simulated time, nodes with crash and restart,
  a network with latency, loss, partitions and targeted drops, per-node
  disk, and the `always` invariants checked after every step. It writes
  `out/trace.jsonl` (`synod-trace/1`) and prints one status line. The same
  program and seed give a byte-identical trace on every platform.
- `--seed n` (or `--seed=n`, a whole number from 0 to 2^53) replaces the
  seed of the run; it wins over `sim.Seed`. Without either the seed is 1.
- `--trace file` chooses the trace file (parents are created); `--trace -`
  writes the trace to stdout and the status line to stderr.
- `--summary` prints a compact text summary of the run (steps that
  recorded facts, invariant results, final state of every node) instead of
  the status line, for agents.
- Every native phrase of the std catalog works: `sim`, `net`, `time`,
  `random`, `lists`, `maps`, `math`, `text`, `disk`. The `look` phrases and
  the observers wait for a later release; `--scene`, `--pace`, `--mode`,
  `--watch` and `--json-events` are recognized and exit 2 with
  `not implemented yet`.
- Run-time errors, reported as the last `sim.Failed` line of the trace,
  `failed at <time>: <pos>: error[<code>]: <message>` and exit 1:
  `missing-key`, `index-range`, `division-by-zero`, `nobody`, `disk-type`,
  `time-limit` (10 minutes of simulated time), `bad-argument`,
  `number-range` and `run-limit` (steps at one instant, call depth,
  collection size).
- Exit codes of `synod run`: 0 for a run that ended (also when an `always`
  broke, which is a result, not an error), 1 for diagnostics or a failed
  run, 2 for usage and I/O errors.

### Changed

- `sim.Seed` and `math.Hash` are specified completely in the std catalog
  (domain and precedence of the seed; the exact FNV-1a based hash).
- Demo seeds that did not tell their story under the final simulator
  semantics were changed (gossip 15, paxos 105, twopc 1, cassandra 4) and
  the ring demo places other keys; see "Notes for implementers" in the
  examples.

## v0.2.0 — 2026-10-21

`synod check` now understands whole programs: it loads modules, finds the
phrase every sentence calls, and checks types and contexts, reporting every
compile-time error of specification v0.2.1. Running programs
(`synod run`) comes in the next release.

### Added

- Language types and their acceptance rules (`list of T`, `map of K to V`,
  node types `Node` and `p.Node`).
- The standard library, embedded in the binary: every phrase of the std
  catalog, with native declarations and the phrases written in Synod.
- Module loader: `synod.mod`, package graph, main files next to their
  library, `std/` imports, remote modules from the
  module's `vendor/` folder, import cycles.
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

- `synod fmt` applies the fixes above when the package loads; a file in a
  package that does not load is formatted as before.
- Diagnostic paths are shown relative to the working directory when inside it.
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
