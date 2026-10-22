//go:build !js

// Package wasmtest runs the WebAssembly build under Node and compares it with
// the native JS API (internal/jsapi) and with the committed goldens.
package wasmtest

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/synodlang/compiler/internal/release"
)

const (
	testdata  = "../../testdata"
	maxPerRun = 5 * time.Second
)

// request is one call of the JS API. args are the JSON values passed to the
// function; opts is nil when the call has no options argument.
type request struct {
	Fn   string `json:"fn"`
	Args []any  `json:"args"`

	label string
	// want holds golden expectations: result field -> exact text.
	want map[string]string
}

type response struct {
	Result map[string]any `json:"result"`
	Ms     float64        `json:"ms"`
}

type driverOut struct {
	Responses []response `json:"responses"`
	Shape     []string   `json:"shape"`
	Problems  []string   `json:"problems"`
}

func TestWasm(t *testing.T) {
	require := os.Getenv("SYNOD_REQUIRE_NODE") != ""
	skip := func(why string) {
		if require {
			t.Fatal(why)
		}
		t.Skip(why)
	}
	if testing.Short() && !require {
		t.Skip("-short")
	}
	node, err := exec.LookPath("node")
	if err != nil {
		skip("node is not on PATH")
	}
	goroot, err := goEnv("GOROOT")
	if err != nil {
		t.Fatal(err)
	}
	execJS, err := release.FindWasmExec(goroot)
	if err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	wasm := filepath.Join(dir, "synod.wasm")
	build := exec.Command("go", "build", "-trimpath", "-ldflags", "-s -w -X main.version=dev", "-o", wasm, "./cmd/synod-wasm")
	build.Dir = "../.."
	build.Env = append(os.Environ(), "GOOS=js", "GOARCH=wasm", "CGO_ENABLED=0")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
	raw, err := os.ReadFile(wasm)
	if err != nil {
		t.Fatal(err)
	}
	gz := gzipSize(t, raw)
	t.Logf("synod.wasm: %d bytes, %d gzipped", len(raw), gz)
	if err := release.CheckWasmSize(int64(len(raw)), int64(gz)); err != nil {
		t.Error(err)
	}

	reqs := buildRequests(t)
	reqJSON, err := json.Marshal(reqs)
	if err != nil {
		t.Fatal(err)
	}
	reqPath, respPath := filepath.Join(dir, "requests.json"), filepath.Join(dir, "responses.json")
	if err := os.WriteFile(reqPath, reqJSON, 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(node, "testdata/driver.cjs", wasm, execJS, reqPath, respPath)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("node: %v\n%s", err, out)
	}
	data, err := os.ReadFile(respPath)
	if err != nil {
		t.Fatal(err)
	}
	var got driverOut
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	for _, p := range got.Problems {
		t.Errorf("driver: %s", p)
	}
	if len(got.Responses) != len(reqs) {
		t.Fatalf("%d responses for %d requests", len(got.Responses), len(reqs))
	}

	var total float64
	for i, r := range reqs {
		resp := got.Responses[i]
		total += resp.Ms
		t.Logf("%-60s %8.1f ms", r.label, resp.Ms)
		if time.Duration(resp.Ms*float64(time.Millisecond)) > maxPerRun {
			t.Errorf("%s: %.0f ms, over %v", r.label, resp.Ms, maxPerRun)
		}
		if resp.Result == nil {
			t.Errorf("%s: no result", r.label)
			continue
		}
		want := native(t, r)
		for _, d := range diff(want, resp.Result) {
			t.Errorf("%s: wasm differs from native: %s", r.label, d)
		}
		if strings.HasPrefix(r.label, "usage ") {
			e, _ := resp.Result["error"].(map[string]any)
			if resp.Result["exitCode"] != float64(2) || e == nil || e["code"] != "usage" {
				t.Errorf("%s: want a usage error with exit code 2, got %v", r.label, resp.Result)
			}
		}
		if strings.HasPrefix(r.label, "run lookfault") {
			f, _ := resp.Result["failure"].(map[string]any)
			if resp.Result["exitCode"] != float64(1) || f == nil || f["kind"] != "look" {
				t.Errorf("%s: want a look failure with exit code 1, got %v", r.label, resp.Result["failure"])
			}
		}
		for field, golden := range r.want {
			if s, _ := resp.Result[field].(string); s != golden {
				t.Errorf("%s: %s differs from the golden\n%s", r.label, field, firstDiff(golden, s))
			}
		}
	}
	t.Logf("%d calls, %.0f ms in total", len(reqs), total)
}

