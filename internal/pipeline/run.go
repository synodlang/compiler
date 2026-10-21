package pipeline

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"

	"github.com/synodlang/compiler/internal/driver"
	"github.com/synodlang/compiler/internal/ir"
	"github.com/synodlang/compiler/internal/load"
	"github.com/synodlang/compiler/internal/render"
	"github.com/synodlang/compiler/internal/scene"
	"github.com/synodlang/compiler/internal/sim"
	"github.com/synodlang/compiler/internal/source"
	"github.com/synodlang/compiler/internal/trace"
	"github.com/synodlang/compiler/internal/value"
)

// Exit codes of a run (implementation/05-cli.md).
const (
	ExitOK    = 0
	ExitDiags = 1
	ExitUsage = 2
)

// ParsePace reads a decimal number of at most three fraction digits from 0.25
// to 4 as thousandths.
func ParsePace(v string) (int, bool) {
	whole, frac, dot := strings.Cut(v, ".")
	if len(whole) != 1 || len(frac) > 3 || dot && frac == "" {
		return 0, false
	}
	for _, part := range []string{whole, frac} {
		for _, c := range part {
			if c < '0' || c > '9' {
				return 0, false
			}
		}
	}
	n := int(whole[0]-'0') * 1000
	for i := 0; i < len(frac); i++ {
		n += int(frac[i]-'0') * [3]int{100, 10, 1}[i]
	}
	return n, n >= 250 && n <= 4000
}

// FormatSceneLength prints ms as seconds with at most two decimals, trailing
// zeros trimmed, rounding half away from zero: 147982 is "147.98s".
func FormatSceneLength(ms int64) string {
	neg := ms < 0
	if neg {
		ms = -ms
	}
	h := (ms + 5) / 10 // hundredths of a second
	s := strconv.FormatInt(h/100, 10)
	if f := h % 100; f != 0 {
		s += "." + strings.TrimRight(fmt.Sprintf("%02d", f), "0")
	}
	if neg && h != 0 {
		s = "-" + s
	}
	return s + "s"
}

// RunInput is everything Run needs: where to read the program from, how to
// run it and draw it, and how to word the output.
type RunInput struct {
	// Load tells the loader where to read from (nil Std means the embedded
	// std).
	Load load.Config
	// Folder is the folder to run, as a load path; FolderShown is how
	// diagnostics name it.
	Folder      string
	FolderShown string
	// Seed is the seed, nil for the program's own.
	Seed *uint64
	// Pace is the scene pace in thousandths (1000 is 1x).
	Pace int
	Mode scene.Mode
	// TraceDest and SceneDest are the destinations named in the status
	// lines ("-" reads as "stdout"); TracePath is the trace path of the
	// summary.
	TraceDest string
	SceneDest string
	TracePath string
	// WantSummary also builds Summary.
	WantSummary bool
	// DiagStyle styles the diagnostics that stop a run; StatusStyle styles
	// the failure diagnostics of the status.
	DiagStyle   Style
	StatusStyle Style
	// Lookup finds a source file the graph does not hold (the CLI reads the
	// disk); nil means none.
	Lookup func(string) *source.File
	// Sim runs the simulator; nil means sim.Run.
	Sim func(*ir.Program, sim.Options) *sim.Result
}

// Failure is a run failure or a look failure.
type Failure struct {
	// Kind is "run" or "look".
	Kind string
	// Line is the "failed at ..." text; for a look it begins with "look ".
	Line string
	// Diagnostic is the fault as a diagnostic.
	Diagnostic source.Diagnostic
}

// RunOutput is the outcome of Run.
type RunOutput struct {
	// Code is the exit code: 0, 1 for diagnostics, a run failure or a look
	// failure, 2 when Err is set.
	Code int
	// Diags are the user diagnostics that stopped the run, including the
	// synthesized missing-main one; empty otherwise.
	Diags []source.Diagnostic
	// Trace and Scene are the documents; nil when diagnostics stopped the
	// run.
	Trace, Scene []byte
	// Status is what `synod run` prints to stdout without --summary: the
	// diagnostics, or the status line, the broken-at lines and the scene
	// line or the look failure.
	Status []byte
	// Summary is what --summary prints, the look failure line included;
	// nil unless WantSummary and the run was not stopped.
	Summary []byte
	// Failure is the run failure, or when there is none the look failure.
	Failure *Failure
	// Err is an internal error (encoding); Code is 2 and nothing else is
	// set.
	Err error
}

// lookupFunc finds a file for the renderer: alias maps a name used by a
// diagnostic to the key in known, then the graph's sources, then fallback.
func lookupFunc(known map[string]*source.File, alias map[string]string, fallback func(string) *source.File) func(string) *source.File {
	return func(file string) *source.File {
		if a, ok := alias[file]; ok {
			file = a
		}
		if f := known[file]; f != nil {
			return f
		}
		if fallback != nil {
			return fallback(file)
		}
		return nil
	}
}

