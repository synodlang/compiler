//go:build js && wasm

// Command synod-wasm is the WebAssembly build of the compiler. It sets the
// global synod = {check, run, format, version} and waits; see internal/jsapi
// for what the functions take and return. The functions are synchronous and
// never throw: bad arguments give an error result with exit code 2.
package main

import (
	"fmt"
	"syscall/js"

	"github.com/synodlang/compiler/internal/jsapi"
)

// version is the compiler version, set with -ldflags "-X main.version=...".
var version = "dev"

func main() {
	api := js.Global().Get("Object").New()
	api.Set("check", js.FuncOf(func(_ js.Value, args []js.Value) any { return call("check", args, jsapi.Check) }))
	api.Set("run", js.FuncOf(func(_ js.Value, args []js.Value) any { return call("run", args, jsapi.Run) }))
	api.Set("format", js.FuncOf(func(_ js.Value, args []js.Value) any { return call("format", args, jsapi.Format) }))
	api.Set("version", js.FuncOf(func(_ js.Value, _ []js.Value) any { return convert(jsapi.Version(version), nil) }))
	js.Global().Set("synod", api)
	select {}
}

// call runs fn on the JSON of files and options. It never panics.
func call(name string, args []js.Value, fn func(files, options string) map[string]any) (ret any) {
	defer func() {
		if r := recover(); r != nil {
			ret = convert(jsapi.Usage(name, fmt.Sprintf("the arguments could not be read: %v", r)), nil)
		}
	}()
	var files, options js.Value
	if len(args) > 0 {
		files = args[0]
	}
	if len(args) > 1 {
		options = args[1]
	}
	fj, msg := filesJSON(files)
	if msg != "" {
		return convert(jsapi.Usage(name, msg), nil)
	}
	oj, msg := optionsJSON(options)
	if msg != "" {
		return convert(jsapi.Usage(name, msg), nil)
	}
	return convert(fn(fj, oj), jsapi.Usage(name, "internal error converting the result"))
}

// isPlainObject reports whether v is an object that is neither null, an
// array nor a function.
func isPlainObject(v js.Value) bool {
	if v.Type() != js.TypeObject {
		return false
	}
	return !js.Global().Get("Array").Call("isArray", v).Bool()
}

// filesJSON checks files (an object of strings) and returns its JSON.
func filesJSON(v js.Value) (string, string) {
	if !isPlainObject(v) {
		return "", "files must be an object mapping paths to source text"
	}
	keys := js.Global().Get("Object").Call("keys", v)
	for i := 0; i < keys.Length(); i++ {
		k := keys.Index(i).String()
		if v.Get(k).Type() != js.TypeString {
			return "", fmt.Sprintf("file %q must be a string", k)
		}
	}
	return stringify(v, "files")
}

// optionsJSON returns the JSON of options; undefined and null mean none.
func optionsJSON(v js.Value) (string, string) {
	switch v.Type() {
	case js.TypeUndefined, js.TypeNull:
		return "null", ""
	}
	if !isPlainObject(v) {
		return "", "options must be an object"
	}
	return stringify(v, "options")
}

// stringify is JSON.stringify; a throwing getter, toJSON or a cycle is a
// usage error.
func stringify(v js.Value, what string) (s string, msg string) {
	defer func() {
		if r := recover(); r != nil {
			s, msg = "", fmt.Sprintf("%s cannot be converted to JSON: %v", what, r)
		}
	}()
	r := js.Global().Get("JSON").Call("stringify", v)
	if r.Type() != js.TypeString {
		return "", what + " cannot be converted to JSON"
	}
	return r.String(), ""
}

// convert is js.ValueOf; if it panics the fallback result is converted
// instead.
func convert(m, fallback map[string]any) (ret any) {
	defer func() {
		if r := recover(); r != nil {
			if fallback == nil {
				fallback = map[string]any{"exitCode": 2, "error": map[string]any{"code": "internal", "message": fmt.Sprint(r)}}
			}
			ret = js.ValueOf(fallback)
		}
	}()
	return js.ValueOf(m)
}
