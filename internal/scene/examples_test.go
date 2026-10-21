package scene

import (
	"bytes"
	"testing"
	"time"

	"github.com/synodlang/compiler/internal/sim"
	"github.com/synodlang/compiler/internal/trace"
)

// decoded encodes the facts as trace.jsonl and decodes them again.
func (p *prog) decoded(t testing.TB, h trace.Header, facts []trace.Fact) []trace.Fact {
	t.Helper()
	var b bytes.Buffer
	if err := trace.Encode(&b, h, facts, p.opts()); err != nil {
		t.Fatal(err)
	}
	var extra = p.Prog.Packages
	if p.Look != nil {
		extra = append(append(extra[:0:0], extra...), p.Look.Packages...)
	}
	_, out, err := trace.Decode(&b, p.Prog, extra...)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// TestExamplesBuild builds the scene of every example at its own seed: no
// look fault, the same bytes when built twice and when built from the decoded
// trace.
func TestExamplesBuild(t *testing.T) {
	ex := examples(t)
	for _, n := range names(ex) {
		p := ex[n]
		t.Run(n, func(t *testing.T) {
			r := p.run()
			if r.Failed != nil {
				t.Fatalf("failed: %v", r.Failed)
			}
			start := time.Now()
			sc, fl := Build(p.input(r.Facts), Options{})
			took := time.Since(start)
			if fl != nil {
				t.Fatalf("look failed at fact %d: %v", fl.Seq, fl.Fault.Error())
			}
			checkScene(t, sc)
			checkSlots(t, r.Facts, sc, Options{})
			a := encodeScene(t, sc)
			t.Logf("%s: %d shapes, duration %dms, %d facts, %d bytes, built in %v", n, len(sc.Shapes), sc.Duration, len(r.Facts), len(a), took)
			sc2, fl := Build(p.input(r.Facts), Options{})
			if fl != nil {
				t.Fatal(fl.Fault)
			}
			if !bytes.Equal(a, encodeScene(t, sc2)) {
				t.Error("two builds differ")
			}
			sc3, fl := Build(p.input(p.decoded(t, r.Header, r.Facts)), Options{})
			if fl != nil {
				t.Fatal(fl.Fault)
			}
			if !bytes.Equal(a, encodeScene(t, sc3)) {
				t.Error("Build(facts) differs from Build(Decode(Encode(facts)))")
			}
		})
	}
}

// checkSlots verifies the scheduling rules on the slots of a scene (plan
// 5.5): the first fact is at 0, slots never go back, and in ordered mode a
// slot is at least the scaled idle gap and the minimum step after the
// previous one, and a landing is at least the scaled flight time after its
// send.
func checkSlots(t testing.TB, facts []trace.Fact, sc *trace.Scene, o Options) {
	t.Helper()
	pace := int64(o.Pace)
	if pace == 0 {
		pace = 1000
	}
	pace = max(250, min(4000, pace))
	scale := func(ms int64) int64 { return ms * 1000 / pace }
	if len(sc.Slots) > len(facts) {
		t.Errorf("%d slots for %d facts", len(sc.Slots), len(facts))
		return
	}
	sent := map[int]int64{}
	for i, e := range sc.Slots {
		f := facts[i]
		if i == 0 {
			if e.At != 0 {
				t.Errorf("first slot is %d", e.At)
			}
		} else {
			prev := sc.Slots[i-1].At
			if e.At < prev {
				t.Errorf("slot %d goes back", i+1)
			}
			if o.Mode == Ordered {
				gap := min((f.T-facts[i-1].T)*GapScale/1000, MaxIdle)
				if e.At-prev < scale(gap) {
					t.Errorf("slot %d: gap %d is less than the scaled idle gap %d", i+1, e.At-prev, scale(gap))
				}
				if e.At-prev < scale(MinStep) {
					t.Errorf("slot %d: step %d is less than %d", i+1, e.At-prev, scale(MinStep))
				}
			}
		}
		switch f.Ev {
		case "net.Sent":
			sent[f.MsgID] = e.At
		case "net.Arrived", "net.Dropped":
			if at, ok := sent[f.MsgID]; ok && o.Mode == Ordered && e.At-at < scale(MinFlight) {
				t.Errorf("slot %d: lands %d after its send, less than %d", i+1, e.At-at, scale(MinFlight))
			}
		}
	}
}

// TestExamplesSeeds builds every example at twenty other seeds and at the
// extreme paces and in proportional mode: no look fault, valid scenes, the
// same bytes when built twice (plan 5.3).
func TestExamplesSeeds(t *testing.T) {
	ex := examples(t)
	for _, n := range names(ex) {
		p := ex[n]
		t.Run(n, func(t *testing.T) {
			for seed := uint64(1); seed <= 20; seed++ {
				s := seed
				r := sim.Run(p.Prog, sim.Options{Seed: &s})
				if r.Failed != nil {
					t.Fatalf("seed %d failed: %v", seed, r.Failed)
				}
				o := Options{}
				switch seed % 5 {
				case 1:
					o.Pace = 250
				case 2:
					o.Pace = 4000
				case 3:
					o.Mode = Proportional
				case 4:
					o.Pace, o.Mode = 2000, Proportional
				}
				sc, fl := Build(p.input(r.Facts), o)
				if fl != nil {
					t.Fatalf("seed %d: look failed at fact %d: %v", seed, fl.Seq, fl.Fault)
				}
				checkScene(t, sc)
				checkSlots(t, r.Facts, sc, o)
				sc2, _ := Build(p.input(r.Facts), o)
				if !bytes.Equal(encodeScene(t, sc), encodeScene(t, sc2)) {
					t.Fatalf("seed %d: two builds differ", seed)
				}
			}
		})
	}
}
