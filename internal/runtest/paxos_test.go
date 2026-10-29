package runtest

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/synodlang/compiler/internal/sim"
)

// countEv counts the facts of the result with the event name.
func countEv(r *sim.Result, ev string) int {
	n := 0
	for _, f := range r.Facts {
		if f.Ev == ev {
			n++
		}
	}
	return n
}

// learned returns the values of the Learned records emitted in the run.
func learned(r *sim.Result) []string {
	var out []string
	for _, f := range r.Facts {
		if f.Ev == "sim.Emitted" && f.Record.Type.Name == "Learned" {
			out = append(out, f.Record.Fields[0].Str)
		}
	}
	return out
}

// TestPaxosSeeds runs the Paxos demo with seeds 1 to 200: the safety
// invariant never breaks, nothing fails, and a value is learned for a good
// share of the seeds (plan 5.4).
func TestPaxosSeeds(t *testing.T) {
	p := examples(t)["paxos"]
	withLearned := 0
	for s := uint64(1); s <= 200; s++ {
		r := p.run(u64(s))
		if r.Failed != nil {
			t.Fatalf("seed %d failed: %v", s, r.Failed)
		}
		if n := countEv(r, "sim.Broken"); n != 0 {
			t.Fatalf("seed %d: the invariant broke %d times", s, n)
		}
		vals := learned(r)
		if len(vals) > 0 {
			withLearned++
		}
		for _, v := range vals {
			if v != vals[0] {
				t.Fatalf("seed %d learned %q and %q", s, vals[0], v)
			}
		}
	}
	t.Logf("%d of 200 seeds learned a value", withLearned)
	if withLearned == 0 {
		t.Error("no seed learned a value")
	}
}

// copyTree copies the directory src to a new temp dir and returns it.
func copyTree(t *testing.T, src string) string {
	t.Helper()
	dst := t.TempDir()
	copyTreeTo(t, src, dst)
	return dst
}

// copyTreeTo copies the directory src into the existing directory dst.
func copyTreeTo(t testing.TB, src, dst string) {
	t.Helper()
	err := filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		return os.WriteFile(target, b, 0o644)
	})
	if err != nil {
		t.Fatal(err)
	}
}

// breakPaxos removes the loop that adopts the highest prior value (and the
// variable that only it uses) from the copy of the examples in dir.
func breakPaxos(t *testing.T, dir string) {
	t.Helper()
	path := filepath.Join(dir, "paxos", "paxos.synod")
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	const loop = "      set best to 0\n" +
		"      each p in n.promises\n" +
		"        if (p.prior more than best)\n" +
		"          set best to p.prior\n" +
		"          set n.proposal to p.priorValue\n"
	if !strings.Contains(string(b), loop) {
		t.Fatal("the adopt loop is not where the test expects it")
	}
	out := strings.Replace(string(b), loop, "", 1)
	if err := os.WriteFile(path, []byte(out), 0o644); err != nil {
		t.Fatal(err)
	}
}

// brokenSeeds runs the broken Paxos of the examples copy in dir over seeds 1
// to 200 and returns how many seeds break the invariant, the first of them
// and the rule text.
func brokenSeeds(t *testing.T, dir string) (count int, first uint64, rule string) {
	t.Helper()
	p := check(t, dir)["paxos"]
	for s := uint64(1); s <= 200; s++ {
		r := p.run(u64(s))
		if r.Failed != nil {
			t.Fatalf("seed %d failed: %v", s, r.Failed)
		}
		for _, f := range r.Facts {
			if f.Ev == "sim.Broken" {
				if count == 0 {
					first, rule = s, f.Rule
				}
				count++
				break
			}
		}
	}
	return count, first, rule
}

// TestPaxosBroken patches the adopt loop out: the program still checks, and
// for some seed in 1 to 200 the "at most one chosen value" invariant breaks
// (plan 5.5). With the demo's own latency (10ms to 40ms) no seed in 1 to 200
// breaks it. With the wider spread 1ms to 40ms in a patched demo some seeds
// do, and the first of them is logged.
func TestPaxosBroken(t *testing.T) {
	dir := copyTree(t, examplesDir)
	breakPaxos(t, dir)
	n, first, _ := brokenSeeds(t, dir)
	t.Logf("broken Paxos, demo latency 10ms-40ms: %d of 200 seeds break the invariant, first %d", n, first)

	demo := filepath.Join(dir, "paxos", "demo.synod")
	b, err := os.ReadFile(demo)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "net.Latency between 10ms and 40ms") {
		t.Fatal("the demo latency is not where the test expects it")
	}
	patched := strings.Replace(string(b), "10ms and 40ms", "1ms and 40ms", 1)
	if err := os.WriteFile(demo, []byte(patched), 0o644); err != nil {
		t.Fatal(err)
	}
	n2, first2, rule := brokenSeeds(t, dir)
	t.Logf("broken Paxos, latency 1ms-40ms: %d of 200 seeds break the invariant, first %d; rule %q", n2, first2, rule)
	if n+n2 == 0 {
		t.Fatal("no seed breaks the invariant")
	}
	if n2 > 0 {
		if !strings.HasSuffix(rule, "always ((lists.Size ChosenValues) at most 1)") {
			t.Errorf("rule %q", rule)
		}
		// The trace names the rule by its module-relative path.
		p := check(t, dir)["paxos"]
		tr := string(p.encode(t, p.run(u64(first2))))
		if !strings.Contains(tr, `"rule":"paxos/paxos.synod:120: always ((lists.Size ChosenValues) at most 1)"`) {
			t.Errorf("trace of seed %d has no module-relative rule", first2)
		}
	}
}
