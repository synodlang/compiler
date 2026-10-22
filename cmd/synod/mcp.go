package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/synodlang/compiler/internal/mcp"
)

// cliRunner runs one synod command line in-process. The server uses the same
// run function as main, so each tool is exactly the CLI command it names.
type cliRunner func(args []string, stdout, stderr io.Writer) int

func defaultRunner(args []string, stdout, stderr io.Writer) int {
	return run(args, strings.NewReader(""), stdout, stderr)
}

// runMCP implements `synod mcp [--root dir]`: a stdio MCP server that runs
// until stdin closes.
func runMCP(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	root := "."
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--root" && i+1 < len(args):
			i++
			root = args[i]
		case strings.HasPrefix(a, "--root="):
			root = a[len("--root="):]
		default:
			fmt.Fprintf(stderr, "synod: mcp: unexpected argument %q\nusage: synod mcp [--root dir]\n", a)
			return exitUsage
		}
	}
	srv, err := newMCPServer(root, version, defaultRunner)
	if err != nil {
		fmt.Fprintf(stderr, "synod: mcp: %v\n", err)
		return exitUsage
	}
	if err := srv.Serve(stdin, stdout); err != nil {
		fmt.Fprintf(stderr, "synod: mcp: %v\n", err)
		return exitUsage
	}
	return exitOK
}

// cwdMu serializes the working-directory change that runs a command from the
// root.
var cwdMu sync.Mutex

type mcpTools struct {
	root string // absolute, symlinks resolved
	exec cliRunner
}

func newMCPServer(root, ver string, exec cliRunner) (*mcp.Server, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	if r, err := filepath.EvalSymlinks(abs); err == nil {
		abs = r
	}
	if fi, err := os.Stat(abs); err != nil || !fi.IsDir() {
		return nil, fmt.Errorf("root %q is not a folder", root)
	}
	m := &mcpTools{root: abs, exec: exec}
	return &mcp.Server{Name: "synod", Version: ver, Tools: m.tools()}, nil
}

const (
	readOnly   = `{"readOnlyHint":true}`
	idempotent = `{"readOnlyHint":false,"idempotentHint":true}`
)

