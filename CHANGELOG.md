# Changelog

All notable changes to the `synod` command. Each released version's
section is the text of its GitHub release page.

## Unreleased

Implements Synod 0.3. `synod version` prints `language 0.3`, `synod new` writes `synod 0.3`, and trace headers say `"synod":"0.3"`. A `synod.mod` that says an older version (`synod 0.2`) is still accepted, and the version line does not gate features; a newer one (`synod 0.4`) is an error.

### Changed

- The embedded `look/basic` follows `std`: message tags show only mid-flight. Scene ids and goldens change; traces are unchanged.
- `look.Flight` moves the flown shape from the edge of `a` to the edge of `b`, 8 clear of each box (the `Place` size model, group extents, proportional clamp when the cards are close, halfway between the clipped ends when dropped; straight up above the shape when both are at one position). Scenes with flights change; traces are unchanged. The embedded std follows `std`.
- The `ring` layout is wider and sits higher: radii 380 and 170 (growth up to 1.4 in height), and the ring, row and grid layouts centre on (520, 315), 10 above the scene centre, since state rows hang below the cards. An even number of cards turns half a slot counter-clockwise, so `n1` is at the left or top left. Scenes change; traces are unchanged.
- `sim.StateRows` gives each row at most 24 characters (`StateRowMax`): a longer one keeps its first 23 and ends in `…`. Look-only, so traces keep full values. The embedded std and the vendored examples follow `std` and `examples`.
- The fit test also checks a mid-scene key, that every state row has at most 24 characters, that rows clear other cards and icons by 8, and that no two labels overlap.
- The embedded `look/basic` writes the lesson note at the top of the canvas (`y` 40). The fit test measures notes and reasons against cards, rows, heads, icons, counters and each other, with no exclusion, and counts a counter as a box.
- The embedded and vendored `paxos` keeps its promise, accepted proposal and round on `std/disk`; only the paxos trace gains `disk.Written` events, every other trace is byte-identical.
- Scenes use a 1040 by 650 canvas with its centre at (520, 325), `look.Orbit` has radius 200, and the `ring` layout is an ellipse (radii 300 and 150, turned half a slot for an even number of cards, growing up to 1.4 with the number of cards). Default-theme scenes change once; traces are unchanged. The embedded std and the vendored examples follow `std` and `examples`.

### Fixed

- `synod new demo` is refused like `main`, `time` and `sim`, with or without `--lib`: the package `demo` collided with `demo/demo.synod`, the main program, which overwrote the library.
- `synod check` (and `fmt`, `doc`) over a folder that holds two module folders with the same `module` path, such as `counter/synod.mod` and `proj/counter/synod.mod`, panicked while lowering because both modules' packages shared one import path. It now reports `error[duplicate-name]: module "counter" is declared by two folders, counter and proj/counter` at the second `synod.mod`, with the hint to check one module folder at a time, and exits 1.

## v0.2.7 — 2026-10-28

Implements Synod 0.2. The documentation is now checked against the compiler, and `synod get` downloads are byte-exact.

### Added

- `internal/forms` and `internal/forms/cmd/doccheck` (not linked into `synod` or `synod.wasm`): the catalog of the forms of `spec/02`, the types of `spec/04`, the contexts of `spec/05` and the std packages, checked against the spec text, and `doccheck`, which checks the tutorial and the Library examples of `dsl` (decisions 104 and 105): coverage, order of introduction, `synod new` and `examples/raft` copies, golden summaries, "why" programs and one runnable example per std phrase. A `docs` job in CI runs it once the pinned `dsl` has a tutorial.

### Changed

- `synod explain --json`: `example.files` holds every file the example is checked or run with, the implied `synod.mod` included (decision 111). The text form, `--catalog` and the MCP tool are unchanged.

### Fixed

- `synod get` checks out modules byte for byte (no line-ending, filter or attribute conversion), refuses symbolic links by tree mode and tolerates two `synod get` processes racing on the store.

## v0.2.6 — 2026-10-22

Implements Synod 0.2. Tools for people and agents: `synod doc` and `synod explain` answer questions from the command line, `synod new` and `synod get` start a module and fetch what it requires, and `synod mcp` serves the lot to agents that have no shell.

