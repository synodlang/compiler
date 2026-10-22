# compiler

The `synod` command: it formats, checks and runs Synod programs, deterministically.

[![ci](https://github.com/synodlang/compiler/actions/workflows/ci.yml/badge.svg)](https://github.com/synodlang/compiler/actions/workflows/ci.yml)
[![release](https://img.shields.io/github/v/release/synodlang/compiler)](https://github.com/synodlang/compiler/releases/latest)
[![licence](https://img.shields.io/badge/licence-Apache--2.0-blue)](LICENSE)

Synod is a small, consistent language for describing distributed systems.
A program is simulated on a deterministic clock, and the run can later be
animated. This repository holds the `synod` binary, written in Go with the
standard library only. It implements Synod 0.2.

## Where it fits

The language is specified in [dsl](https://github.com/synodlang/dsl). This
repository turns that specification into a tool.

| Repository | Role | Status |
| --- | --- | --- |
| [dsl](https://github.com/synodlang/dsl) | The specification, the std catalog, the implementation guides | Released (v0.2) |
| `compiler` (this one) | `synod fmt`, `synod check`, `synod run` | Released (v0.2.2) |
| [std](https://github.com/synodlang/std) | The standard library, embedded in the binary | Released |
| [examples](https://github.com/synodlang/examples) | Sample programs; the acceptance tests | Released |
| `player`, Mac app, agent skills | Playback and tooling around the trace and the scene | Later milestones |

The pipeline, and how much of it exists today:

```
.synod sources --> check --> simulate --> trace.jsonl --> scene builder --> scene.json --> player
                   done      done         done            done             done          not yet
```

## Install

The full guide is at https://synodlang.github.io/install/.

```
brew install synodlang/tap/synod
```

```
scoop bucket add synodlang https://github.com/synodlang/scoop-bucket
scoop install synod
```

```
curl -fsSL https://synodlang.github.io/install.sh | sh
```

```
go install github.com/synodlang/compiler/cmd/synod@latest
```

You can also download a binary from the
[releases](https://github.com/synodlang/compiler/releases) page. The
binaries are not code-signed yet, so macOS and Windows warn about a file
downloaded with a browser. The package managers and `install.sh` avoid
the warning.

Check the install:

```
synod version
```

It prints three lines: the language version, the compiler version with its
commit, and the file formats it reads and writes.

## Quick start

Get the examples and look at the TCP handshake. Each command below was run
with the current build.

```
git clone https://github.com/synodlang/examples
cd examples
```

Check a program. A clean check prints nothing and exits 0.

```
synod check ./tcp
```

Run it. The simulator writes `out/trace.jsonl`, the scene builder draws the
run into `out/scene.json`, and two lines are printed. `empty` means the run
ended because nothing more could happen; `stop` would mean it reached
`sim.StopAt`. The scene line gives the length of the animation and the number
of shapes.

```
$ synod run ./tcp
ok seed 1: empty at 100ms, 2 nodes, 19 events -> out/trace.jsonl
scene 2.19s, 20 shapes -> out/scene.json
```

Read the run as text with `--summary`. Facts are grouped per step, and the
last block shows every node at the end.

```
$ synod run ./tcp --summary
run github.com/synodlang/examples/tcp seed 1
end: empty at 100ms, 2 nodes, 9 steps, 19 events
steps:
@0s main
  sim.Started
  sim.Added n1 runs tcp: tcp.State "CLOSED"
  sim.Added n2 runs tcp: tcp.State "CLOSED"
  sim.Changed tcp.State of n2 from "CLOSED" to "LISTEN"
  sim.Emitted lesson.Noted{Text "A connection needs three messages before data can flow."} by nobody
  sim.Injected tcp.Open{Server n2} into n1
@0s deliver n1
  sim.Changed tcp.State of n1 from "CLOSED" to "SYN-SENT"
  net.Sent #1 tcp.Syn{Seq 100} from n1 to n2
@5ms timer main
  sim.Emitted lesson.Asked{Question "What does the server send back after a SYN?", Choices {"An ACK, and the connection is open", "A SYN-ACK: its own sequence number and an acknowledgment of the client's", "A FIN, closing the connection", "Nothing until the client sends data"}} by nobody
@22.142ms receive n2
  net.Arrived #1 tcp.Syn{Seq 100} from n1 to n2
  sim.Changed tcp.State of n2 from "LISTEN" to "SYN-RECEIVED"
  net.Sent #2 tcp.SynAck{Seq 300, AckNo 101} from n2 to n1
@33.094ms receive n1
  net.Arrived #2 tcp.SynAck{Seq 300, AckNo 101} from n2 to n1
  sim.Changed tcp.State of n1 from "SYN-SENT" to "ESTABLISHED"
  net.Sent #3 tcp.Ack{AckNo 301} from n1 to n2
@59.234ms receive n2
  net.Arrived #3 tcp.Ack{AckNo 301} from n1 to n2
  sim.Changed tcp.State of n2 from "SYN-RECEIVED" to "ESTABLISHED"
@100ms timer main
  sim.Emitted lesson.Noted{Text "Both ends are ESTABLISHED: data can now flow either way."} by nobody
  sim.Ended empty
final:
  n1 up  tcp.State "ESTABLISHED"
  n2 up  tcp.State "ESTABLISHED"
```

The same program and seed give the same bytes on every run and every
platform. Try `--seed 7` and compare.

Now break something. In a copy of the checkout, misspell `net.Send` as
`net.Sned` on line 27 of `tcp/tcp.synod`, and check it. On a terminal you
get the rich form; piped output stays the short form. The rich form below
was made with `--style rich --color never`.

```
$ synod check --style rich --color never ./tcp
tcp/tcp.synod:27:3: error[unknown-phrase]: no phrase matches "net.Sned (message Syn with Seq n.seq) from n to Open.Server"
   |
27 |   net.Sned (message Syn with Seq n.seq) from n to Open.Server
   |   ^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^ no phrase takes these items
   |
   = note: "net.Send" is in std/net
help: did you mean "net.Send [m: msg] from [a: node] to [b: node]"?
   |
27 |   net.Send (message Syn with Seq n.seq) from n to Open.Server
   |   ~~~~~~~~
   |
```

The first line is the same in both styles. Agents and CI read the short
form, which has that line and a `hint:` line, and nothing else. Exit code 1.

## Commands

| Command | What it does |
| --- | --- |
| `synod fmt [paths...] [--check]` | Rewrites `.synod` files into canonical form. With `--check` it changes nothing, lists the files that would change and exits 1. |
| `synod check [paths...] [--json] [--tokens] [--overlay file]` | Loads, resolves and checks packages, and prints every diagnostic. With no path it checks the current folder. `--json` prints the `synod-check/1` format. `--tokens` adds a token list per file, for editors (needs `--json`; without it the command exits 2); a file that does not parse still gets lexical tokens. `--overlay file` checks files as if they held other content (`synod-overlay/1`: `{"format":"synod-overlay/1","replace":{"<path>":"<path>"}}`), for unsaved editor buffers. |
| `synod run <folder> [--seed n] [--trace file] [--scene file] [--pace x] [--mode m] [--summary]` | Checks the folder's main file, simulates it and draws the run. Writes the trace and the scene and prints the status lines. |
| `synod version` | Prints the language, compiler and format versions. |

`fmt`, `check` and `run` also take `--style rich|short` and
`--color auto|always|never`, as `--style rich` or `--style=rich`. Any other
value exits 2.

- Rich is the default when the output is a terminal, short otherwise. The
  short form is unchanged by the rich style, byte for byte.
- Color needs `NO_COLOR` unset or empty and `TERM` other than `dumb`.
  `--color` beats the environment. `--json` ignores both flags.

Flags of `synod run`:

| Flag | Meaning |
| --- | --- |
| `--seed n` | Seed of the run, a whole number from 0 to 2^53. It wins over `sim.Seed` in the program. Without either, the seed is 1. |
| `--trace file` | Where to write the trace. The default is `out/trace.jsonl`; parent folders are created. `--trace -` writes the trace to stdout and the status line to stderr. |
| `--scene file` | Where to write the scene. The default is `out/scene.json`; parent folders are created. `--scene -` writes the scene to stdout and the status lines to stderr. `--trace -` and `--scene -` together exit 2. |
| `--pace x` | Playback speed of the scene, a decimal number from 0.25 to 4 with at most three decimals (default 1). `--pace 2` makes the animation half as long. |
| `--mode ordered\|proportional` | How facts are spaced in the scene. `ordered` (default) gives every fact its own moment; `proportional` spreads them over about 30 seconds in proportion to simulated time. |
| `--summary` | Prints a text summary instead of the status lines. The trace and the scene are still written, silently. |

Programs that use `std/look` bring their own looks; every other program is
drawn by `std/look/basic`. A run-time error in a look is a look failure: it
is not in the trace and does not fail the simulation. The scene stops at that
fact and gets a `failed` marker, and `synod run` prints `look failed at
<time>: <pos>: error[<code>]: <message>` in place of the scene line and exits
1. The partial scene is still written.

`--watch` and `--json-events` are recognized but not built yet: they exit 2
with `not implemented yet`. Likewise `synod
new`, `get`, `doc` and `explain`, which the
[CLI guide](https://github.com/synodlang/dsl/blob/main/implementation/05-cli.md)
plans, do not exist yet.

### Exit codes

| Code | Meaning |
| --- | --- |
| 0 | Success. A run that ended is 0, also when an `always` invariant broke: that is a result, not an error. |
| 1 | Diagnostics, files that `fmt --check` would change, a run that failed (the trace ends in `sim.Failed`), or a look that failed. |
| 2 | Usage or I/O error. |

## Output formats

Both formats are versioned contracts, specified in
[implementation/04-formats.md](https://github.com/synodlang/dsl/blob/main/implementation/04-formats.md)
in dsl. Inside a major version, keys are only added, and readers ignore keys
they do not know.

- `check.json` (`synod-check/1`): `synod check --json`. One entry per
  diagnostic, with code, position, end, message and hint, plus `label`,
  `labels`, `help` (with edits) and `notes` when present.
- `trace.jsonl` (`synod-trace/1`): one JSON object per line. Line 1 is a
  header with the format, language version, seed, program and packages. Then
  one line per trace event (`sim.Changed`, `net.Sent`, `net.Arrived`,
  `sim.Broken`, and so on). Times are integer microseconds of simulated
  time. The events are listed in
  [spec/08](https://github.com/synodlang/dsl/blob/main/spec/08-trace-events.md).
- `scene.json` (`synod-scene/1`): the animation of a run as shapes with
  keyframes, markers and one slot per trace fact. Times are integer
  milliseconds of presentation time. The same program, seed, pace and mode
  give the same bytes on every platform.

A run-time error is the last line of the trace, a `sim.Failed` event, and
`synod run` prints `failed at <time>: <pos>: error[<code>]: <message>` with
exit 1. The codes are `missing-key`, `index-range`, `division-by-zero`,
`nobody`, `disk-type`, `time-limit` (10 minutes of simulated time),
`bad-argument`, `number-range` and `run-limit`.

## Determinism

The same program, seed and Synod version give a byte-identical trace, on
Linux, macOS and Windows. The rules are in
[spec/07](https://github.com/synodlang/dsl/blob/main/spec/07-execution-model.md#77-determinism).
In this implementation:

- One PRNG, PCG32 (`internal/rng`), seeded with the seed of the run.
- Every random decision draws exactly one number, whatever its arguments,
  and draws happen in step order.
- Time is simulated; no wall-clock time reaches the trace or stdout.
- Evaluation is left to right; maps iterate in ascending key order.
- Numbers are float64, with no fused multiply-add; `math.Hash` is a fixed
  FNV-1a 64 mixed with the MurmurHash3 finalizer.
- Paths in traces are relative to the module, so the working directory does
  not matter.

CI proves it: the `trace-hashes` job hashes the trace of every example on
Linux, macOS and Windows and compares the three lists.

## Project layout

```
cmd/synod/          the command: flags, exit codes, terminal detection
internal/
  source/           files, positions, diagnostics and their codes
  lex/              the line-oriented lexer; INDENT and DEDENT tokens
  parse/            the structure grammar: lines, blocks, continuation lines
  format/           canonical form and its fixes
  load/             synod.mod, the package graph, main files, imports
  stdlib/           the std sources, embedded in the binary
  resolve/          scopes and phrase matching
  types/            language types and their acceptance rules
  check/            type and context checking; lowers programs to the IR
  ir/               the checked program the simulator runs
  driver/           the pipeline: load, resolve, format check, check
  render/           rich diagnostics: snippets, labels, help diffs, color
  value/            run-time values and their text form
  rng/              PCG32
  sim/              the event loop, nodes, network, disk, invariants
  natives/          the native phrases of std (sim, net, time, random...)
  trace/            facts, trace.jsonl and the summary
  release/          release notes and package-manager files
  spectest/         tests against the dsl and examples repositories
  runtest/          golden runs, seed stories, fuzzing, benchmarks
testdata/           golden inputs and outputs for every stage
```

## Development

You need Go 1.23 or later. There are no dependencies besides the standard
library.

```
go build -o synod ./cmd/synod
go vet ./...
go test ./...
```

Some tests read the sibling repositories. By default they look for `../dsl`,
`../std` and `../examples` next to this checkout, and skip when a folder is
missing. Point them elsewhere with an environment variable, and set the
matching `SYNOD_REQUIRE_*=1` to fail instead of skip (CI does).

| Variable | What it enables |
| --- | --- |
| `SYNOD_DSL` | Checks every `synod` code block of the specification, and that the std catalog and README snippets match the std tables. |
| `SYNOD_STD` | Checks that the embedded copy in `internal/stdlib/std` equals the `std` repository (a drift test). |
| `SYNOD_EXAMPLES` | Checks that the vendored copy in `testdata/examples` equals the `examples` repository (a drift test). |

Other switches: `SYNOD_HASHES_OUT=<file>` writes the example trace hashes
(used by the cross-platform CI job), and `SYNOD_FIND_SEEDS=1` makes the story
tests log the smallest seed that tells each example's story.

```
SYNOD_DSL=$HOME/src/dsl SYNOD_REQUIRE_DSL=1 go test ./internal/spectest ./internal/stdlib
```

Golden files live in `testdata` and next to each package. After a deliberate
change, rewrite them and review the diff:

```
go test ./internal/driver ./internal/format ./internal/lex ./internal/load \
  ./internal/parse ./internal/render ./internal/release ./internal/sim \
  ./internal/trace ./internal/runtest ./cmd/synod -update
```

Only packages with goldens define `-update`, so `go test ./... -update` fails
on the others. Name the packages as above.

The cases of `testdata/check` each have `expect.txt` (short form),
`expect-rich.txt` (rich form without color) and `expect.json`; the cases of
`testdata/run/errors` have `expect-rich.txt`.

Fuzzing runs weekly in CI and on demand (`fuzz.yml`, ten minutes per
target). Locally:

```
go test ./internal/lex -run '^$' -fuzz '^FuzzLex$' -fuzztime 1m
go test ./internal/parse -run '^$' -fuzz '^FuzzParse$' -fuzztime 1m
go test ./internal/format -run '^$' -fuzz '^FuzzFormat$' -fuzztime 1m
go test ./internal/driver -run '^$' -fuzz '^FuzzCheck$' -fuzztime 1m
go test ./internal/runtest -run '^$' -fuzz '^FuzzRun$' -fuzztime 1m
```

CI also vets and builds for WebAssembly, runs the run and driver goldens
compiled to WebAssembly under Node, and runs `internal/wasmtest`, which loads
`synod.wasm` in Node and compares every call with the native API and the
goldens. `SYNOD_REQUIRE_NODE=1 go test ./internal/wasmtest` fails instead of
skipping when Node is missing.

## WebAssembly

The compiler also builds as a WebAssembly module for the browser and Node.

```
go run ./internal/release/cmd/wasm -tag v0.3.0 -out dist
```

This writes `dist/synod-<tag>.wasm` and `dist/synod-<tag>-wasm_exec.js` (the
`wasm_exec.js` of the same Go toolchain; the two must match) and prints the
sizes (about 8.4 MiB, 2.2 MiB gzipped; the build fails over the budget). Both
files are release assets and are listed in `SHA256SUMS`.

Load the module, then use the global `synod`:

```js
const go = new Go(); // from synod-<tag>-wasm_exec.js
const { instance } = await WebAssembly.instantiateStreaming(fetch("synod-v0.3.0.wasm"), go.importObject);
go.run(instance); // do not await: it never returns
const r = synod.run({ "synod.mod": mod, "p/main.synod": src }, { program: "p", seed: 1 });
```

The calls are synchronous and CPU-bound. Run the module in a Web Worker so the
page stays responsive. Calls never throw: a result with `error` set has exit
code 2 and every output `null`. If a JS exception does reach you, the Go
instance is dead; instantiate it again.

`files` maps relative slash paths to source text. std is embedded; other
modules go under `vendor/<module>@<version>/` with a `require` line in
`synod.mod`, as `synod get` lays them out. Results are the CLI's bytes as
strings.

| Call | Options | Result |
| ---- | ------- | ------ |
| `synod.check(files, options?)` | `paths`, `tokens`, `style` (`rich` or `short`) | `exitCode`, `error`, `check` (check.json), `text` |
| `synod.run(files, options)` | `program` (required), `seed`, `pace`, `mode`, `style`, `summary` | `exitCode`, `error`, `check`, `status`, `summary`, `trace`, `scene`, `failure` |
| `synod.format(files, options?)` | `paths`, `style` | `exitCode`, `error`, `files`, `changed`, `check`, `text` |
| `synod.version()` | none | `api` (1), `language`, `compiler`, `formats`, `go`, `text` |

## Releases

A release is built on every tag that starts with `v`.

- The version comes from the tag. The commit comes from Go build info.
- The release notes are the section of `CHANGELOG.md` whose heading is
  `## <tag>`. Write the section first, then tag; a missing or empty section
  stops the release.
- It builds `synod-<tag>-<os>-<arch>.tar.gz` (Windows: `.zip`) for macOS,
  Linux and Windows on amd64 and arm64, a macOS universal archive, and
  `SHA256SUMS`, then updates the Homebrew tap and the Scoop bucket.
- Versioning rule: MAJOR.MINOR is the Synod language version. Language 0.2
  means this repository releases `v0.2.x`. A patch is a bug fix or more of the
  same language implemented. A new language feature raises MINOR, after the
  specification changes in dsl.

## Status and roadmap

Today (v0.2.5, implements Synod 0.2): `synod fmt`, `synod check` with every
compile-time error of the specification, rich diagnostics, and `synod run`
with the deterministic simulator, `trace.jsonl` and `--summary`. Every native
phrase of `std/sim`, `net`, `time`, `random`, `lists`, `maps`, `math`, `text`
and `disk` works.

Released: the `look` phrases and observers, the scene builder and
`scene.json` (v0.2.3), and `synod.wasm` for browsers (v0.2.4).

Not yet: `--watch` and `--json-events`, and then the player, the Mac app, the editor modes and the agent skills (M7 to M11).
The order and acceptance checks are in the
[roadmap](https://github.com/synodlang/dsl/blob/main/implementation/10-roadmap.md).
The [change log](CHANGELOG.md) lists what each release added.

## Why Synod

Synod is named after the Synod protocol, the core consensus algorithm in Leslie Lamport's paper "The Part-Time Parliament" (ACM Transactions on Computer Systems, 1998; first submitted in 1990). The paper tells it as the story of the parliament of the ancient Greek island of Paxos, whose part-time legislators could agree on decrees although they came and went; today the algorithm is known as Paxos. Lamport later explained it plainly in "Paxos Made Simple" (2001), and in 2013 he received the ACM A.M. Turing Award for fundamental contributions to the theory and practice of distributed and concurrent systems. His "Time, Clocks, and the Ordering of Events in a Distributed System" (1978) and his TLA+ specification language shaped how we reason about such systems. We are grateful for his work, and we hope Synod honours it by making these ideas easier to watch and understand. The compiler simulates programs deterministically, in the spirit of reasoning carefully about the order of events.


Implements Synod 0.2 (specification: https://github.com/synodlang/dsl).

Licensed under the [Apache License 2.0](LICENSE).
