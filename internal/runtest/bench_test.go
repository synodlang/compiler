package runtest

import (
	"io"
	"testing"
	"time"

	"github.com/synodlang/compiler/internal/trace"
)

// ping5000 returns the benchmark program: 6 nodes, each sending a ping to a
// random peer every 35ms for 30s (about 5,100 messages).
func ping5000(t testing.TB) *program {
	t.Helper()
	m := check(t, "../../testdata/run/bench/ping5000")
	for _, p := range m {
		return p
	}
	t.Fatal("no program")
	return nil
}

// simAndEncode runs the program and encodes its trace into a discarding
// writer: the work `synod run` does after the check.
func simAndEncode(t testing.TB, p *program) (facts int) {
	r := p.run(nil)
	if r.Failed != nil {
		t.Fatal(r.Failed)
	}
	if err := trace.Encode(io.Discard, r.Header, r.Facts, p.opts()); err != nil {
		t.Fatal(err)
	}
	return len(r.Facts)
}

// buildScene builds and encodes the scene of the facts into a discarding
// writer: the work `synod run` does after the simulation.
func buildScene(t testing.TB, p *program, facts []trace.Fact) (shapes int) {
	sc, fl := p.scene(t, facts)
	if fl != nil {
		t.Fatal(fl.Fault.Error())
	}
	if err := trace.EncodeScene(io.Discard, sc); err != nil {
		t.Fatal(err)
	}
	return len(sc.Shapes)
}

// TestPerformance is a loose guard: the target is below 50ms for each
// simulation benchmark and 100ms for the scene, the test only fails above
// one second (two for the simulation and the scene together).
func TestPerformance(t *testing.T) {
	progs := map[string]*program{"ping5000": ping5000(t), "cassandra": examples(t)["cassandra"]}
	for name, p := range progs {
		start := time.Now()
		n := simAndEncode(t, p)
		d := time.Since(start)
		t.Logf("%s: %d facts in %s", name, n, d)
		if d > time.Second {
			t.Errorf("%s took %s", name, d)
		}
	}
	p := progs["ping5000"]
	r := p.run(nil)
	start := time.Now()
	n := buildScene(t, p, r.Facts)
	d := time.Since(start)
	t.Logf("ping5000 scene: %d shapes from %d facts in %s", n, len(r.Facts), d)
	if d > time.Second {
		t.Errorf("the scene took %s", d)
	}
}

func BenchmarkPing5000(b *testing.B) {
	p := ping5000(b)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		simAndEncode(b, p)
	}
}

func BenchmarkCassandra(b *testing.B) {
	p := examples(b)["cassandra"]
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		simAndEncode(b, p)
	}
}

func BenchmarkScenePing5000(b *testing.B) {
	p := ping5000(b)
	r := p.run(nil)
	if r.Failed != nil {
		b.Fatal(r.Failed)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buildScene(b, p, r.Facts)
	}
}