### Added

- `synod doc [package]`: prints a package's phrases, records and `Node` fields from its source, or lists the visible packages. `--json` prints `synod-doc/1`, `--scope file` gives the scope of one file for editors, `--overlay file` reads unsaved buffers and `--catalog` prints the std catalog as Markdown.
- `synod explain [code]`: explains a diagnostic code with an example and its fix, or lists the 38 codes. `--json` prints `synod-explain/1`; `--catalog` prints the table of codes, meanings and fixes as Markdown.
- `synod new <dir> [--module path] [--lib]`: creates a module with `synod.mod`, `.gitignore`, a package folder and a `demo.synod` main file that checks and runs.
- `synod get [module@version]`: makes `vendor/` and `synod.sum` match the build list, downloading what the store lacks with `git` and verifying the commit and the tree hash.
- `synod mcp [--root dir]`: a stdio MCP server with the tools `doc`, `explain`, `check`, `run_summary` and `fmt`; each gives the text of the matching command.

### Changed

- The embedded std matches std v0.2.2: every phrase, record and field has a doc comment, and these comments are what `synod doc` prints.
- `synod version` lists the new formats: `formats synod-trace/1 synod-scene/1 synod-check/1 synod-doc/1 synod-explain/1`. The same list is in `synod.version()` of the WebAssembly build.

## v0.2.5 — 2026-10-22

Implements Synod 0.2. Editors and learning modes: `synod check` can read unsaved
buffers and still colors a file that does not parse, `lesson.Ask` can offer
choices, and a run-time error inside std now points at the user's own code.

### Added

- `synod check --overlay <file>` (`synod-overlay/1`): checks each listed `.synod` file or `synod.mod` as if it held the content of another file; diagnostics and tokens keep the original path and positions. A missing overlay, a bad format or a key that is not an existing file is exit 2.
- `synod check --json --tokens` lists lexical tokens (`language`, `value`, `comment` and `name`, without `def`) for a file that fails to lex or parse, for every other file of its package and for extra main files, so an editor keeps its highlighting while the user types.
- `lesson.Ask [q: text] choosing [c: list of text]` for Predict: 2 to 6 distinct, non-empty choices, else a `bad-argument` run-time error.
- `scene.json` shapes carry `node` and `msg` (the node or message a shape stands for), and `pause` markers carry `choices` when the question has choices.

### Changed

- A run-time error raised while std code runs is reported at the position in user code of the call that entered std, in the trace (`sim.Failed`), the `failed at` and `look failed at` lines, the scene's `failed` marker and the rich form; the std position is kept as a note (decision 89). `lesson.Ask … choosing` with bad choices now points at the `lesson.Ask` call, not at `std/lesson/lesson.synod`.
- The trace record `lesson.Asked` gains the field `Choices` (`{}` for a plain `lesson.Ask`), so every trace and summary of a program with a question changes.

## v0.2.4 — 2026-10-22

Implements Synod 0.2. The compiler now also runs in the browser: `synod.wasm`
checks, formats, runs and draws programs with exactly the same output as the
command line, ready for the website's interactive examples.

### Added

- `synod.wasm`, the compiler for WebAssembly (`GOOS=js GOARCH=wasm`, built from `cmd/synod-wasm`): a global `synod` with the synchronous functions `check`, `run`, `format` and `version`. They take the program as a `files` object and return the CLI's check.json, status, summary, trace and scene as text, byte-identical to the native build; they never throw (errors are `{code, message}` with exit code 2). std is embedded; other modules come in `files` under `vendor/`.
- Release assets `synod-<tag>.wasm` (about 8.4 MiB, 2.2 MiB gzipped) and `synod-<tag>-wasm_exec.js` (from the Go toolchain that built the module), both listed in `SHA256SUMS`. `go run ./internal/release/cmd/wasm -tag <tag> -out dist` builds them and fails over the size budget (9.5 MiB, 2.75 MiB gzipped).
- CI runs the runtest and driver goldens compiled to WebAssembly under Node, diffs the trace and scene hashes against Linux, and compares the JS API running in Node with the native API and the goldens (`internal/wasmtest`).

### Changed

