package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/synodlang/compiler/internal/driver"
	"github.com/synodlang/compiler/internal/render"
	"github.com/synodlang/compiler/internal/sim"
	"github.com/synodlang/compiler/internal/source"
	"github.com/synodlang/compiler/internal/trace"
	"github.com/synodlang/compiler/internal/value"
)

// simRun runs the simulator; tests replace it to provoke a panic.
var simRun = sim.Run

// maxSeed is the largest seed (2^53): above it a number is not exact.
const maxSeed = 1 << 53

// defaultTrace is where the trace goes without --trace.
const defaultTrace = "out/trace.jsonl"

// runArgs are the parsed arguments of `synod run`.
type runArgs struct {
	folder  string
	seed    *uint64
	trace   string
	summary bool
	style   styleFlags
}

// notYet are the flags of later milestones: recognized and rejected.
var notYet = map[string]bool{"scene": true, "pace": true, "mode": true, "watch": true, "json-events": true}

// parseRunArgs parses `synod run` arguments: one folder, --seed (n or =n),
// --trace (file or =file) and --summary, in any order. "--" ends the flags.
func parseRunArgs(args []string) (runArgs, error) {
	ra := runArgs{trace: defaultTrace}
	var pos []string
	flags := true
	for i := 0; i < len(args); i++ {
		a := args[i]
		if !flags || !strings.HasPrefix(a, "-") || a == "-" {
			pos = append(pos, a)
			continue
		}
		if a == "--" {
			flags = false
			continue
		}
		name := strings.TrimLeft(a, "-")
		val, hasVal := "", false
		if k := strings.IndexByte(name, '='); k >= 0 {
			name, val, hasVal = name[:k], name[k+1:], true
		}
		value := func() (string, error) {
			if hasVal {
				return val, nil
			}
			if i+1 >= len(args) {
				return "", fmt.Errorf("--%s needs a value", name)
			}
			i++
			return args[i], nil
		}
		switch {
		case notYet[name]:
			return ra, fmt.Errorf("--%s is not implemented yet", name)
		case name == "summary":
			if hasVal {
				return ra, fmt.Errorf("--summary takes no value")
			}
			ra.summary = true
		case name == "seed":
			v, err := value()
			if err != nil {
				return ra, err
			}
			n, perr := strconv.ParseUint(v, 10, 64)
			if perr != nil || n > maxSeed {
				return ra, fmt.Errorf("--seed needs a whole number from 0 to %d, not %q", uint64(maxSeed), v)
			}
			ra.seed = &n
		case name == "style" || name == "color":
			v, err := value()
			if err != nil {
				return ra, err
			}
			if name == "style" {
				ra.style.style = v
			} else {
				ra.style.color = v
			}
			if err := ra.style.check(); err != nil {
				return ra, err
			}
		case name == "trace":
			v, err := value()
			if err != nil {
				return ra, err
			}
			if v == "" {
				return ra, fmt.Errorf("--trace needs a file name")
			}
			ra.trace = v
		default:
			return ra, fmt.Errorf("unknown flag %q", a)
		}
	}
	switch len(pos) {
	case 0:
		return ra, fmt.Errorf("needs a folder")
	case 1:
		ra.folder = pos[0]
	default:
		return ra, fmt.Errorf("takes exactly one folder, got %d", len(pos))
	}
	return ra, nil
}

