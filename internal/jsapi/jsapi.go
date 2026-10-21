// Package jsapi is the JavaScript API of the WebAssembly build, in pure Go:
// check, run, format and version take their arguments as JSON text and return
// plain maps that syscall/js can convert (strings, bools, ints, nil, []any
// and map[string]any only). The results carry the command line's own bytes,
// assembled by internal/pipeline. No function panics: a panic becomes an
// internal error, and bad arguments a usage error, both with exit code 2.
package jsapi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"path"
	"runtime"
	"runtime/debug"
	"sort"

	"github.com/synodlang/compiler/internal/driver"
	"github.com/synodlang/compiler/internal/format"
	"github.com/synodlang/compiler/internal/pipeline"
	"github.com/synodlang/compiler/internal/source"
)

// APIVersion changes only when the JS shape changes.
const APIVersion = 1

// Where the CLI would write by default; the status and the summary name them.
const (
	defaultTrace = "out/trace.jsonl"
	defaultScene = "out/scene.json"
)

// Result fields of each function.
var (
	checkFields  = []string{"check", "text"}
	runFields    = []string{"check", "status", "summary", "trace", "scene", "failure"}
	formatFields = []string{"files", "changed", "check", "text"}
)

// failed is the result of a function that could not produce output: exit
// code 2, the error and every output field null.
func failed(fields []string, code, msg string) map[string]any {
	m := map[string]any{
		"exitCode": pipeline.ExitUsage,
		"error":    map[string]any{"code": code, "message": msg},
	}
	for _, f := range fields {
		m[f] = nil
	}
	return m
}

// Usage is the result of a call whose arguments the glue could not hand
// over; fn is "check", "run" or "format".
func Usage(fn, msg string) map[string]any {
	switch fn {
	case "check":
		return failed(checkFields, "usage", msg)
	case "run":
		return failed(runFields, "usage", msg)
	}
	return failed(formatFields, "usage", msg)
}

// guard turns a panic into an internal error.
func guard(fn string, fields []string, out *map[string]any) {
	if r := recover(); r != nil {
		*out = failed(fields, "internal", fmt.Sprintf("synod %s: internal error: %v", fn, r))
	}
}

func internal(fn string, fields []string, err error) map[string]any {
	return failed(fields, "internal", fmt.Sprintf("synod %s: internal error: %v", fn, err))
}

func ok(m map[string]any) map[string]any {
	m["error"] = nil
	return m
}

func exitOf(diags []source.Diagnostic) int {
	if len(diags) > 0 {
		return pipeline.ExitDiags
	}
	return pipeline.ExitOK
}

// Check is synod.check(files, options): files and options are JSON text.
func Check(filesJSON, optionsJSON string) (out map[string]any) {
	defer guard("check", checkFields, &out)
	var o struct {
		Paths  json.RawMessage `json:"paths"`
		Tokens json.RawMessage `json:"tokens"`
		Style  json.RawMessage `json:"style"`
	}
	usage := func(err error) map[string]any { return failed(checkFields, "usage", err.Error()) }
	if err := decodeOptions(optionsJSON, &o); err != nil {
		return usage(err)
	}
	tr, err := decodeFiles(filesJSON)
	if err != nil {
		return usage(err)
	}
	paths, err := parsePaths(o.Paths)
	if err != nil {
		return usage(err)
	}
	tokens := false
	if present(o.Tokens) {
		if tokens, err = optBool("tokens", o.Tokens); err != nil {
			return usage(err)
		}
	}
	rich, err := parseStyle(o.Style)
	if err != nil {
		return usage(err)
	}
	_, targets, err := tr.collect(paths)
	if err != nil {
		return usage(err)
	}
	res := driver.Check(driver.Options{Load: tr.loadConfig(), Tokens: tokens}, targets)
	diags := pipeline.UserDiags(res)
	var js, text bytes.Buffer
	if err := pipeline.WriteCheckJSON(&js, diags, res, tokens); err != nil {
		return internal("check", checkFields, err)
	}
	lookup := func(f string) *source.File { return res.Sources[f] }
	if err := pipeline.WriteDiags(&text, diags, pipeline.Style{Rich: rich}, lookup); err != nil {
		return internal("check", checkFields, err)
	}
	return ok(map[string]any{
		"exitCode": exitOf(diags),
		"check":    js.String(),
		"text":     text.String(),
	})
}

