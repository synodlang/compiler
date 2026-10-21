package release

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWasmAssets(t *testing.T) {
	w, j := WasmAssets("v1.2.3")
	if w != "synod-v1.2.3.wasm" || j != "synod-v1.2.3-wasm_exec.js" {
		t.Errorf("got %q %q", w, j)
	}
}

func TestFindWasmExec(t *testing.T) {
	root := t.TempDir()
	if _, err := FindWasmExec(root); err == nil {
		t.Fatal("found in an empty root")
	}
	old := filepath.Join(root, "misc", "wasm")
	if err := os.MkdirAll(old, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, n := range []string{"wasm_exec.js", "go_js_wasm_exec"} {
		if err := os.WriteFile(filepath.Join(old, n), nil, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	got, err := FindWasmExec(root)
	if err != nil || got != filepath.Join(old, "wasm_exec.js") {
		t.Errorf("misc: %q %v", got, err)
	}
	nw := filepath.Join(root, "lib", "wasm")
	if err := os.MkdirAll(nw, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nw, "wasm_exec.js"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	got, _ = FindWasmExec(root)
	if got != filepath.Join(nw, "wasm_exec.js") {
		t.Errorf("lib should win: %q", got)
	}
	got, err = FindWasmExecRunner(root)
	if err != nil || got != filepath.Join(old, "go_js_wasm_exec") {
		t.Errorf("runner: %q %v", got, err)
	}
}

func TestCheckWasmSize(t *testing.T) {
	if err := CheckWasmSize(WasmMaxBytes, WasmMaxGzipBytes); err != nil {
		t.Error(err)
	}
	if err := CheckWasmSize(WasmMaxBytes+1, 1); err == nil || !strings.Contains(err.Error(), "budget") {
		t.Errorf("raw: %v", err)
	}
	if err := CheckWasmSize(1, WasmMaxGzipBytes+1); err == nil {
		t.Error("gzip over budget accepted")
	}
}
