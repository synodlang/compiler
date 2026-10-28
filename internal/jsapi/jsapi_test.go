package jsapi

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

const testdata = "../../testdata"

// tree reads dir into the files argument: every file, keyed by its slash
// path relative to dir.
func readTree(t *testing.T, dir string) map[string]string {
	t.Helper()
	m := map[string]string{}
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(dir, p)
		m[filepath.ToSlash(rel)] = string(b)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func read(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func enc(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

type opts map[string]any

func checkCall(t *testing.T, files map[string]string, o opts) map[string]any {
	t.Helper()
	return Check(enc(t, files), enc(t, o))
}

func runCall(t *testing.T, files map[string]string, o opts) map[string]any {
	t.Helper()
	return Run(enc(t, files), enc(t, o))
}

func formatCall(t *testing.T, files map[string]string, o opts) map[string]any {
	t.Helper()
	return Format(enc(t, files), enc(t, o))
}

func str(t *testing.T, m map[string]any, k string) string {
	t.Helper()
	s, ok := m[k].(string)
	if !ok {
		t.Fatalf("%s is %#v, not a string", k, m[k])
	}
	return s
}

// plain checks that the map holds only what js.ValueOf accepts.
func plain(t *testing.T, path string, v any) {
	t.Helper()
	switch v := v.(type) {
	case nil, string, bool, int:
	case []any:
		for i, e := range v {
			plain(t, path+"[]", e)
			_ = i
		}
	case map[string]any:
		for k, e := range v {
			plain(t, path+"."+k, e)
		}
	default:
		t.Errorf("%s has type %T", path, v)
	}
}

func usageOnly(t *testing.T, m map[string]any, fields []string, want string) {
	t.Helper()
	plain(t, "result", m)
	if m["exitCode"] != 2 {
		t.Errorf("exitCode %v", m["exitCode"])
	}
	e, _ := m["error"].(map[string]any)
	if e == nil || e["code"] != "usage" {
		t.Fatalf("error %v", m["error"])
	}
	if !strings.Contains(e["message"].(string), want) {
		t.Errorf("message %q, want %q", e["message"], want)
	}
	for _, f := range fields {
		if m[f] != nil {
			t.Errorf("%s is %v", f, m[f])
		}
	}
}

func TestCheckGoldens(t *testing.T) {
	root := filepath.Join(testdata, "check")
	n := 0
	codes, _ := os.ReadDir(root)
	for _, c := range codes {
		cases, _ := os.ReadDir(filepath.Join(root, c.Name()))
		for _, cs := range cases {
			if !cs.IsDir() {
				continue
			}
			dir := filepath.Join(root, c.Name(), cs.Name())
			t.Run(c.Name()+"/"+cs.Name(), func(t *testing.T) {
				n++
				files := readTree(t, dir)
				rich := checkCall(t, files, nil)
				short := checkCall(t, files, opts{"style": "short"})
				plain(t, "result", rich)
				if rich["error"] != nil || rich["exitCode"] != 1 {
					t.Fatalf("exit %v, error %v", rich["exitCode"], rich["error"])
				}
				for _, g := range []struct {
					got  string
					file string
				}{
					{str(t, rich, "check"), "expect.json"},
					{str(t, rich, "text"), "expect-rich.txt"},
					{str(t, short, "check"), "expect.json"},
					{str(t, short, "text"), "expect.txt"},
				} {
					if want := files[g.file]; g.got != want {
						t.Errorf("%s differs\n--- got\n%s--- want\n%s", g.file, g.got, want)
					}
				}
			})
		}
	}
	if n < 20 {
		t.Errorf("only %d cases", n)
	}
}

func TestCheckClean(t *testing.T) {
	files := readTree(t, filepath.Join(testdata, "examples"))
	r := checkCall(t, files, nil)
	if r["exitCode"] != 0 || r["error"] != nil || r["text"] != "" {
		t.Fatalf("%v", r)
	}
	if !strings.Contains(r["check"].(string), `"ok":true`) {
		t.Errorf("check %q", r["check"])
	}
	tk := checkCall(t, files, opts{"tokens": true, "paths": []string{"raft"}})
	if !strings.Contains(str(t, tk, "check"), `"file":"raft/raft.synod","tokens":[`) {
		t.Errorf("tokens %.200s", tk["check"])
	}
	if tk["exitCode"] != 0 {
		t.Errorf("exit %v", tk["exitCode"])
	}
}

func TestCheckPaths(t *testing.T) {
	files := map[string]string{
		"synod.mod":          "module example.com/t\nsynod 0.2\n",
		"a/a.synod":          "# package a\ndefine One\n  result 1\n",
		"testdata/x/x.synod": "garbage",
		"vendor/v/v.synod":   "garbage",
		"_skip/s.synod":      "garbage",
		".hid/h.synod":       "garbage",
	}
	if r := checkCall(t, files, nil); r["exitCode"] != 0 {
		t.Errorf("default skip rules: %v", r)
	}
	// An explicit folder is walked even when its name is skipped.
	if r := checkCall(t, files, opts{"paths": []string{"testdata"}}); r["exitCode"] != 1 {
		t.Errorf("explicit testdata: %v", r)
	}
	if r := checkCall(t, files, opts{"paths": []string{"a/a.synod"}}); r["exitCode"] != 0 {
		t.Errorf("file path: %v", r)
	}
	usageOnly(t, checkCall(t, files, opts{"paths": []string{"nope"}}), checkFields, "no such file or folder: nope")
	usageOnly(t, checkCall(t, map[string]string{"synod.mod": "x"}, nil), checkFields, "no .synod files found")
	usageOnly(t, checkCall(t, map[string]string{}, nil), checkFields, "no .synod files found")
}

func TestRunExamples(t *testing.T) {
	files := readTree(t, filepath.Join(testdata, "examples"))
	goldens := filepath.Join(testdata, "run", "examples")
	ents, _ := os.ReadDir(goldens)
	if len(ents) != 9 {
		t.Fatalf("%d examples", len(ents))
	}
	for _, e := range ents {
		name := e.Name()
		t.Run(name, func(t *testing.T) {
			r := runCall(t, files, opts{"program": name, "summary": true})
			plain(t, "result", r)
			if r["error"] != nil || r["exitCode"] != 0 || r["failure"] != nil {
				t.Fatalf("exit %v error %v failure %v", r["exitCode"], r["error"], r["failure"])
			}
			for key, file := range map[string]string{"trace": "trace.jsonl", "scene": "scene.json", "summary": "summary.txt"} {
				if got, want := str(t, r, key), read(t, filepath.Join(goldens, name, file)); got != want {
					t.Errorf("%s differs from %s", key, file)
				}
			}
			if s := str(t, r, "status"); !strings.Contains(s, "-> out/trace.jsonl") || !strings.Contains(s, "-> out/scene.json") {
				t.Errorf("status %q", s)
			}
			if r["check"] != "{\"format\":\"synod-check/1\",\"ok\":true,\"diagnostics\":[]}\n" {
				t.Errorf("check %q", r["check"])
			}
		})
	}
}

func TestRunOptions(t *testing.T) {
	files := readTree(t, filepath.Join(testdata, "examples"))
	base := runCall(t, files, opts{"program": "raft"})
	if base["summary"] != nil {
		t.Error("summary without the option")
	}
	again := runCall(t, files, opts{"program": "raft", "seed": 7, "pace": 0.5, "mode": "proportional", "style": "short"})
	if again["exitCode"] != 0 || !strings.HasPrefix(str(t, again, "status"), "ok seed 7: ") {
		t.Fatalf("%v", again["status"])
	}
	if again["scene"] == base["scene"] || again["trace"] == base["trace"] {
		t.Error("options had no effect")
	}
	own := runCall(t, files, opts{"program": "raft/"})
	if own["trace"] != base["trace"] {
		t.Error("raft/ differs from raft")
	}
}

func TestRunFailures(t *testing.T) {
	root := filepath.Join(testdata, "run", "errors")
	n := 0
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || !d.IsDir() {
			return err
		}
		if _, err := os.Stat(filepath.Join(p, "trace.jsonl")); err != nil {
			return nil
		}
		n++
		rel, _ := filepath.Rel(root, p)
		t.Run(rel, func(t *testing.T) {
			files := readTree(t, p)
			rich := runCall(t, files, opts{"program": "p"})
			short := runCall(t, files, opts{"program": "p", "style": "short"})
			if rich["exitCode"] != 1 || rich["error"] != nil {
				t.Fatalf("exit %v", rich["exitCode"])
			}
			f, _ := rich["failure"].(map[string]any)
			if f == nil || f["kind"] != "run" {
				t.Fatalf("failure %v", rich["failure"])
			}
			if got := str(t, rich, "trace"); got != files["trace.jsonl"] {
				t.Errorf("trace differs")
			}
			lines := strings.Split(strings.TrimSuffix(files["expect.txt"], "\n"), "\n")
			if got := str(t, short, "status"); !strings.HasPrefix(got, lines[len(lines)-1]+"\n") {
				t.Errorf("short status %q, want %q first", got, lines[len(lines)-1])
			}
			if got := str(t, rich, "status"); got != files["expect-rich.txt"] {
				t.Errorf("rich status %q, want %q", got, files["expect-rich.txt"])
			}
			if !strings.HasSuffix(strings.TrimSuffix(files["expect.txt"], "\n"), f["line"].(string)) {
				t.Errorf("line %q", f["line"])
			}
			var d map[string]any
			if err := json.Unmarshal([]byte(f["diagnostic"].(string)), &d); err != nil || d["severity"] != "error" || d["file"] == nil {
				t.Errorf("diagnostic %v: %v", f["diagnostic"], err)
			}
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if n < 9 {
		t.Errorf("%d failure cases", n)
	}
}

func TestRunLookFault(t *testing.T) {
	files := readTree(t, "../../cmd/synod/testdata/lookfault")
	r := runCall(t, files, opts{"program": "main", "style": "short"})
	if r["exitCode"] != 1 || r["error"] != nil {
		t.Fatalf("exit %v", r["exitCode"])
	}
	f, _ := r["failure"].(map[string]any)
	if f == nil || f["kind"] != "look" || !strings.HasPrefix(f["line"].(string), "look failed at ") {
		t.Fatalf("failure %v", r["failure"])
	}
	lines := strings.Split(strings.TrimSuffix(str(t, r, "status"), "\n"), "\n")
	if len(lines) != 2 || !strings.HasPrefix(lines[0], "ok seed ") || lines[1] != f["line"] {
		t.Errorf("status %q", r["status"])
	}
	if strings.Contains(str(t, r, "trace"), "look failed") || r["scene"] == nil {
		t.Error("trace or scene wrong")
	}
	sum := runCall(t, files, opts{"program": "main", "style": "short", "summary": true})
	if !strings.HasSuffix(str(t, sum, "summary"), f["line"].(string)+"\n") {
		t.Errorf("summary lacks the look line:\n%s", sum["summary"])
	}
}

func TestRunStopped(t *testing.T) {
	files := readTree(t, filepath.Join(testdata, "check", "bad-header", "missing"))
	r := runCall(t, files, opts{"program": "p", "style": "short"})
	if r["exitCode"] != 1 || r["trace"] != nil || r["scene"] != nil || r["failure"] != nil || r["summary"] != nil {
		t.Fatalf("%v", r)
	}
	if str(t, r, "status") != files["expect.txt"] {
		t.Errorf("status %q", r["status"])
	}
	if !strings.Contains(str(t, r, "check"), `"ok":false`) {
		t.Errorf("check %q", r["check"])
	}
	// A folder without a main file.
	m := runCall(t, map[string]string{"synod.mod": "module example.com/t\nsynod 0.2\n", "p/p.synod": "# package p\ndefine One\n  result 1\n"},
		opts{"program": "p", "style": "short"})
	if m["exitCode"] != 1 || str(t, m, "status") != "p: error[missing-main]: no main file in p\n" && !strings.Contains(str(t, m, "status"), "no main file in p") {
		t.Errorf("%v", m)
	}
	if !strings.Contains(str(t, m, "check"), `"ok":false`) {
		t.Errorf("check %q", m["check"])
	}
}

func TestFormat(t *testing.T) {
	bad := filepath.Join(testdata, "misformatted")
	files := map[string]string{"synod.mod": "module github.com/synodlang/examples\nsynod 0.2\n"}
	ents, _ := os.ReadDir(bad)
	var want []string
	for _, e := range ents {
		for k, v := range readTree(t, filepath.Join(bad, e.Name())) {
			files[e.Name()+"/"+k] = v
			want = append(want, e.Name()+"/"+k)
		}
	}
	sort.Strings(want)
	r := formatCall(t, files, nil)
	plain(t, "result", r)
	if r["exitCode"] != 0 || r["error"] != nil {
		t.Fatalf("%v", r)
	}
	var changed []string
	for _, c := range r["changed"].([]any) {
		changed = append(changed, c.(string))
	}
	if !reflect.DeepEqual(changed, want) {
		t.Errorf("changed %v, want %v", changed, want)
	}
	out := r["files"].(map[string]any)
	for _, p := range want {
		name := strings.TrimSuffix(strings.SplitN(p, "/", 2)[1], ".synod")
		dir := strings.SplitN(p, "/", 2)[0]
		good := read(t, filepath.Join(testdata, "examples", dir, name+".synod"))
		if out[p] != good {
			t.Errorf("%s does not match the example", p)
		}
	}
	// Formatted text is stable: nothing changes the second time.
	clean := map[string]string{"synod.mod": files["synod.mod"]}
	for p, v := range out {
		clean[p] = v.(string)
	}
	if r2 := formatCall(t, clean, nil); len(r2["changed"].([]any)) != 0 || r2["exitCode"] != 0 {
		t.Errorf("second pass: %v", r2["changed"])
	}
}

func TestFormatDiagnostics(t *testing.T) {
	files := map[string]string{"a/a.synod": "# package a\ndefine One\n\tresult 1\n", "a/b.synod": "# package a\ndefine Two\n  result 2\n"}
	r := formatCall(t, files, opts{"style": "short"})
	plain(t, "result", r)
	if r["exitCode"] != 1 || r["error"] != nil {
		t.Fatalf("%v", r)
	}
	if _, ok := r["files"].(map[string]any)["a/a.synod"]; ok {
		t.Error("unformattable file in files")
	}
	if !strings.HasPrefix(str(t, r, "text"), "a/a.synod:") || !strings.Contains(str(t, r, "check"), `"ok":false`) {
		t.Errorf("text %q", r["text"])
	}
	if _, ok := r["files"].(map[string]any)["a/b.synod"]; !ok {
		t.Error("b missing")
	}
}

func TestVersion(t *testing.T) {
	v := Version("v1.2.3")
	plain(t, "result", v)
	if v["api"] != 1 || v["language"] != "0.3" || v["error"] != nil || v["exitCode"] != 0 {
		t.Errorf("%v", v)
	}
	if !reflect.DeepEqual(v["formats"], []any{"synod-trace/1", "synod-scene/1", "synod-check/1", "synod-doc/1", "synod-explain/1"}) {
		t.Errorf("formats %v", v["formats"])
	}
	if !strings.HasPrefix(str(t, v, "go"), "go1.") || !strings.Contains(str(t, v, "text"), "compiler "+str(t, v, "compiler")+"\n") {
		t.Errorf("%v", v)
	}
	if !strings.HasPrefix(str(t, v, "compiler"), "v1.2.3") {
		t.Errorf("compiler %q", v["compiler"])
	}
}

func TestUsageErrors(t *testing.T) {
	ex := readTree(t, filepath.Join(testdata, "examples"))
	ok1 := map[string]string{"synod.mod": "module example.com/t\nsynod 0.2\n", "p/p.synod": "# package p\n"}
	for _, c := range []struct {
		name  string
		files any
		o     any
		want  string
	}{
		{"seed fraction", ex, opts{"program": "raft", "seed": 1.5}, `seed needs a whole number from 0 to 9007199254740992, not "1.5"`},
		{"seed negative", ex, opts{"program": "raft", "seed": -1}, `not "-1"`},
		{"seed too big", ex, opts{"program": "raft", "seed": json.RawMessage("9007199254740993")}, "seed needs a whole number"},
		{"seed string", ex, opts{"program": "raft", "seed": "7"}, `not "7"`},
		{"pace low", ex, opts{"program": "raft", "pace": 0.1}, `pace needs a number from 0.25 to 4, not "0.1"`},
		{"pace digits", ex, opts{"program": "raft", "pace": 1.2345}, "pace needs a number"},
		{"pace bool", ex, opts{"program": "raft", "pace": true}, "pace needs a number"},
		{"mode", ex, opts{"program": "raft", "mode": "x"}, `mode needs ordered or proportional, not "x"`},
		{"style", ex, opts{"program": "raft", "style": "loud"}, `style needs rich or short, not "loud"`},
		{"unknown option", ex, opts{"program": "raft", "bogus": 1}, `unknown option "bogus"`},
		{"missing program", ex, opts{}, "needs a folder"},
		{"program not folder", ex, opts{"program": "nope"}, "nope is not a folder in files"},
		{"program is file", ex, opts{"program": "synod.mod"}, "synod.mod is not a folder in files"},
		{"program type", ex, opts{"program": 3}, "program must be a string"},
		{"summary type", ex, opts{"program": "raft", "summary": "yes"}, "summary must be true or false"},
		{"dotdot key", map[string]string{"a/../b.synod": "x"}, opts{"program": "a"}, `invalid file path "a/../b.synod"`},
		{"abs key", map[string]string{"/a.synod": "x"}, opts{"program": "a"}, "invalid file path"},
		{"backslash key", map[string]string{`a\b.synod`: "x"}, opts{"program": "a"}, "invalid file path"},
		{"empty key", map[string]string{"": "x"}, opts{"program": "a"}, "invalid file path"},
		{"empty segment", map[string]string{"a//b": "x"}, opts{"program": "a"}, "invalid file path"},
		{"dot segment", map[string]string{"./a": "x"}, opts{"program": "a"}, "invalid file path"},
		{"trailing slash", map[string]string{"a/": "x"}, opts{"program": "a"}, "invalid file path"},
		{"file and folder", map[string]string{"a": "x", "a/b": "y"}, opts{"program": "a"}, "both a file and a folder"},
		{"non-string value", map[string]any{"a": 1}, opts{"program": "a"}, `file "a" must be a string`},
		{"files array", []any{}, opts{"program": "a"}, "files must be an object"},
		{"files null", nil, opts{"program": "a"}, "files must be an object"},
		{"options array", ok1, []any{}, "options must be an object"},
	} {
		t.Run(c.name, func(t *testing.T) {
			r := Run(enc(t, c.files), enc(t, c.o))
			usageOnly(t, r, runFields, c.want)
			if len(r) != 2+len(runFields) {
				t.Errorf("keys %v", r)
			}
		})
	}
	usageOnly(t, Check("not json", "null"), checkFields, "files must be an object")
	usageOnly(t, Check("{}", "{bad"), checkFields, "options are not valid")
	usageOnly(t, Check(`{"a":"b"} {}`, "null"), checkFields, "unexpected data")
	usageOnly(t, checkCall(t, ok1, opts{"tokens": 1}), checkFields, "tokens must be true or false")
	usageOnly(t, checkCall(t, ok1, opts{"paths": "p"}), checkFields, "paths must be an array of strings")
	usageOnly(t, checkCall(t, ok1, opts{"program": "p"}), checkFields, `unknown option "program"`)
	usageOnly(t, formatCall(t, ok1, opts{"style": 1}), formatFields, "style must be a string")
	usageOnly(t, Usage("format", "x"), formatFields, "x")
	// options may be absent or null.
	for _, o := range []string{"", "null", "{}"} {
		if r := Check(enc(t, ok1), o); r["error"] != nil {
			t.Errorf("options %q: %v", o, r["error"])
		}
	}
}

// TestPanicRecovered runs a call that panics inside the pipeline.
func TestPanicRecovered(t *testing.T) {
	var out map[string]any
	func() {
		defer guard("run", runFields, &out)
		panic("boom")
	}()
	plain(t, "result", out)
	e := out["error"].(map[string]any)
	if out["exitCode"] != 2 || e["code"] != "internal" || e["message"] != "synod run: internal error: boom" || out["trace"] != nil {
		t.Errorf("%v", out)
	}
}

func TestVendoredModule(t *testing.T) {
	files := map[string]string{
		"synod.mod":      "module example.com/app\nsynod 0.2\nrequire example.com/dep v1.0.0\n",
		"app/main.synod": "# package app\nuse example.com/dep/lib\n\ndefine One\n  result 1\n",
		"vendor/example.com/dep@v1.0.0/synod.mod":     "module example.com/dep\nsynod 0.2\n",
		"vendor/example.com/dep@v1.0.0/lib/lib.synod": "# package lib\ndefine Two\n  result 2\n",
	}
	r := checkCall(t, files, opts{"style": "short"})
	// The import resolves from vendor/; only its being unused is reported.
	if r["error"] != nil || !strings.Contains(str(t, r, "text"), "error[unused-import]") || strings.Contains(str(t, r, "text"), "unknown-import") {
		t.Fatalf("vendored import: %v", r)
	}
}
