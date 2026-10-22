package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/synodlang/compiler/internal/mcp"
)

// mcpClient is a deterministic client that drives a server over io.Pipe.
type mcpClient struct {
	t    *testing.T
	in   io.WriteCloser
	out  *bufio.Reader
	next int
	done chan struct{}
}

func startMCP(t *testing.T, root string) *mcpClient {
	t.Helper()
	srv, err := newMCPServer(root, "test", defaultRunner)
	if err != nil {
		t.Fatal(err)
	}
	return startServer(t, srv)
}

func startServer(t *testing.T, srv *mcp.Server) *mcpClient {
	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	c := &mcpClient{t: t, in: inW, out: bufio.NewReader(outR), done: make(chan struct{})}
	go func() {
		srv.Serve(inR, outW)
		outW.Close()
		close(c.done)
	}()
	t.Cleanup(c.close)
	return c
}

func (c *mcpClient) close() {
	c.in.Close()
	<-c.done
}

// raw sends one line and returns the one response line.
func (c *mcpClient) raw(line string) string {
	c.t.Helper()
	if _, err := io.WriteString(c.in, line+"\n"); err != nil {
		c.t.Fatal(err)
	}
	resp, err := c.out.ReadString('\n')
	if err != nil {
		c.t.Fatalf("no response to %s: %v", line, err)
	}
	return strings.TrimSuffix(resp, "\n")
}

type rpcReply struct {
	Result json.RawMessage `json:"result"`
	Error  *mcp.Error      `json:"error"`
}

func (c *mcpClient) rpc(method string, params any) rpcReply {
	c.t.Helper()
	c.next++
	b, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": c.next, "method": method, "params": params})
	var r rpcReply
	if err := json.Unmarshal([]byte(c.raw(string(b))), &r); err != nil {
		c.t.Fatal(err)
	}
	return r
}

type toolReply struct {
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	Structured struct {
		ExitCode int    `json:"exitCode"`
		Stdout   string `json:"stdout"`
		Stderr   string `json:"stderr"`
	} `json:"structuredContent"`
	IsError bool `json:"isError"`
}

func (c *mcpClient) tool(name string, args any) (toolReply, *mcp.Error) {
	c.t.Helper()
	r := c.rpc("tools/call", map[string]any{"name": name, "arguments": args})
	var tr toolReply
	if r.Error != nil {
		return tr, r.Error
	}
	if err := json.Unmarshal(r.Result, &tr); err != nil {
		c.t.Fatal(err)
	}
	if len(tr.Content) != 1 || tr.Content[0].Type != "text" ||
		tr.Content[0].Text != tr.Structured.Stdout+tr.Structured.Stderr {
		c.t.Fatalf("content and structuredContent disagree: %+v", tr)
	}
	return tr, nil
}

var tracePath = regexp.MustCompile(`full trace: [^)]*\)`)

func examplesRoot(t *testing.T) string {
	t.Helper()
	abs, err := filepath.Abs(examples)
	if err != nil {
		t.Fatal(err)
	}
	return abs
}

// cli runs the CLI directly from root, as a shell would.
func cli(t *testing.T, root string, args ...string) (int, string, string) {
	t.Helper()
	old, _ := os.Getwd()
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(old)
	return exec(args...)
}