func (m *mcpTools) tools() []mcp.Tool {
	return []mcp.Tool{
		{
			Name:        "doc",
			Description: "Show the API documentation of a package: `synod doc [package] [--scope s] --json`. Without a package it lists the module's packages.",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"package":{"type":"string","description":"Package path, for example std/net."},"scope":{"type":"string","description":"Which part of the documentation to show (the doc command's --scope)."},"json":{"type":"boolean","default":true,"description":"Machine-readable output (default true)."}},"additionalProperties":false}`),
			Annotations: json.RawMessage(readOnly),
			Call:        m.doc,
		},
		{
			Name:        "explain",
			Description: "Explain a diagnostic code: `synod explain [code] [--json]`. Without a code it lists all codes.",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"code":{"type":"string","description":"Diagnostic code, for example unknown-phrase."},"json":{"type":"boolean","default":false,"description":"Machine-readable output."}},"additionalProperties":false}`),
			Annotations: json.RawMessage(readOnly),
			Call:        m.explain,
		},
		{
			Name:        "check",
			Description: "Check files or folders for errors: `synod check <paths> [--json] --style short`. Exit code 1 means diagnostics were found, which is a normal result.",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"paths":{"type":"array","items":{"type":"string"},"default":["."],"description":"Files or folders inside the root (default the root)."},"json":{"type":"boolean","default":true,"description":"Machine-readable output (default true)."}},"additionalProperties":false}`),
			Annotations: json.RawMessage(readOnly),
			Call:        m.check,
		},
		{
			Name:        "run_summary",
			Description: "Run a simulation folder and return its summary: `synod run <folder> --summary --style short`. Nothing is written to the module.",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"folder":{"type":"string","description":"Folder inside the root that holds the main file."},"seed":{"type":"integer","minimum":0,"description":"Random seed."},"mode":{"type":"string","enum":["ordered","proportional"],"description":"Scene timing mode."}},"required":["folder"],"additionalProperties":false}`),
			Annotations: json.RawMessage(readOnly),
			Call:        m.runSummary,
		},
		{
			Name:        "fmt",
			Description: "Format files or folders in place: `synod fmt <paths> [--check] --style short`. With check it only reports files that need formatting.",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"paths":{"type":"array","items":{"type":"string"},"default":["."],"description":"Files or folders inside the root (default the root)."},"check":{"type":"boolean","default":false,"description":"Report without writing."}},"additionalProperties":false}`),
			Annotations: json.RawMessage(idempotent),
			Call:        m.fmt,
		},
	}
}

// decode parses tool arguments strictly: unknown properties and wrong types
// are -32602.
func decode(raw json.RawMessage, v any) *mcp.Error {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return mcp.InvalidParams("bad arguments: " + err.Error())
	}
	return nil
}

func boolOr(p *bool, def bool) bool {
	if p == nil {
		return def
	}
	return *p
}

// rel maps a path argument to a path relative to the root, or reports that it
// lies outside it.
func (m *mcpTools) rel(p string) (string, bool) {
	if p == "" || strings.ContainsRune(p, 0) {
		return "", false
	}
	abs := p
	if !filepath.IsAbs(p) {
		abs = filepath.Join(m.root, p)
	}
	abs = filepath.Clean(abs)
	inside := func(a string) (string, bool) {
		r, err := filepath.Rel(m.root, a)
		if err != nil || r == ".." || strings.HasPrefix(r, ".."+string(filepath.Separator)) {
			return "", false
		}
		return r, true
	}
	// Judge the path by where it really leads, so a symlink cannot escape
	// and an absolute path through a symlinked prefix (/var on macOS) works.
	if real, err := filepath.EvalSymlinks(abs); err == nil {
		abs = real
	} else if real, err := filepath.EvalSymlinks(filepath.Dir(abs)); err == nil {
		abs = filepath.Join(real, filepath.Base(abs))
	}
	return inside(abs)
}

func (m *mcpTools) rels(paths []string, def []string) ([]string, *mcp.Result) {
	if paths == nil {
		paths = def
	}
	out := make([]string, 0, len(paths))
	for _, p := range paths {
		r, ok := m.rel(p)
		if !ok {
			return nil, outside(p)
		}
		out = append(out, r)
	}
	return out, nil
}

func outside(p string) *mcp.Result {
	msg := fmt.Sprintf("outside-module: %q is not inside the root\n", p)
	return result(exitUsage, "", msg)
}

func result(code int, stdout, stderr string) *mcp.Result {
	text := stdout + stderr
	return &mcp.Result{
		Content: []mcp.Content{{Type: "text", Text: text}},
		StructuredContent: map[string]any{
			"exitCode": code,
			"stdout":   stdout,
			"stderr":   stderr,
		},
		IsError: code >= exitUsage,
	}
}

// call runs one command line from the root.
func (m *mcpTools) call(args []string) *mcp.Result {
	cwdMu.Lock()
	defer cwdMu.Unlock()
	old, err := os.Getwd()
	if err != nil {
		return result(exitUsage, "", "synod: "+err.Error()+"\n")
	}
	if err := os.Chdir(m.root); err != nil {
		return result(exitUsage, "", "synod: "+err.Error()+"\n")
	}
	defer os.Chdir(old)
	var o, e bytes.Buffer
	code := m.exec(args, &o, &e)
	return result(code, o.String(), e.String())
}

func plain(name, v string) *mcp.Error {
	if strings.HasPrefix(v, "-") {
		return mcp.InvalidParams(name + " must not start with \"-\"")
	}
	return nil
}

func (m *mcpTools) doc(raw json.RawMessage) (*mcp.Result, *mcp.Error) {
	var a struct {
		Package string `json:"package"`
		Scope   string `json:"scope"`
		JSON    *bool  `json:"json"`
	}
	if err := decode(raw, &a); err != nil {
		return nil, err
	}
	for k, v := range map[string]string{"package": a.Package, "scope": a.Scope} {
		if err := plain(k, v); err != nil {
			return nil, err
		}
	}
	args := []string{"doc"}
	if a.Package != "" {
		args = append(args, a.Package)
	}
	if a.Scope != "" {
		args = append(args, "--scope", a.Scope)
	}
	if boolOr(a.JSON, true) {
		args = append(args, "--json")
	}
	return m.call(args), nil
}

func (m *mcpTools) explain(raw json.RawMessage) (*mcp.Result, *mcp.Error) {
	var a struct {
		Code string `json:"code"`
		JSON *bool  `json:"json"`
	}
	if err := decode(raw, &a); err != nil {
		return nil, err
	}
	if err := plain("code", a.Code); err != nil {
		return nil, err
	}
	args := []string{"explain"}
	if a.Code != "" {
		args = append(args, a.Code)
	}
	if boolOr(a.JSON, false) {
		args = append(args, "--json")
	}
	return m.call(args), nil
}

func (m *mcpTools) check(raw json.RawMessage) (*mcp.Result, *mcp.Error) {
	var a struct {
		Paths []string `json:"paths"`
		JSON  *bool    `json:"json"`
	}
	if err := decode(raw, &a); err != nil {
		return nil, err
	}
	paths, bad := m.rels(a.Paths, []string{"."})
	if bad != nil {
		return bad, nil
	}
	args := []string{"check"}
	if boolOr(a.JSON, true) {
		args = append(args, "--json")
	}
	args = append(args, "--style", "short", "--")
	return m.call(append(args, paths...)), nil
}

func (m *mcpTools) fmt(raw json.RawMessage) (*mcp.Result, *mcp.Error) {
	var a struct {
		Paths []string `json:"paths"`
		Check *bool    `json:"check"`
	}
	if err := decode(raw, &a); err != nil {
		return nil, err
	}
	paths, bad := m.rels(a.Paths, []string{"."})
	if bad != nil {
		return bad, nil
	}
	args := []string{"fmt"}
	if boolOr(a.Check, false) {
		args = append(args, "--check")
	}
	args = append(args, "--style", "short", "--")
	return m.call(append(args, paths...)), nil
}

func (m *mcpTools) runSummary(raw json.RawMessage) (*mcp.Result, *mcp.Error) {
	var a struct {
		Folder *string      `json:"folder"`
		Seed   *json.Number `json:"seed"`
		Mode   *string      `json:"mode"`
	}
	if err := decode(raw, &a); err != nil {
		return nil, err
	}
	if a.Folder == nil || *a.Folder == "" {
		return nil, mcp.InvalidParams("bad arguments: folder is required")
	}
	if a.Mode != nil && *a.Mode != "ordered" && *a.Mode != "proportional" {
		return nil, mcp.InvalidParams("bad arguments: mode must be ordered or proportional")
	}
	seed := ""
	if a.Seed != nil {
		if _, err := strconv.ParseUint(a.Seed.String(), 10, 64); err != nil {
			return nil, mcp.InvalidParams("bad arguments: seed must be a whole number, 0 or more")
		}
		seed = a.Seed.String()
	}
	folder, ok := m.rel(*a.Folder)
	if !ok {
		return outside(*a.Folder), nil
	}
	tmp, err := os.MkdirTemp("", "synod-mcp-")
	if err != nil {
		return result(exitUsage, "", "synod: "+err.Error()+"\n"), nil
	}
	defer os.RemoveAll(tmp)
	args := []string{"run", folder, "--summary", "--style", "short",
		"--trace", filepath.Join(tmp, "t.jsonl"), "--scene", filepath.Join(tmp, "s.json")}
	if seed != "" {
		args = append(args, "--seed", seed)
	}
	if a.Mode != nil {
		args = append(args, "--mode", *a.Mode)
	}
	return m.call(args), nil
}
