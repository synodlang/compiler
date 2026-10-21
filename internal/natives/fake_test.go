package natives

import (
	"fmt"
	"sort"
	"testing"

	"github.com/synodlang/compiler/internal/ir"
	"github.com/synodlang/compiler/internal/rng"
	"github.com/synodlang/compiler/internal/source"
	"github.com/synodlang/compiler/internal/stdlib"
	"github.com/synodlang/compiler/internal/types"
	"github.com/synodlang/compiler/internal/value"
)

type stored struct {
	v value.Value
	t types.Type
}

// fake is a Host that logs every action it is asked to do.
type fake struct {
	log   []string
	now   int64
	r     *rng.PCG32
	draws int
	nodes []int
	run   map[*ir.Package][]int
	up    map[int]bool
	disk  map[string]stored
	acc   func(s, t types.Type) bool
	next  int
}

func newFake() *fake {
	return &fake{r: rng.New(42), run: map[*ir.Package][]int{}, up: map[int]bool{}, disk: map[string]stored{},
		acc: types.Equal}
}

func (f *fake) logf(format string, a ...any) { f.log = append(f.log, fmt.Sprintf(format, a...)) }
func (f *fake) Now() int64                   { return f.now }
func (f *fake) RNG() *rng.PCG32              { return f.r }
func (f *fake) Seed(s uint64)                { f.logf("seed %d", s) }
func (f *fake) StopAt(t int64)               { f.logf("stopAt %d", t) }
func (f *fake) Spawn(p *ir.Package) int {
	f.next++
	f.nodes = append(f.nodes, f.next)
	f.run[p] = append(f.run[p], f.next)
	f.up[f.next] = true
	f.logf("spawn %s %d", p.Name, f.next)
	return f.next
}
func (f *fake) Crash(n int)                        { f.logf("crash %d", n) }
func (f *fake) Restart(n int)                      { f.logf("restart %d", n) }
func (f *fake) Remove(n int)                       { f.logf("remove %d", n) }
func (f *fake) Nodes() []int                       { return f.nodes }
func (f *fake) NodesRunning(p *ir.Package) []int   { return f.run[p] }
func (f *fake) IsUp(n int) bool                    { return f.up[n] }
func (f *fake) Send(m *value.Record, from, to int) { f.logf("send %s %d %d", m.Type.Name, from, to) }
func (f *fake) SetLatency(a, b int64)              { f.logf("latency %d %d", a, b) }
func (f *fake) SetLoss(p float64)                  { f.logf("loss %v", p) }
func (f *fake) Partition(a, b []int)               { f.logf("partition %v %v", a, b) }
func (f *fake) Heal()                              { f.logf("heal") }
func (f *fake) DropNext(typ string, from, to int)  { f.logf("dropNext %s %d %d", typ, from, to) }
func (f *fake) Accepts(s, t types.Type) bool       { return f.acc(s, t) }
func (f *fake) DiskWrite(n int, k string, v value.Value, t types.Type) {
	f.disk[fmt.Sprint(n, k)] = stored{v, t}
	f.logf("write %d %s", n, k)
}
func (f *fake) DiskRead(n int, k string) (value.Value, types.Type, bool) {
	s, ok := f.disk[fmt.Sprint(n, k)]
	return s.v, s.t, ok
}

var testPos = ir.Pos{File: "demo/demo.synod", Line: 7, Col: 3}

// mkCall builds a call whose arguments have the given static types.
func mkCall(ts ...types.Type) *ir.Call {
	c := &ir.Call{Base: ir.Base{At: testPos}}
	for _, t := range ts {
		c.Args = append(c.Args, &ir.Const{Base: ir.Base{Ty: t}})
	}
	return c
}

func num(f float64) value.Value { return value.Number(f) }
func txt(s string) value.Value  { return value.Text(s) }
func tm(us int64) value.Value   { return value.Time(us) }
func nd(id int) value.Value     { return value.Node(id) }
func lst(v ...value.Value) value.Value {
	return value.ListOf(v)
}
func nums(f ...float64) value.Value {
	var v []value.Value
	for _, x := range f {
		v = append(v, num(x))
	}
	return lst(v...)
}
func mp(kv ...value.Value) value.Value {
	m := &value.Map{}
	for i := 0; i < len(kv); i += 2 {
		m = m.With(kv[i], kv[i+1])
	}
	return value.MapValue(m)
}

// run calls the native id with c (or a call without typed arguments).
func run(t *testing.T, h *fake, id string, c *ir.Call, args ...value.Value) (value.Value, *value.Fault) {
	t.Helper()
	fn := Table[id]
	if fn == nil {
		t.Fatalf("no native %s", id)
	}
	if c == nil {
		c = mkCall()
	}
	return fn(h, c, args)
}

func ok(t *testing.T, h *fake, id string, args ...value.Value) value.Value {
	t.Helper()
	v, f := run(t, h, id, nil, args...)
	if f != nil {
		t.Fatalf("%s: unexpected fault %v", id, f)
	}
	return v
}

// wantFault checks the code, message and position of a fault.
func wantFault(t *testing.T, id string, f *value.Fault, code source.Code, msg string) {
	t.Helper()
	if f == nil {
		t.Fatalf("%s: want fault %s, got none", id, code)
	}
	if f.Code != code || f.Message != msg || !f.HasPos || f.Pos != testPos {
		t.Errorf("%s: got %s %q pos %v/%v; want %s %q at %v", id, f.Code, f.Message, f.Pos, f.HasPos, code, msg, testPos)
	}
}

func same(t *testing.T, what string, got, want value.Value) {
	t.Helper()
	if !value.Equal(got, want) || got.K != want.K {
		t.Errorf("%s: got %s, want %s", what, value.Format(got, value.FormatOpts{}), value.Format(want, value.FormatOpts{}))
	}
}

func wantLog(t *testing.T, h *fake, want ...string) {
	t.Helper()
	if fmt.Sprint(h.log) != fmt.Sprint(want) {
		t.Errorf("log %v, want %v", h.log, want)
	}
}

func TestCoverage(t *testing.T) {
	want := map[string]bool{}
	for id, s := range stdlib.Natives {
		if s.Kind == stdlib.Phrase && len(id) > 5 && id[:5] != "look." {
			want[id] = true
		}
	}
	for id := range Table {
		if !want[id] {
			t.Errorf("Table has %s, which is not a std phrase native", id)
		}
		delete(want, id)
	}
	var missing []string
	for id := range want {
		missing = append(missing, id)
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		t.Errorf("missing natives: %v", missing)
	}
}
