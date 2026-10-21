// Command wasm builds the WebAssembly release assets: synod-<tag>.wasm and
// the matching synod-<tag>-wasm_exec.js from the same Go toolchain.
//
//	go run ./internal/release/cmd/wasm -tag v0.3.0 -out dist
package main

import (
	"bytes"
	"compress/gzip"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/synodlang/compiler/internal/release"
)

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "wasm:", err)
		os.Exit(1)
	}
}

func run(args []string, out io.Writer) error {
	fs := flag.NewFlagSet("wasm", flag.ContinueOnError)
	tag := fs.String("tag", "dev", "release tag, compiled into the module's version")
	dir := fs.String("out", "dist", "output directory")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if err := os.MkdirAll(*dir, 0o755); err != nil {
		return err
	}
	wasmName, jsName := release.WasmAssets(*tag)
	wasmPath := filepath.Join(*dir, wasmName)

	b, err := exec.Command("go", "env", "GOROOT").Output()
	if err != nil {
		return fmt.Errorf("go env GOROOT: %w", err)
	}
	goroot := strings.TrimSpace(string(b))
	execJS, err := release.FindWasmExec(goroot)
	if err != nil {
		return err
	}

	cmd := exec.Command("go", "build", "-trimpath",
		"-ldflags", "-s -w -X main.version="+*tag,
		"-o", wasmPath, "./cmd/synod-wasm")
	cmd.Env = append(os.Environ(), "GOOS=js", "GOARCH=wasm", "CGO_ENABLED=0")
	cmd.Stdout, cmd.Stderr = os.Stderr, os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("go build: %w", err)
	}
	js, err := os.ReadFile(execJS)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(*dir, jsName), js, 0o644); err != nil {
		return err
	}

	raw, err := os.ReadFile(wasmPath)
	if err != nil {
		return err
	}
	var gz bytes.Buffer
	zw, _ := gzip.NewWriterLevel(&gz, gzip.BestCompression)
	if _, err := zw.Write(raw); err != nil {
		return err
	}
	if err := zw.Close(); err != nil {
		return err
	}
	fmt.Fprintf(out, "%s: %d bytes, %d gzipped\n", wasmPath, len(raw), gz.Len())
	fmt.Fprintf(out, "%s: from %s\n", filepath.Join(*dir, jsName), execJS)
	return release.CheckWasmSize(int64(len(raw)), int64(gz.Len()))
}
