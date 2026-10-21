package release

import (
	"fmt"
	"os"
	"path/filepath"
)

// Size budget of the WebAssembly build: the raw synod.wasm and its gzip -9
// size. They sit about 1 MiB (raw) and 0.5 MiB (gzip) above the measured
// sizes, so growth is noticed before it reaches the site.
const (
	WasmMaxBytes     = 19 << 19 // 9.5 MiB; measured 8.42 MiB (8828838 bytes)
	WasmMaxGzipBytes = 11 << 18 // 2.75 MiB; measured 2.23 MiB (2337281 bytes)
)

// WasmAssets returns the release asset names of the WebAssembly build: the
// module and the wasm_exec.js of the Go toolchain that built it.
func WasmAssets(tag string) (wasm, js string) {
	return fmt.Sprintf("synod-%s.wasm", tag), fmt.Sprintf("synod-%s-wasm_exec.js", tag)
}

// wasmDirs lists, relative to GOROOT, where the Go toolchain keeps its
// WebAssembly support files: lib/wasm from Go 1.24, misc/wasm before.
var wasmDirs = []string{filepath.Join("lib", "wasm"), filepath.Join("misc", "wasm")}

func findInGoroot(goroot, name string) (string, error) {
	for _, d := range wasmDirs {
		p := filepath.Join(goroot, d, name)
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p, nil
		}
	}
	return "", fmt.Errorf("%s not found under %s (looked in lib/wasm and misc/wasm)", name, goroot)
}

// FindWasmExec returns the path of the toolchain's wasm_exec.js.
func FindWasmExec(goroot string) (string, error) {
	return findInGoroot(goroot, "wasm_exec.js")
}

// FindWasmExecRunner returns the path of the toolchain's go_js_wasm_exec,
// the script `go test` uses to run js/wasm test binaries under Node.
func FindWasmExecRunner(goroot string) (string, error) {
	return findInGoroot(goroot, "go_js_wasm_exec")
}

// CheckWasmSize reports an error when the raw or gzip size is over budget.
func CheckWasmSize(raw, gz int64) error {
	if raw > WasmMaxBytes {
		return fmt.Errorf("synod.wasm is %d bytes, over the budget of %d", raw, int64(WasmMaxBytes))
	}
	if gz > WasmMaxGzipBytes {
		return fmt.Errorf("synod.wasm is %d bytes gzipped, over the budget of %d", gz, int64(WasmMaxGzipBytes))
	}
	return nil
}
