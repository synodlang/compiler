//go:build !js

package wasmtest

import (
	"encoding/json"
	"testing"

	"github.com/synodlang/compiler/internal/jsapi"
)

// native calls the JS API in Go on the same arguments the glue would pass:
// each argument as JSON text, "null" for a missing options object.
func native(t *testing.T, r request) map[string]any {
	t.Helper()
	arg := func(i int) string {
		if i >= len(r.Args) || r.Args[i] == nil {
			return "null"
		}
		b, err := json.Marshal(r.Args[i])
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	switch r.Fn {
	case "check":
		return jsapi.Check(arg(0), arg(1))
	case "run":
		return jsapi.Run(arg(0), arg(1))
	case "format":
		return jsapi.Format(arg(0), arg(1))
	case "version":
		return jsapi.Version("dev")
	}
	t.Fatalf("unknown function %q", r.Fn)
	return nil
}