// Run checks the folder and, when it has no diagnostics, runs it, builds the
// trace and the scene and words the output. It writes nothing anywhere.
func Run(in RunInput) RunOutput {
	simRun := in.Sim
	if simRun == nil {
		simRun = sim.Run
	}
	res := driver.Check(driver.Options{Load: in.Load, DefaultLook: true}, []string{in.Folder})
	if diags := UserDiags(res); len(diags) > 0 {
		return stopped(in, res, diags)
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
		d := source.Diagnostic{File: in.FolderShown, Severity: "error", Code: source.MissingMain, Message: "no main file in " + in.FolderShown}
		return stopped(in, res, []source.Diagnostic{d})
	}

	opts := sim.Options{}
	if in.Seed != nil {
		opts.Seed = in.Seed
	}
	r := simRun(prog, opts)
	topts := trace.Options{Paths: trace.FilePaths(res.Graph), Qual: trace.Qualifiers(prog)}
	var buf bytes.Buffer
	if err := trace.Encode(&buf, r.Header, r.Facts, topts); err != nil {
		return RunOutput{Code: ExitUsage, Err: err}
	}
	var look *ir.Program
	if !scene.UsesLook(prog) {
		look = res.Programs[driver.DefaultLookPath]
	}
	sc, lfail := scene.Build(scene.Input{Prog: prog, Look: look, Facts: r.Facts, Trace: topts},
		scene.Options{Pace: in.Pace, Mode: in.Mode})
	var sbuf bytes.Buffer
	if err := trace.EncodeScene(&sbuf, sc); err != nil {
		return RunOutput{Code: ExitUsage, Err: err}
	}
	out := RunOutput{Trace: buf.Bytes(), Scene: sbuf.Bytes()}

	// The renderer looks files up by trace path; known is keyed by display
	// path.
	alias := map[string]string{}
	for display, tp := range topts.Paths {
		alias[tp] = display
	}
	lookup := lookupFunc(res.Sources, alias, in.Lookup)

	var status, lookLine bytes.Buffer
	writeStatus(&status, in, r, topts, lookup)
	if lfail != nil {
		writeLookFailure(&lookLine, in, lfail, topts, lookup)
		status.Write(lookLine.Bytes())
	} else {
		dest := in.SceneDest
		if dest == "-" {
			dest = "stdout"
		}
		fmt.Fprintf(&status, "scene %s, %s -> %s\n", FormatSceneLength(sc.Duration), trace.Count(len(sc.Shapes), "shape"), dest)
	}
	out.Status = status.Bytes()

	if in.WantSummary {
		var sum bytes.Buffer
		si := trace.SummaryInput{
			Header: r.Header, Facts: r.Facts, Steps: r.Steps, Final: r.Final,
			Invariants: r.Invariants, EndReason: r.EndReason, End: r.End,
			Failed: r.Failed, TracePath: in.TracePath,
		}
		if err := trace.WriteSummary(&sum, si, topts); err != nil {
			return RunOutput{Code: ExitUsage, Err: err}
		}
		sum.Write(lookLine.Bytes())
		out.Summary = sum.Bytes()
	}

	if f := r.Failed; f != nil {
		out.Failure = &Failure{Kind: "run", Line: trace.FailedLine(f, r.End, topts), Diagnostic: trace.FaultDiagnostic(f, topts)}
	} else if lfail != nil {
		out.Failure = &Failure{Kind: "look", Line: "look " + trace.FailedLine(lfail.Fault, lfail.T, topts), Diagnostic: trace.FaultDiagnostic(lfail.Fault, topts)}
	}
	if lfail != nil || r.Failed != nil {
		out.Code = ExitDiags
	}
	return out
}

// stopped is the output of a run that diagnostics stopped.
func stopped(in RunInput, res driver.Result, diags []source.Diagnostic) RunOutput {
	var buf bytes.Buffer
	if err := WriteDiags(&buf, diags, in.DiagStyle, lookupFunc(res.Sources, nil, in.Lookup)); err != nil {
		return RunOutput{Code: ExitUsage, Err: err}
	}
	return RunOutput{Code: ExitDiags, Diags: diags, Status: buf.Bytes()}
}

// writeFault writes line, followed by the rich rendering of d when the style
// is rich (the line is then the header).
func writeFault(w *bytes.Buffer, st Style, line string, d source.Diagnostic, lookup func(string) *source.File) {
	if !st.Rich {
		fmt.Fprintln(w, line)
		return
	}
	if err := render.Write(w, d, render.Options{Color: st.Color, Source: lookup, Header: line}); err != nil {
		fmt.Fprintln(w, line)
	}
}

// writeLookFailure writes the failure of a look in place of the scene line.
func writeLookFailure(w *bytes.Buffer, in RunInput, lf *scene.Failure, o trace.Options, lookup func(string) *source.File) {
	line := "look " + trace.FailedLine(lf.Fault, lf.T, o)
	writeFault(w, in.StatusStyle, line, trace.FaultDiagnostic(lf.Fault, o), lookup)
}

// writeStatus writes the status line and one line per broken always.
func writeStatus(w *bytes.Buffer, in RunInput, r *sim.Result, o trace.Options, lookup func(string) *source.File) {
	if f := r.Failed; f != nil {
		writeFault(w, in.StatusStyle, trace.FailedLine(f, r.End, o), trace.FaultDiagnostic(f, o), lookup)
		return
	}
	dest := in.TraceDest
	if dest == "-" {
		dest = "stdout"
	}
	fmt.Fprintf(w, "ok seed %d: %s at %s, %s, %s -> %s\n",
		r.Header.Seed, r.EndReason, value.FormatTime(r.End), trace.Count(len(r.Final), "node"), trace.Count(len(r.Facts), "event"), dest)
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