- The call-depth limit is 500 nested calls on every target (it was 10000), in the simulator and in the scene builder (`value.MaxCallDepth`): the WebAssembly build overflows the JS stack of plain Node at about 3000 nested calls, and browsers can have smaller stacks. The `run-limit` message is still `more than <limit> nested calls`.
- `synod check` and `synod fmt` skip folders named `vendor` when they walk a folder, like `testdata`, `.x` and `_x`; an explicit file or folder argument is still processed.

## v0.2.3 — 2026-10-22

Implements Synod 0.2, now complete. `synod run` draws the run: next to the
trace it writes `scene.json`, the animation the player will show. Programs
without their own look are drawn by the default look: node cards with their
state, messages flying between nodes, crashes, broken invariants and
captions.

### Added

- `synod run` writes `out/scene.json` (`synod-scene/1`) next to the trace and prints `scene <length>, <n> shapes -> out/scene.json` (the length in seconds, such as `147.98s`) after the existing status lines. The scene is written for every run, also after `sim.Failed`; `--summary` prints the same text as before and writes the scene silently.
- `--scene file` (`-` for stdout, with the status lines on stderr; `--trace -` and `--scene -` together exit 2), `--pace x` (0.25 to 4, up to three decimals) and `--mode ordered|proportional`. `--watch` and `--json-events` still exit 2.
- The scene builder runs the observers of the program's looks over the facts of the run. A program that does not use `std/look` is drawn by `std/look/basic`: node cards, state rows, message flights, the `sim.Broken` border and lesson captions.
- A fault inside a look is a look failure, not part of the trace: `look failed at <time>: <pos>: error[<code>]: <message>`, exit 1, and a partial scene with a `failed` marker.
- `trace.Decode` reads `trace.jsonl` back into facts; `Build(facts)` equals `Build(Decode(Encode(facts)))`.
- std: `look.Flight`, `net.Id`, `net.Type`, `sim.StateRows`, and `look.Tween` of `"x"` and `"y"` to a shape; `look.Travel`, `look.Remove` and `std/look/basic` are ordinary Synod.
- Golden `scene.json` for every example, scene determinism over 20 seeds, scene hashes in the cross-OS CI job, `FuzzScene` in the weekly fuzz run, and `BenchmarkScenePing5000`.
- The `run` fuzz target and the generator now also build scenes, with generated looks.

### Changed

- The status of a failed run is followed by the scene line, so the rich `failed at …` goldens of `testdata/run/errors` end with it.

## v0.2.2 — 2026-10-21

Implements Synod 0.2. Errors now look like a good compiler's: on a terminal,
every compile-time and run-time error shows the source line, underlines the
exact problem, points at related places ("first defined here", "called from
node context here", "this is 0") and shows the fix as a diff. Piped output,
which agents and CI read, is unchanged byte for byte.

### Added

- `--style rich|short` and `--color auto|always|never` on `synod fmt`, `check` and `run` (both `--flag value` and `--flag=value`; a bad value exits 2). Rich output draws the source line with markers, labels, notes and help under the unchanged first line; it is the default when the destination is a terminal. Piped or redirected output stays the one-line form byte for byte. Color needs `NO_COLOR` unset or empty and `TERM` other than `dumb`; the flags beat the environment. `--json` ignores both flags.
- `synod run` failures render a snippet under the `failed at …` line in rich style.
- `check.json` diagnostics gain `end`, and `label`, `labels`, `help` (with edits) and `notes` when present; the format stays `synod-check/1`.
- A diagnostic without a position prints as `path: error[code]: message`.
- Fixes shown under `help:` for missing parentheses, imports to add, remove
  or rename, the package header, canonical form, receivers, `sim.Nobody`,
  "did you mean" names and more; each is tested to remove its error.
- Hidden control and bidi characters in source are neutralised in snippets,
  so a file cannot inject terminal escapes.
- Golden cases of `testdata/check` have `expect-rich.txt` and `expect.json` next to `expect.txt`; `testdata/run/errors` cases have `expect-rich.txt`.

## v0.2.1 — 2026-10-21

Implements Synod 0.2. `synod run` simulates a program deterministically and writes its trace.

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