func goEnv(name string) (string, error) {
	out, err := exec.Command("go", "env", name).Output()
	return strings.TrimSpace(string(out)), err
}

func gzipSize(t *testing.T, b []byte) int {
	t.Helper()
	var buf bytes.Buffer
	zw, err := gzip.NewWriterLevel(&buf, gzip.BestCompression)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := zw.Write(b); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Len()
}

// diff compares two results field by field after a JSON round trip, so ints
// and floats agree.
func diff(want, got map[string]any) []string {
	w, g := roundTrip(want), roundTrip(got)
	var keys []string
	seen := map[string]bool{}
	for k := range w {
		keys = append(keys, k)
		seen[k] = true
	}
	for k := range g {
		if !seen[k] {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	var out []string
	for _, k := range keys {
		a, _ := json.Marshal(w[k])
		b, _ := json.Marshal(g[k])
		if k == "compiler" || k == "text" && w["api"] != nil {
			continue // the native test binary has no -X main.version
		}
		if !bytes.Equal(a, b) {
			out = append(out, fmt.Sprintf("field %q:\n%s", k, firstDiff(string(a), string(b))))
		}
	}
	return out
}

func roundTrip(m map[string]any) map[string]any {
	b, _ := json.Marshal(m)
	var out map[string]any
	_ = json.Unmarshal(b, &out)
	return out
}

func firstDiff(want, got string) string {
	n := 0
	for n < len(want) && n < len(got) && want[n] == got[n] {
		n++
	}
	cut := func(s string) string {
		lo, hi := max(n-40, 0), min(n+80, len(s))
		return fmt.Sprintf("%q (len %d)", s[lo:hi], len(s))
	}
	return "  want ..." + cut(want) + "\n  got  ..." + cut(got)
}

// tree reads the files under root as slash paths, minus the named files at
// any depth.
func tree(t *testing.T, root string, skip ...string) map[string]any {
	t.Helper()
	files := map[string]any{}
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		for _, s := range skip {
			if d.Name() == s {
				return nil
			}
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, p)
		files[filepath.ToSlash(rel)] = string(b)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return files
}

func readGolden(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func subdirs(t *testing.T, dir string) []string {
	t.Helper()
	es, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, e := range es {
		if e.IsDir() {
			out = append(out, e.Name())
		}
	}
	return out
}

func buildRequests(t *testing.T) []request {
	t.Helper()
	var reqs []request
	add := func(label, fn string, want map[string]string, args ...any) {
		if args == nil {
			args = []any{}
		}
		reqs = append(reqs, request{Fn: fn, Args: args, label: label, want: want})
	}

	// Every example, own seed and seeds 1 and 2.
	exFiles := tree(t, filepath.Join(testdata, "examples"))
	goldDir := filepath.Join(testdata, "run", "examples")
	examples := subdirs(t, goldDir)
	if len(examples) != 9 {
		t.Fatalf("%d example goldens", len(examples))
	}
	for _, n := range examples {
		g := filepath.Join(goldDir, n)
		own := map[string]string{
			"trace": readGolden(t, filepath.Join(g, "trace.jsonl")),
			"scene": readGolden(t, filepath.Join(g, "scene.json")),
		}
		add("run "+n+" seed=own", "run", own, exFiles, map[string]any{"program": n})
		for _, s := range []int{1, 2} {
			add(fmt.Sprintf("run %s seed=%d", n, s), "run", nil, exFiles, map[string]any{"program": n, "seed": s})
		}
	}
	add("run raft pace=0.5", "run", nil, exFiles, map[string]any{"program": "raft", "pace": 0.5})
	add("run raft mode=proportional", "run", nil, exFiles, map[string]any{"program": "raft", "mode": "proportional"})
	add("run raft summary", "run", nil, exFiles, map[string]any{"program": "raft", "summary": true})
	add("run raft short", "run", nil, exFiles, map[string]any{"program": "raft", "style": "short"})

	// Every check case, rich and short; the first also with tokens.
	for _, code := range subdirs(t, filepath.Join(testdata, "check")) {
		for _, cs := range subdirs(t, filepath.Join(testdata, "check", code)) {
			d := filepath.Join(testdata, "check", code, cs)
			files := tree(t, d, "expect.json", "expect.txt", "expect-rich.txt")
			rich := map[string]string{
				"check": readGolden(t, filepath.Join(d, "expect.json")),
				"text":  readGolden(t, filepath.Join(d, "expect-rich.txt")),
			}
			add("check "+code+"/"+cs+" rich", "check", rich, files, map[string]any{"style": "rich"})
			short := map[string]string{"text": readGolden(t, filepath.Join(d, "expect.txt"))}
			add("check "+code+"/"+cs+" short", "check", short, files, map[string]any{"style": "short"})
			if len(reqs) < 80 {
				add("check "+code+"/"+cs+" tokens", "check", nil, files, map[string]any{"tokens": true})
			}
		}
	}

	// Run failures and cases.
	for _, kind := range []string{"errors", "cases"} {
		for _, code := range subdirs(t, filepath.Join(testdata, "run", kind)) {
			dirs := []string{filepath.Join(testdata, "run", kind, code)}
			label := kind + "/" + code
			var cases []string
			if kind == "errors" {
				cases = subdirs(t, dirs[0])
			} else {
				cases = []string{""}
			}
			for _, cs := range cases {
				d := filepath.Join(dirs[0], cs)
				files := tree(t, d, "trace.jsonl", "expect.txt", "expect-rich.txt")
				var want map[string]string
				if _, err := os.Stat(filepath.Join(d, "trace.jsonl")); err == nil {
					want = map[string]string{"trace": readGolden(t, filepath.Join(d, "trace.jsonl"))}
				}
				add("run "+filepath.Join(label, cs), "run", want, files, map[string]any{"program": "p", "style": "short"})
			}
		}
	}

	// A look fault.
	look := tree(t, "../../cmd/synod/testdata/lookfault")
	add("run lookfault short", "run", nil, look, map[string]any{"program": "main", "style": "short"})
	add("run lookfault rich", "run", nil, look, map[string]any{"program": "main"})

	// A look observer that recurses past the call-depth limit: the scene
	// builder reports run-limit instead of overflowing the JS stack.
	lookRec := map[string]any{
		"synod.mod":       "module example.com/t\nsynod 0.2\n",
		"main/main.synod": "# package main\nuse example.com/t/lk\nuse example.com/t/p\nuse std/sim\n\ndefine main\n  set a to (sim.Spawn p)\n  set a.p.Got to 1\n",
		"p/p.synod":       "# package p\nNode has\n  set Got to 0\n",
		"lk/lk.synod":     "# package lk\nuse std/sim\n\ndefine Down\n  Down\n\nwhen (sim.Added n)\n  Down\n",
	}
	add("run look recursion short", "run", nil, lookRec, map[string]any{"program": "main", "style": "short"})
	add("run look recursion rich", "run", nil, lookRec, map[string]any{"program": "main"})

	// Format.
	for _, n := range subdirs(t, filepath.Join(testdata, "misformatted")) {
		files := tree(t, filepath.Join(testdata, "misformatted", n))
		add("format misformatted/"+n, "format", nil, files)
		add("format misformatted/"+n+" short", "format", nil, files, map[string]any{"style": "short"})
	}

	add("version", "version", nil)

	// Usage errors.
	ok := map[string]any{"synod.mod": "module m\nsynod 0.2\n", "p/p.synod": "// p\n"}
	add("usage seed 1.5", "run", nil, ok, jsonRaw(`{"program":"p","seed":1.5}`))
	add("usage pace 0.1", "run", nil, ok, map[string]any{"program": "p", "pace": 0.1})
	add("usage mode x", "run", nil, ok, map[string]any{"program": "p", "mode": "x"})
	add("usage unknown option", "run", nil, ok, map[string]any{"program": "p", "colour": true})
	add("usage key with ..", "check", nil, map[string]any{"../a.synod": "x"})
	add("usage missing program", "run", nil, ok, map[string]any{})
	add("usage program not a folder", "run", nil, ok, map[string]any{"program": "nope"})
	add("usage check unknown path", "check", nil, ok, map[string]any{"paths": []string{"nope"}})
	add("usage no synod files", "check", nil, map[string]any{"a.txt": "x"})
	return reqs
}

// jsonRaw is a JSON value kept as written, so 1.5 stays 1.5.
func jsonRaw(s string) json.RawMessage { return json.RawMessage(s) }