func TestMCPGolden(t *testing.T) {
	root := examplesRoot(t)
	c := startMCP(t, root)
	init := func(v string) string {
		return `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"` + v + `","capabilities":{},"clientInfo":{"name":"golden","version":"1"}}}`
	}
	reqs := []string{
		init("2025-11-25"), init("2025-06-18"), init("2025-03-26"), init("2024-11-05"), init("not-a-version"),
		`{"jsonrpc":"2.0","id":2,"method":"ping"}`,
		`{"jsonrpc":"2.0","id":3,"method":"tools/list"}`,
		`{"jsonrpc":"2.0","id":4,"method":"resources/list"}`,
		`{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"run_summary","arguments":{"folder":"gossip","mode":"fast"}}}`,
		`{"jsonrpc":"2.0","id":6,"method":"tools/call","params":{"name":"check","arguments":{"paths":["../x"]}}}`,
	}
	var got strings.Builder
	for _, r := range reqs {
		got.WriteString(r + "\n" + c.raw(r) + "\n")
	}
	golden := filepath.Join("testdata", "mcp", "session.jsonl")
	if os.Getenv("SYNOD_UPDATE_GOLDEN") != "" {
		if err := os.MkdirAll(filepath.Dir(golden), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(golden, []byte(got.String()), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatal(err)
	}
	if string(want) != got.String() {
		t.Errorf("session differs from %s (rerun with SYNOD_UPDATE_GOLDEN=1 to update)\n got:\n%s\nwant:\n%s", golden, got.String(), want)
	}
}

func TestMCPProtocol(t *testing.T) {
	c := startMCP(t, examplesRoot(t))
	if got := c.raw(`garbage`); !strings.Contains(got, `"code":-32700`) {
		t.Errorf("garbage: %s", got)
	}
	if r := c.rpc("prompts/list", nil); r.Error == nil || r.Error.Code != -32601 {
		t.Errorf("unknown method: %+v", r)
	}
	// Notifications get no response: the next response must be the ping's.
	io.WriteString(c.in, `{"jsonrpc":"2.0","method":"notifications/initialized"}`+"\n")
	io.WriteString(c.in, `{"jsonrpc":"2.0","method":"notifications/cancelled","params":{"requestId":1,"reason":"x"}}`+"\n")
	if got := c.raw(`{"jsonrpc":"2.0","id":"p","method":"ping"}`); got != `{"jsonrpc":"2.0","id":"p","result":{}}` {
		t.Errorf("ping after notifications: %s", got)
	}
}

func TestMCPToolsMatchCLI(t *testing.T) {
	needReal(t)
	root := examplesRoot(t)
	c := startMCP(t, root)

	same := func(name string, args any, cliArgs ...string) toolReply {
		t.Helper()
		tr, rerr := c.tool(name, args)
		if rerr != nil {
			t.Fatalf("%s: %+v", name, rerr)
		}
		code, out, errs := cli(t, root, cliArgs...)
		// The summary names the trace file, whose folder differs per run.
		tr.Structured.Stdout = tracePath.ReplaceAllString(tr.Structured.Stdout, "full trace: TRACE)")
		out = tracePath.ReplaceAllString(out, "full trace: TRACE)")
		if tr.Structured.ExitCode != code || tr.Structured.Stdout != out || tr.Structured.Stderr != errs {
			t.Errorf("%s differs from CLI %v:\n mcp: %+v\n cli: %d %q %q", name, cliArgs, tr.Structured, code, out, errs)
		}
		if tr.IsError != (code >= 2) {
			t.Errorf("%s isError = %v for exit %d", name, tr.IsError, code)
		}
		return tr
	}

	same("check", map[string]any{}, "check", "--json", "--style", "short", "--", ".")
	same("check", map[string]any{"paths": []string{"gossip"}, "json": false}, "check", "--style", "short", "--", "gossip")
	same("fmt", map[string]any{"check": true}, "fmt", "--check", "--style", "short", "--", ".")

	tmp := t.TempDir()
	tr := same("run_summary", map[string]any{"folder": "gossip", "seed": 7, "mode": "ordered"},
		"run", "gossip", "--summary", "--style", "short", "--trace", filepath.Join(tmp, "t.jsonl"),
		"--scene", filepath.Join(tmp, "s.json"), "--seed", "7", "--mode", "ordered")
	if tr.Structured.ExitCode != 0 || tr.Structured.Stdout == "" {
		t.Errorf("run_summary: %+v", tr.Structured)
	}
}

// doc and explain give the same text as the CLI.
func TestMCPDocExplain(t *testing.T) {
	root := examplesRoot(t)
	c := startMCP(t, root)
	for _, tc := range []struct {
		name string
		args map[string]any
		cli  []string
	}{
		{"doc", map[string]any{}, []string{"doc", "--json"}},
		{"doc", map[string]any{"json": false}, []string{"doc"}},
		{"explain", map[string]any{"code": "unknown-phrase"}, []string{"explain", "unknown-phrase"}},
		{"explain", map[string]any{"json": true}, []string{"explain", "--json"}},
	} {
		tr, rerr := c.tool(tc.name, tc.args)
		if rerr != nil {
			t.Fatalf("%s: %+v", tc.name, rerr)
		}
		code, out, errs := cli(t, root, tc.cli...)
		if tr.Structured.ExitCode != code || tr.Structured.Stdout != out || tr.Structured.Stderr != errs {
			t.Errorf("%s %v differs from CLI:\n mcp: %+v\n cli: %d %q %q", tc.name, tc.args, tr.Structured, code, out, errs)
		}
	}
}

func TestMCPCheckErrorIsNormal(t *testing.T) {
	needReal(t)
	root := t.TempDir()
	write(t, filepath.Join(root, "synod.mod"), "module example.com/m\nsynod 0.2\n")
	write(t, filepath.Join(root, "bad", "bad.synod"), "this is not synod\n")
	c := startMCP(t, root)
	tr, rerr := c.tool("check", map[string]any{"paths": []string{"bad"}})
	if rerr != nil {
		t.Fatal(rerr)
	}
	if tr.Structured.ExitCode != 1 || tr.IsError || tr.Structured.Stdout == "" {
		t.Errorf("check on a bad file: %+v isError=%v", tr.Structured, tr.IsError)
	}
}

func TestMCPBadArguments(t *testing.T) {
	c := startMCP(t, examplesRoot(t))
	for _, tc := range []struct {
		name string
		args any
	}{
		{"run_summary", map[string]any{"folder": "gossip", "mode": "fast"}},
		{"run_summary", map[string]any{}},
		{"run_summary", map[string]any{"folder": "gossip", "seed": -1}},
		{"run_summary", map[string]any{"folder": "gossip", "seed": 1.5}},
		{"check", map[string]any{"paths": "gossip"}},
		{"check", map[string]any{"extra": true}},
		{"fmt", map[string]any{"check": "yes"}},
		{"explain", map[string]any{"code": "--json"}},
		{"nope", map[string]any{}},
	} {
		if _, rerr := c.tool(tc.name, tc.args); rerr == nil || rerr.Code != mcp.CodeInvalidParams {
			t.Errorf("%s %v: want -32602, got %+v", tc.name, tc.args, rerr)
		}
	}
}

func TestMCPOutsideModule(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "synod.mod"), "module example.com/m\nsynod 0.2\n")
	outsideDir := t.TempDir()
	if err := os.Symlink(outsideDir, filepath.Join(root, "link")); err != nil {
		t.Log("no symlinks:", err)
	}
	c := startMCP(t, root)
	for _, tc := range []struct {
		name string
		args map[string]any
	}{
		{"check", map[string]any{"paths": []string{"../x"}}},
		{"check", map[string]any{"paths": []string{"a/../../x"}}},
		{"check", map[string]any{"paths": []string{outsideDir}}},
		{"check", map[string]any{"paths": []string{"link"}}},
		{"fmt", map[string]any{"paths": []string{".."}}},
		{"run_summary", map[string]any{"folder": "../x"}},
	} {
		tr, rerr := c.tool(tc.name, tc.args)
		if rerr != nil || !tr.IsError || tr.Structured.ExitCode != 2 || !strings.Contains(tr.Structured.Stderr, "outside-module") {
			t.Errorf("%s %v: %+v %+v", tc.name, tc.args, tr, rerr)
		}
	}
	// An absolute path inside the root is fine.
	if tr, rerr := c.tool("check", map[string]any{"paths": []string{root}}); rerr != nil || strings.Contains(tr.Structured.Stderr, "outside-module") {
		t.Errorf("absolute path inside root: %+v %+v", tr, rerr)
	}
}

