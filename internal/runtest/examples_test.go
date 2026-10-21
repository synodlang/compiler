package runtest

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const goldenDir = "../../testdata/run/examples"

// TestExampleGoldens compares the trace and the summary of every example with
// the golden files (plan 5.1). The programs load with the default display
// paths (relative to the working directory) and the goldens use the trace
// paths, so the cwd-independence of the paths is exercised.
func TestExampleGoldens(t *testing.T) {
	ex := examples(t)
	if len(ex) != 9 {
		t.Fatalf("%d main programs: %v", len(ex), names(ex))
	}
	for _, n := range names(ex) {
		p := ex[n]
		t.Run(n, func(t *testing.T) {
			r := p.run(nil)
			if r.Failed != nil {
				t.Fatalf("failed: %v", r.Failed)
			}
			golden(t, filepath.Join(goldenDir, n, "trace.jsonl"), p.encode(t, r))
			golden(t, filepath.Join(goldenDir, n, "summary.txt"), []byte(p.summary(t, r)))
			golden(t, filepath.Join(goldenDir, n, "scene.json"), p.sceneBytes(t, nil))
		})
	}
}

// TestExamplesDeterministic runs every example twice with its own seed and
// with 20 other seeds: the traces and summaries are byte-identical (plan 5.2).
func TestExamplesDeterministic(t *testing.T) {
	ex := examples(t)
	for _, n := range names(ex) {
		p := ex[n]
		t.Run(n, func(t *testing.T) {
			seeds := []*uint64{nil}
			for s := uint64(1); s <= 20; s++ {
				seeds = append(seeds, u64(s))
			}
			for _, s := range seeds {
				r1, r2 := p.run(s), p.run(s)
				a, b := p.encode(t, r1), p.encode(t, r2)
				if !bytes.Equal(a, b) {
					t.Fatalf("seed %v: traces differ", s)
				}
				if p.summary(t, r1) != p.summary(t, r2) {
					t.Fatalf("seed %v: summaries differ", s)
				}
				if r1.Failed != nil {
					t.Fatalf("seed %v failed: %v", s, r1.Failed)
				}
				checkLines(t, a)
				if !bytes.Equal(p.sceneBytes(t, s), p.sceneBytes(t, s)) {
					t.Fatalf("seed %v: scenes differ", s)
				}
			}
		})
	}
}

// checkLines checks that every line of a trace is a JSON object, that seq
// counts from 1 and that sim.Failed, if present, is last.
func checkLines(t testing.TB, trace []byte) {
	t.Helper()
	if len(trace) == 0 || trace[len(trace)-1] != '\n' {
		t.Fatalf("trace does not end with a newline")
	}
	ls := strings.Split(strings.TrimSuffix(string(trace), "\n"), "\n")
	var hdr map[string]any
	if err := json.Unmarshal([]byte(ls[0]), &hdr); err != nil || hdr["format"] != "synod-trace/1" {
		t.Fatalf("bad header %q: %v", ls[0], err)
	}
	for i, l := range ls[1:] {
		var m map[string]any
		if err := json.Unmarshal([]byte(l), &m); err != nil {
			t.Fatalf("line %d: %v: %s", i+2, err, l)
		}
		if m["seq"] != float64(i+1) {
			t.Fatalf("line %d: seq %v", i+2, m["seq"])
		}
		if m["ev"] == "sim.Failed" && i != len(ls)-2 {
			t.Fatalf("sim.Failed is not the last line")
		}
	}
	for _, bad := range []string{"Infinity", "NaN", `-0,`, `-0}`, "e-0"} {
		if strings.Contains(string(trace), bad) {
			t.Errorf("trace contains %q", bad)
		}
	}
}

// TestTraceHashes writes the SHA-256 of every example trace (own seed and
// seeds 1 and 2) and of its scene.json to the file named by SYNOD_HASHES_OUT, one "name seed hash"
// line each (traces and scenes). CI runs it on Linux, macOS and Windows and diffs the files
// (plan 5.3). Without the variable it only checks the hashes are stable.
func TestTraceHashes(t *testing.T) {
	ex := examples(t)
	var out strings.Builder
	for _, n := range names(ex) {
		p := ex[n]
		for _, s := range []*uint64{nil, u64(1), u64(2)} {
			r := p.run(s)
			sum := sha256.Sum256(p.encode(t, r))
			label := "own"
			if s != nil {
				label = fmt.Sprint(*s)
			}
			fmt.Fprintf(&out, "%s seed=%s %s\n", n, label, hex.EncodeToString(sum[:]))
			ssum := sha256.Sum256(p.sceneBytes(t, s))
			fmt.Fprintf(&out, "%s seed=%s scene %s\n", n, label, hex.EncodeToString(ssum[:]))
		}
	}
	if path := os.Getenv("SYNOD_HASHES_OUT"); path != "" {
		if err := os.WriteFile(path, []byte(out.String()), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}
