package runtest

import (
	"bytes"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/synodlang/compiler/internal/driver"
	"github.com/synodlang/compiler/internal/load"
	"github.com/synodlang/compiler/internal/sim"
	"github.com/synodlang/compiler/internal/trace"
)

// Limits of the runs of the fuzz and generator tests: a program that does not
// stop by itself fails with run-limit or time-limit instead of hanging.
const (
	fuzzMaxSteps  = 50_000
	fuzzTimeLimit = 30_000_000 // 30s of simulated time
)

// runMap checks the module in fsys (rooted at /m) for the target folder and,
// if it checks clean and has a main file, runs it twice with limits. It
// asserts the properties of plan 5.8: no panic (a panic fails the caller), the
// trace encodes, the second run is byte-identical, every line is a JSON
// object and seq increases. It reports whether the program checked and ran.
func runMap(t testing.TB, fsys load.MapFS, target string) (ran bool, diags string) {
	t.Helper()
	res := driver.Check(driver.Options{Load: load.Config{FS: fsys}}, []string{target})
	if len(res.Diags) > 0 || res.Graph == nil {
		var b strings.Builder
		for _, d := range res.Diags {
			b.WriteString(d.String() + "\n")
		}
		return false, b.String()
	}
	var ip string
	for _, p := range res.Graph.Targets {
		if p.IsMain {
			ip = p.Path
		}
	}
	prog := res.Programs[ip]
	if ip == "" || prog == nil {
		return false, "no main program"
	}
	o := trace.Options{Paths: trace.FilePaths(res.Graph), Qual: trace.Qualifiers(prog)}
	var out [2][]byte
	for i := range out {
		r := sim.Run(prog, sim.Options{MaxSteps: fuzzMaxSteps, TimeLimit: fuzzTimeLimit})
		var b bytes.Buffer
		if err := trace.Encode(&b, r.Header, r.Facts, o); err != nil {
			t.Fatalf("encode: %v", err)
		}
		out[i] = b.Bytes()
		if r.Failed != nil && r.Facts[len(r.Facts)-1].Ev != "sim.Failed" {
			t.Fatalf("failure %v is not the last fact", r.Failed)
		}
	}
	if !bytes.Equal(out[0], out[1]) {
		t.Fatalf("two runs differ")
	}
	checkTrace(t, out[0])
	return true, ""
}

// checkTrace checks that every line is a JSON object, the first is the
// header and seq increases by one from 1.
func checkTrace(t testing.TB, trace []byte) {
	t.Helper()
	ls := strings.Split(strings.TrimSuffix(string(trace), "\n"), "\n")
	for i, l := range ls {
		var m map[string]any
		if err := json.Unmarshal([]byte(l), &m); err != nil {
			t.Fatalf("line %d is not JSON: %v: %s", i+1, err, l)
		}
		if i == 0 {
			if m["format"] != "synod-trace/1" {
				t.Fatalf("header %s", l)
			}
		} else if m["seq"] != float64(i) {
			t.Fatalf("line %d: seq %v", i+1, m["seq"])
		}
	}
}

// exampleFiles reads the sources of the examples module into a MapFS
// rooted at /m.
func exampleFiles(t testing.TB) load.MapFS {
	m := load.MapFS{}
	root := filepath.FromSlash(examplesDir)
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		if !strings.HasSuffix(p, ".synod") && filepath.Base(p) != "synod.mod" {
			return nil
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, p)
		m["/m/"+filepath.ToSlash(rel)] = string(b)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return m
}

// FuzzRun mutates the main file of one of the examples (the corpus is the
// demo of each example, with its library) and runs the programs that check
// clean. The input is the name of the example folder and the main file.
func FuzzRun(f *testing.F) {
	base := exampleFiles(f)
	for k, src := range base {
		if strings.HasSuffix(k, "/demo.synod") {
			dir := strings.TrimSuffix(strings.TrimPrefix(k, "/m/"), "/demo.synod")
			f.Add(dir, src)
		}
	}
	f.Fuzz(func(t *testing.T, dir, src string) {
		if len(src) > 16<<10 {
			t.Skip()
		}
		key := "/m/" + dir + "/demo.synod"
		if _, ok := base[key]; !ok {
			t.Skip()
		}
		fsys := load.MapFS{}
		for k, v := range base {
			fsys[k] = v
		}
		fsys[key] = src
		runMap(t, fsys, "/m/"+dir)
	})
}

// TestFuzzCorpus runs the seed corpus of FuzzRun as an ordinary test: every
// demo as it is.
func TestFuzzCorpus(t *testing.T) {
	base := exampleFiles(t)
	n := 0
	for k := range base {
		if !strings.HasSuffix(k, "/demo.synod") {
			continue
		}
		dir := strings.TrimSuffix(k, "/demo.synod")
		if ok, d := runMap(t, base, dir); !ok {
			t.Errorf("%s did not check and run: %s", dir, d)
		}
		n++
	}
	if n != 9 {
		t.Errorf("%d demos", n)
	}
}