// Run is synod.run(files, options): files and options are JSON text.
func Run(filesJSON, optionsJSON string) (out map[string]any) {
	defer guard("run", runFields, &out)
	var o struct {
		Program json.RawMessage `json:"program"`
		Seed    json.RawMessage `json:"seed"`
		Pace    json.RawMessage `json:"pace"`
		Mode    json.RawMessage `json:"mode"`
		Style   json.RawMessage `json:"style"`
		Summary json.RawMessage `json:"summary"`
	}
	usage := func(err error) map[string]any { return failed(runFields, "usage", err.Error()) }
	if err := decodeOptions(optionsJSON, &o); err != nil {
		return usage(err)
	}
	tr, err := decodeFiles(filesJSON)
	if err != nil {
		return usage(err)
	}
	if !present(o.Program) {
		return usage(fmt.Errorf("needs a folder: set options.program"))
	}
	program, err := optString("program", o.Program)
	if err != nil {
		return usage(err)
	}
	seed, err := parseSeed(o.Seed)
	if err != nil {
		return usage(err)
	}
	pace, err := parsePace(o.Pace)
	if err != nil {
		return usage(err)
	}
	mode, err := parseMode(o.Mode)
	if err != nil {
		return usage(err)
	}
	rich, err := parseStyle(o.Style)
	if err != nil {
		return usage(err)
	}
	summary := false
	if present(o.Summary) {
		if summary, err = optBool("summary", o.Summary); err != nil {
			return usage(err)
		}
	}
	folder := path.Clean(program)
	if program == "" || !tr.isDir(folder) {
		return usage(fmt.Errorf("%s is not a folder in files", program))
	}
	style := pipeline.Style{Rich: rich}
	ro := pipeline.Run(pipeline.RunInput{
		Load:   tr.loadConfig(),
		Folder: loadPath(folder), FolderShown: folder,
		Seed: seed, Pace: pace, Mode: mode,
		TraceDest: defaultTrace, SceneDest: defaultScene, TracePath: defaultTrace,
		WantSummary: summary,
		DiagStyle:   style, StatusStyle: style,
	})
	if ro.Err != nil {
		return internal("run", runFields, ro.Err)
	}
	var checked bytes.Buffer
	if err := source.WriteJSON(&checked, ro.Diags); err != nil {
		return internal("run", runFields, err)
	}
	m := map[string]any{
		"exitCode": ro.Code,
		"check":    checked.String(),
		"status":   string(ro.Status),
		"summary":  nil,
		"trace":    nil,
		"scene":    nil,
		"failure":  nil,
	}
	if ro.Trace != nil {
		m["trace"], m["scene"] = string(ro.Trace), string(ro.Scene)
	}
	if ro.Summary != nil {
		m["summary"] = string(ro.Summary)
	}
	if f := ro.Failure; f != nil {
		d, err := compactJSON(f.Diagnostic)
		if err != nil {
			return internal("run", runFields, err)
		}
		m["failure"] = map[string]any{"kind": f.Kind, "line": f.Line, "diagnostic": d}
	}
	return ok(m)
}

// Format is synod.format(files, options): files and options are JSON text.
func Format(filesJSON, optionsJSON string) (out map[string]any) {
	defer guard("format", formatFields, &out)
	var o struct {
		Paths json.RawMessage `json:"paths"`
		Style json.RawMessage `json:"style"`
	}
	usage := func(err error) map[string]any { return failed(formatFields, "usage", err.Error()) }
	if err := decodeOptions(optionsJSON, &o); err != nil {
		return usage(err)
	}
	tr, err := decodeFiles(filesJSON)
	if err != nil {
		return usage(err)
	}
	paths, err := parsePaths(o.Paths)
	if err != nil {
		return usage(err)
	}
	rich, err := parseStyle(o.Style)
	if err != nil {
		return usage(err)
	}
	files, targets, err := tr.collect(paths)
	if err != nil {
		return usage(err)
	}
	res := driver.Check(driver.Options{Load: tr.loadConfig()}, targets)
	formatted := map[string]any{}
	var changed []string
	var diags []source.Diagnostic
	known := map[string]*source.File{}
	for _, p := range files {
		if _, done := known[p]; done {
			continue
		}
		src := tr.files[p]
		f := source.NewFile(p, []byte(src))
		known[p] = f
		text, d := pipeline.FormatFile(res, p, f, format.Format)
		if d != nil {
			diags = append(diags, *d)
			continue
		}
		formatted[p] = string(text)
		if string(text) != src {
			changed = append(changed, p)
		}
	}
	sort.Strings(changed)
	list := make([]any, len(changed))
	for i, p := range changed {
		list[i] = p
	}
	var js, text bytes.Buffer
	if err := source.WriteJSON(&js, diags); err != nil {
		return internal("format", formatFields, err)
	}
	lookup := func(f string) *source.File { return known[f] }
	if err := pipeline.WriteDiags(&text, diags, pipeline.Style{Rich: rich}, lookup); err != nil {
		return internal("format", formatFields, err)
	}
	return ok(map[string]any{
		"exitCode": exitOf(diags),
		"files":    formatted,
		"changed":  list,
		"check":    js.String(),
		"text":     text.String(),
	})
}

// Version is synod.version(). injected is the compiler version set at build
// time (-X main.version=<tag>); "dev" shows the module version if any.
func Version(injected string) (out map[string]any) {
	defer guard("version", nil, &out)
	info, _ := debug.ReadBuildInfo()
	compiler := pipeline.CompilerVersion(injected, info)
	formats := make([]any, len(pipeline.Formats))
	for i, f := range pipeline.Formats {
		formats[i] = f
	}
	return ok(map[string]any{
		"exitCode": pipeline.ExitOK,
		"api":      APIVersion,
		"language": pipeline.Language,
		"compiler": compiler,
		"formats":  formats,
		"go":       runtime.Version(),
		"text":     pipeline.VersionText(compiler),
	})
}
