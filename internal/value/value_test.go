package value

import (
	"encoding/json"
	"math"
	"math/rand"
	"sort"
	"testing"

	"github.com/synodlang/compiler/internal/ir"
)

func TestFormatNumberMatchesJSON(t *testing.T) {
	g := rand.New(rand.NewSource(7))
	check := func(f float64) {
		if math.IsNaN(f) || math.IsInf(f, 0) {
			return
		}
		b, err := json.Marshal(f)
		if err != nil {
			t.Fatal(err)
		}
		want := string(b)
		if f == 0 {
			want = "0"
		}
		if got := FormatNumber(f); got != want {
			t.Fatalf("FormatNumber(%v) = %q, json %q", f, got, want)
		}
	}
	for i := 0; i < 100000; i++ {
		switch i % 6 {
		case 0:
			check(math.Float64frombits(g.Uint64()))
		case 1:
			check(float64(g.Int63n(1<<53)) * float64(1-2*g.Intn(2)))
		case 2:
			check(g.NormFloat64() * math.Pow(10, float64(g.Intn(60)-30)))
		case 3:
			check(math.Float64frombits(g.Uint64() & (1<<52 - 1))) // subnormal
		case 4:
			check(math.Pow(10, float64(g.Intn(80)-40)) * float64(1+g.Intn(9)))
		case 5:
			check(g.Float64())
		}
	}
	for _, f := range []float64{0, math.Copysign(0, -1), 1e21, 1e-6, 1e-7, 123456789012345680000, 5e-324, math.MaxFloat64, -1.5} {
		check(f)
	}
	if FormatNumber(1e-7) != "1e-7" || FormatNumber(1e21) != "1e+21" || FormatNumber(math.Copysign(0, -1)) != "0" {
		t.Error("exponent forms")
	}
}

func TestFormatTime(t *testing.T) {
	for _, c := range []struct {
		us   int64
		want string
	}{
		{0, "0s"}, {1, "0.001ms"}, {500, "0.5ms"}, {20431, "20.431ms"}, {1500000, "1500ms"},
		{2000000, "2s"}, {-1500000, "-1500ms"}, {-1, "-0.001ms"}, {1000, "1ms"}, {600000000, "600s"},
		{math.MaxInt64, "9223372036854775.807ms"},
	} {
		if got := FormatTime(c.us); got != c.want {
			t.Errorf("FormatTime(%d) = %q, want %q", c.us, got, c.want)
		}
	}
	_ = FormatTime(math.MinInt64)
}

func testRecords() (*ir.Record, *ir.Record) {
	p := &ir.Package{Path: "example.com/paxos", Name: "paxos"}
	r := &ir.Record{Pkg: p, Name: "Prepare", Exported: true}
	r.Fields = []*ir.Field{
		{Pkg: p, Record: r, Name: "number", Exported: true},
		{Pkg: p, Record: r, Name: "secret", Exported: false},
	}
	q := &ir.Record{Pkg: p, Name: "Empty", Exported: true}
	return r, q
}

func TestFormat(t *testing.T) {
	r, e := testRecords()
	rec := RecordOf(r, []Value{Number(101), Text("s")})
	m := &Map{}
	m = m.With(Text("b"), Number(2)).With(Text("a"), Number(1))
	for _, c := range []struct {
		v    Value
		o    FormatOpts
		want string
	}{
		{Text(`a"b\c` + "\n"), FormatOpts{}, `"a\"b\\c\n"`},
		{Text("raw"), FormatOpts{TopText: true}, "raw"},
		{ListOf([]Value{Text("x")}), FormatOpts{TopText: true}, `{"x"}`},
		{Number(1.5), FormatOpts{}, "1.5"},
		{Number(math.Copysign(0, -1)), FormatOpts{}, "0"},
		{Time(2000000), FormatOpts{}, "2s"},
		{Bool(true), FormatOpts{}, "true"},
		{Node(3), FormatOpts{}, "n3"},
		{Node(0), FormatOpts{}, "nobody"},
		{ListOf(nil), FormatOpts{}, "{}"},
		{ListOf([]Value{Number(1), Node(2)}), FormatOpts{}, "{1, n2}"},
		{MapValue(m), FormatOpts{}, `{"a": 1, "b": 2}`},
		{MapValue(&Map{}), FormatOpts{}, "{}"},
		{rec, FormatOpts{}, `paxos.Prepare{number 101, secret …}`},
		{rec, FormatOpts{ShowUnexported: true}, `paxos.Prepare{number 101, secret "s"}`},
		{rec, FormatOpts{Qual: func(*ir.Package) string { return "p2" }}, `p2.Prepare{number 101, secret …}`},
		{RecordOf(e, nil), FormatOpts{}, "paxos.Empty{}"},
		{FieldOf(r.Fields[0]), FormatOpts{}, "paxos.number"},
	} {
		if got := Format(c.v, c.o); got != c.want {
			t.Errorf("Format = %q, want %q", got, c.want)
		}
	}
}

