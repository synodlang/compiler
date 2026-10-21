package scene

import (
	"strings"
	"testing"

	"github.com/synodlang/compiler/internal/ir"
	"github.com/synodlang/compiler/internal/stdlib"
	"github.com/synodlang/compiler/internal/value"
)

func TestMask(t *testing.T) {
	pkg := &ir.Package{Path: "p", Name: "p"}
	inner := &ir.Record{Pkg: pkg, Name: "In", Fields: []*ir.Field{{Name: "Shown", Exported: true}, {Name: "hidden"}}}
	outer := &ir.Record{Pkg: pkg, Name: "Out", Fields: []*ir.Field{{Name: "Items", Exported: true}, {Name: "secret"}, {Name: "M", Exported: true}}}
	in := value.RecordOf(inner, []value.Value{value.Number(1), value.Number(2)})
	m := (&value.Map{}).With(value.Text("k"), in)
	v := value.RecordOf(outer, []value.Value{value.ListOf([]value.Value{in}), value.Number(3), value.MapValue(m)})
	got := mask(v)
	want := `p.Out{Items {p.In{Shown 1, hidden "…"}}, secret "…", M {"k": p.In{Shown 1, hidden "…"}}}`
	if s := value.Format(got, value.FormatOpts{ShowUnexported: true}); s != want {
		t.Errorf("mask = %s, want %s", s, want)
	}
	if got.Record().Fields[1].K != value.KText || got.Record().Fields[1].Str != "…" {
		t.Errorf("unexported field is %v", got.Record().Fields[1])
	}
	// a message id survives, and the original is untouched
	id := value.Value{K: value.KRecord, Ref: in.Record(), Int: 7}
	if mask(id).Int != 7 {
		t.Error("mask lost the message id")
	}
	if in.Record().Fields[1].Num != 2 {
		t.Error("mask changed its argument")
	}
}

func TestTimerHeap(t *testing.T) {
	var h timerHeap
	for i, at := range []int64{50, 10, 30, 10, 40, 10, 0} {
		h.push(&timer{at: at, seq: i})
	}
	var got []string
	for h.len() > 0 {
		x := h.pop()
		got = append(got, strings.Join([]string{value.FormatNumber(float64(x.at)), value.FormatNumber(float64(x.seq))}, "/"))
	}
	if s := strings.Join(got, " "); s != "0/6 10/1 10/3 10/5 30/2 40/4 50/0" {
		t.Errorf("order %s", s)
	}
}

func TestFloorDiv(t *testing.T) {
	for _, c := range [][3]int64{{0, 1000, 0}, {999, 1000, 0}, {1000, 1000, 1}, {-1, 1000, -1}, {-1000, 1000, -1}, {-1001, 1000, -2}} {
		if got := floorDiv(c[0], c[1]); got != c[2] {
			t.Errorf("floorDiv(%d, %d) = %d, want %d", c[0], c[1], got, c[2])
		}
	}
}

// TestEventIDs: every trace event of std maps from a fact event, and no
// other fact event maps to an observer.
func TestEventIDs(t *testing.T) {
	facts := []string{"sim.Started", "sim.Ended", "sim.Added", "sim.Removed", "sim.Crashed", "sim.Restarted",
		"sim.Changed", "sim.Injected", "sim.Fired", "sim.Broken", "sim.Emitted", "sim.Failed",
		"net.Sent", "net.Arrived", "net.Dropped", "net.Partitioned", "net.Healed", "disk.Written"}
	seen := map[string]bool{}
	for _, ev := range facts {
		id := eventID(ev)
		if id == "" {
			t.Errorf("no observer event for %s", ev)
			continue
		}
		pkg, name, _ := strings.Cut(ev, ".")
		if want := pkg + "." + strings.ToLower(name[:1]) + name[1:]; id != want {
			t.Errorf("eventID(%s) = %s, want %s", ev, id, want)
		}
		seen[id] = true
	}
	for id, s := range stdlib.Natives {
		if s.Kind == stdlib.TraceEvent && !seen[id] {
			t.Errorf("trace event %s has no fact event", id)
		}
	}
	if eventID("net.Received") != "" || eventID("nope") != "" {
		t.Error("an unknown event maps to an observer")
	}
}

func TestUsesLook(t *testing.T) {
	p := &ir.Program{Packages: []*ir.Package{{Path: "std/sim"}}}
	if UsesLook(p) {
		t.Error("a program without std/look uses it")
	}
	p.Packages = append(p.Packages, &ir.Package{Path: "std/look"})
	if !UsesLook(p) {
		t.Error("a program with std/look does not use it")
	}
}

// TestPaceOptions: pace is clamped into 250..4000 and 0 means 1000.
func TestPaceOptions(t *testing.T) {
	for _, c := range [][2]int{{0, 1000}, {1, 250}, {250, 250}, {1000, 1000}, {4000, 4000}, {9000, 4000}} {
		b := newBuilder(Input{}, Options{Pace: c[0]})
		if int(b.clock.pace) != c[1] {
			t.Errorf("pace %d became %d, want %d", c[0], b.clock.pace, c[1])
		}
	}
}

// TestEmptyRun: no facts gives an empty but valid scene.
func TestEmptyRun(t *testing.T) {
	sc, fl := Build(Input{Prog: &ir.Program{}}, Options{})
	if fl != nil {
		t.Fatal(fl.Fault)
	}
	checkScene(t, sc)
	if len(sc.Shapes) != 0 || len(sc.Slots) != 0 || sc.Duration != 0 {
		t.Errorf("scene = %+v", sc)
	}
}