func TestMCPRunSummaryLeavesNothing(t *testing.T) {
	needReal(t)
	root := t.TempDir()
	copyTree(t, examplesRoot(t), root)
	tmp := t.TempDir()
	t.Setenv("TMPDIR", tmp)
	c := startMCP(t, root)
	tr, rerr := c.tool("run_summary", map[string]any{"folder": "gossip"})
	if rerr != nil || tr.Structured.ExitCode != 0 {
		t.Fatalf("%+v %+v", tr, rerr)
	}
	if _, err := os.Stat(filepath.Join(root, "out")); err == nil {
		t.Error("run_summary created out/")
	}
	if left, _ := os.ReadDir(tmp); len(left) != 0 {
		t.Errorf("temp folder left behind: %v", left)
	}
}

func TestRunMCPUsage(t *testing.T) {
	var o, e bytes.Buffer
	if code := runMCP([]string{"--bogus"}, strings.NewReader(""), &o, &e); code != exitUsage {
		t.Errorf("bogus flag: exit %d", code)
	}
	if code := runMCP([]string{"--root", filepath.Join(t.TempDir(), "missing")}, strings.NewReader(""), &o, &e); code != exitUsage {
		t.Errorf("missing root: exit %d", code)
	}
	o.Reset()
	in := `{"jsonrpc":"2.0","id":1,"method":"ping"}` + "\n"
	if code := runMCP([]string{"--root=" + examplesRoot(t)}, strings.NewReader(in), &o, &e); code != exitOK || o.String() != `{"jsonrpc":"2.0","id":1,"result":{}}`+"\n" {
		t.Errorf("session: exit %d out %q", code, o.String())
	}
}

func copyTree(t *testing.T, src, dst string) {
	t.Helper()
	err := filepath.Walk(src, func(p string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		if fi.IsDir() {
			return os.MkdirAll(filepath.Join(dst, rel), 0o755)
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(dst, rel), b, 0o644)
	})
	if err != nil {
		t.Fatal(err)
	}
}