// runRun implements `synod run` (implementation/05-cli.md).
func runRun(args []string, stdout, stderr io.Writer) (code int) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(stderr, "synod run: internal error: %v\n", r)
			code = exitUsage
		}
	}()
	ra, err := parseRunArgs(args)
	if err != nil {
		fmt.Fprintf(stderr, "synod run: %v\n", err)
		return exitUsage
	}
	info, err := os.Stat(ra.folder)
	if err != nil {
		fmt.Fprintf(stderr, "synod run: %v\n", err)
		return exitUsage
	}
	if !info.IsDir() {
		fmt.Fprintf(stderr, "synod run: %s is not a folder\n", ra.folder)
		return exitUsage
	}
	folder := filepath.ToSlash(filepath.Clean(ra.folder))
	res := driver.Check(driver.Options{}, []string{folder})
	if diags := userDiags(res); len(diags) > 0 {
		if err := writeDiags(stdout, diags, ra.style, &sources{known: res.Sources}); err != nil {
			fmt.Fprintf(stderr, "synod run: %v\n", err)
			return exitUsage
		}
		return exitDiags
	}
	var ipath string
	if res.Graph != nil {
		for _, p := range res.Graph.Targets {
			if p.IsMain {
				ipath = p.Path
			}
		}
	}
	prog := res.Programs[ipath]
	if ipath == "" || prog == nil {
		d := source.Diagnostic{File: folder, Severity: "error", Code: source.MissingMain, Message: "no main file in " + folder}
		if err := writeDiags(stdout, []source.Diagnostic{d}, ra.style, &sources{known: res.Sources}); err != nil {
			fmt.Fprintf(stderr, "synod run: %v\n", err)
			return exitUsage
		}
		return exitDiags
	}

	opts := sim.Options{}
	if ra.seed != nil {
		opts.Seed = ra.seed
	}
	r := simRun(prog, opts)
	topts := trace.Options{Paths: trace.FilePaths(res.Graph), Qual: trace.Qualifiers(prog)}
	var buf bytes.Buffer
	if err := trace.Encode(&buf, r.Header, r.Facts, topts); err != nil {
		fmt.Fprintf(stderr, "synod run: %v\n", err)
		return exitUsage
	}
	status := stdout
	if ra.trace == "-" {
		status = stderr
		if _, err := stdout.Write(buf.Bytes()); err != nil {
			fmt.Fprintf(stderr, "synod run: %v\n", err)
			return exitUsage
		}
	} else {
		if dir := filepath.Dir(ra.trace); dir != "." {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				fmt.Fprintf(stderr, "synod run: %v\n", err)
				return exitUsage
			}
		}
		if err := os.WriteFile(ra.trace, buf.Bytes(), 0o644); err != nil {
			fmt.Fprintf(stderr, "synod run: %v\n", err)
			return exitUsage
		}
	}

	if ra.summary {
		in := trace.SummaryInput{
			Header: r.Header, Facts: r.Facts, Steps: r.Steps, Final: r.Final,
			Invariants: r.Invariants, EndReason: r.EndReason, End: r.End,
			Failed: r.Failed, TracePath: ra.trace,
		}
		if err := trace.WriteSummary(status, in, topts); err != nil {
			fmt.Fprintf(stderr, "synod run: %v\n", err)
			return exitUsage
		}
	} else {
		writeStatus(status, ra, r, topts, res.Sources)
	}
	if r.Failed != nil {
		return exitDiags
	}
	return exitOK
}

// writeStatus prints the status line and one line per broken always.
func writeStatus(w io.Writer, ra runArgs, r *sim.Result, o trace.Options, known map[string]*source.File) {
	if f := r.Failed; f != nil {
		line := trace.FailedLine(f, r.End, o)
		rich, color := ra.style.resolve(w)
		if !rich {
			fmt.Fprintln(w, line)
			return
		}
		// The renderer looks files up by trace path; known is keyed by
		// display path.
		alias := map[string]string{}
		for display, tp := range o.Paths {
			alias[tp] = display
		}
		src := &sources{known: known, alias: alias}
		d := trace.FaultDiagnostic(f, o)
		if err := render.Write(w, d, render.Options{Color: color, Source: src.get, Header: line}); err != nil {
			fmt.Fprintln(w, line)
		}
		return
	}
	dest := ra.trace
	if dest == "-" {
		dest = "stdout"
	}
	fmt.Fprintf(w, "ok seed %d: %s at %s, %d nodes, %d events -> %s\n",
		r.Header.Seed, r.EndReason, value.FormatTime(r.End), len(r.Final), len(r.Facts), dest)
	for _, inv := range r.Invariants {
		if inv.Breaks == 0 {
			continue
		}
		rule := inv.Rule
		if inv.Pos.File != "" && strings.HasPrefix(rule, inv.Pos.File+":") {
			if t, ok := o.Paths[inv.Pos.File]; ok {
				rule = t + rule[len(inv.Pos.File):]
			}
		}
		times := "1 time"
		if inv.Breaks > 1 {
			times = fmt.Sprintf("%d times", inv.Breaks)
		}
		fmt.Fprintf(w, "broken at %s (%s): %s\n", value.FormatTime(inv.FirstBreak), times, rule)
	}
}