func randomValue(g *rand.Rand, depth int) Value {
	r, e := testRecords()
	n := 6
	if depth > 0 {
		n = 10
	}
	switch g.Intn(n) {
	case 0:
		return Text(string(rune('a' + g.Intn(3))))
	case 1:
		return Number(float64(g.Intn(5)-2) / 2)
	case 2:
		return Bool(g.Intn(2) == 0)
	case 3:
		return Time(int64(g.Intn(5)))
	case 4:
		return Node(g.Intn(4))
	case 5:
		return Number(math.Copysign(0, -1))
	case 6:
		items := make([]Value, g.Intn(3))
		for i := range items {
			items[i] = randomValue(g, depth-1)
		}
		return ListOf(items)
	case 7:
		m := &Map{}
		for i := g.Intn(3); i > 0; i-- {
			m = m.With(randomValue(g, depth-1), randomValue(g, depth-1))
		}
		return MapValue(m)
	case 8:
		return RecordOf(r, []Value{randomValue(g, depth-1), randomValue(g, depth-1)})
	}
	return RecordOf(e, nil)
}

func TestCompareTotalOrder(t *testing.T) {
	g := rand.New(rand.NewSource(3))
	vs := make([]Value, 400)
	for i := range vs {
		vs[i] = randomValue(g, 2)
	}
	sgn := func(x int) int {
		if x < 0 {
			return -1
		} else if x > 0 {
			return 1
		}
		return 0
	}
	for _, a := range vs {
		if Compare(a, a) != 0 || !Equal(a, a) {
			t.Fatal("reflexivity")
		}
	}
	for i := 0; i < 20000; i++ {
		a, b, c := vs[g.Intn(len(vs))], vs[g.Intn(len(vs))], vs[g.Intn(len(vs))]
		if sgn(Compare(a, b)) != -sgn(Compare(b, a)) {
			t.Fatalf("antisymmetry: %s %s", Format(a, FormatOpts{}), Format(b, FormatOpts{}))
		}
		if Compare(a, b) <= 0 && Compare(b, c) <= 0 && Compare(a, c) > 0 {
			t.Fatalf("transitivity: %s %s %s", Format(a, FormatOpts{}), Format(b, FormatOpts{}), Format(c, FormatOpts{}))
		}
	}
	// Kind order and specifics.
	order := []Value{Text("z"), Number(1), Bool(true), Time(0), Node(0), ListOf(nil), MapValue(&Map{})}
	for i := 1; i < len(order); i++ {
		if Compare(order[i-1], order[i]) >= 0 {
			t.Errorf("kind order at %d", i)
		}
	}
	if Compare(Text("B"), Text("a")) >= 0 || Compare(Bool(false), Bool(true)) >= 0 || Compare(Node(0), Node(1)) >= 0 {
		t.Error("basic order")
	}
	if Compare(ListOf([]Value{Number(1)}), ListOf([]Value{Number(1), Number(0)})) >= 0 {
		t.Error("list prefix")
	}
	if !Equal(Number(0), Number(math.Copysign(0, -1))) {
		t.Error("-0 equal 0")
	}
}

func TestMapOps(t *testing.T) {
	g := rand.New(rand.NewSource(5))
	m := &Map{}
	ref := map[float64]float64{}
	var snapshots []*Map
	var snapKeys [][]float64
	for i := 0; i < 500; i++ {
		k := float64(g.Intn(40))
		if g.Intn(4) == 0 {
			m = m.Without(Number(k))
			delete(ref, k)
		} else {
			v := g.Float64()
			m = m.With(Number(k), Number(v))
			ref[k] = v
		}
		if i%100 == 0 {
			snapshots = append(snapshots, m)
			var ks []float64
			for k := range ref {
				ks = append(ks, k)
			}
			sort.Float64s(ks)
			snapKeys = append(snapKeys, ks)
		}
		if m.Len() != len(ref) {
			t.Fatal("len")
		}
		for j := 1; j < len(m.Keys); j++ {
			if Compare(m.Keys[j-1], m.Keys[j]) >= 0 {
				t.Fatal("not sorted")
			}
		}
		for k, v := range ref {
			if got, ok := m.Get(Number(k)); !ok || got.Num != v {
				t.Fatal("get")
			}
		}
	}
	// Persistence: old maps unchanged.
	for i, s := range snapshots {
		if s.Len() != len(snapKeys[i]) {
			t.Fatal("snapshot changed")
		}
		for j, k := range snapKeys[i] {
			if s.Keys[j].Num != k {
				t.Fatal("snapshot keys changed")
			}
		}
	}
	a := (&Map{}).With(Text("k"), Number(1))
	b := a.With(Text("k"), Number(2))
	if v, _ := a.Get(Text("k")); v.Num != 1 {
		t.Error("With mutated receiver")
	}
	if v, _ := b.Get(Text("k")); v.Num != 2 {
		t.Error("With replace")
	}
	if a.Without(Text("none")) != a {
		t.Error("Without absent")
	}
	if _, ok := a.Get(Text("none")); ok {
		t.Error("Get absent")
	}
}

func TestNegativeZero(t *testing.T) {
	z := Number(math.Copysign(0, -1))
	if math.Signbit(z.Num) {
		t.Error("Number keeps -0")
	}
}

func TestNodeName(t *testing.T) {
	if NodeName(0) != "nobody" || NodeName(12) != "n12" {
		t.Error("NodeName")
	}
}
