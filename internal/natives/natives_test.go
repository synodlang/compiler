package natives

import (
	"math"
	"testing"

	"github.com/synodlang/compiler/internal/ir"
	"github.com/synodlang/compiler/internal/rng"
	"github.com/synodlang/compiler/internal/source"
	"github.com/synodlang/compiler/internal/types"
	"github.com/synodlang/compiler/internal/value"
)

var pkgA = &ir.Package{Name: "a", Path: "x/a"}
var pkgB = &ir.Package{Name: "b", Path: "x/b"}

func TestSim(t *testing.T) {
	h := newFake()
	ok(t, h, "sim.seed", num(7))
	ok(t, h, "sim.seed", num(1<<53))
	ok(t, h, "sim.stopAt", tm(0))
	ok(t, h, "sim.stopAt", tm(5000))
	n := ok(t, h, "sim.spawn", value.PackageOf(pkgA))
	same(t, "spawn", n, nd(1))
	same(t, "cluster", ok(t, h, "sim.cluster", num(2), value.PackageOf(pkgB)), lst(nd(2), nd(3)))
	same(t, "cluster 0", ok(t, h, "sim.cluster", num(0), value.PackageOf(pkgB)), lst())
	ok(t, h, "sim.crash", nd(2))
	ok(t, h, "sim.restart", nd(2))
	ok(t, h, "sim.remove", nd(3))
	wantLog(t, h, "seed 7", "seed 9007199254740992", "stopAt 0", "stopAt 5000", "spawn a 1",
		"spawn b 2", "spawn b 3", "crash 2", "restart 2", "remove 3")
	same(t, "id", ok(t, h, "sim.id", nd(3)), num(3))
	same(t, "id nobody", ok(t, h, "sim.id", nd(0)), num(0))
	same(t, "name", ok(t, h, "sim.name", nd(12)), txt("n12"))
	same(t, "name nobody", ok(t, h, "sim.name", nd(0)), txt("nobody"))
	same(t, "nobody", ok(t, h, "sim.nobody"), nd(0))
	same(t, "nodes", ok(t, h, "sim.nodes"), lst(nd(1), nd(2), nd(3)))
	same(t, "running", ok(t, h, "sim.nodesRunning", value.PackageOf(pkgB)), lst(nd(2), nd(3)))
	same(t, "running none", ok(t, h, "sim.nodesRunning", value.PackageOf(&ir.Package{})), lst())
	h.up[1] = true
	same(t, "up", ok(t, h, "sim.isUp", nd(1)), value.Bool(true))
	same(t, "down", ok(t, h, "sim.isUp", nd(9)), value.Bool(false))
	h.up[0] = true
	same(t, "nobody up", ok(t, h, "sim.isUp", nd(0)), value.Bool(false))

	for _, bad := range []float64{-1, 1.5, 1<<53 + 2, math.Inf(1)} {
		_, f := run(t, h, "sim.seed", nil, num(bad))
		wantFault(t, "sim.seed", f, source.BadArgument, "sim.Seed needs a whole number from 0 to 2^53")
	}
	_, f := run(t, h, "sim.stopAt", nil, tm(-1))
	wantFault(t, "sim.stopAt", f, source.BadArgument, "sim.StopAt needs a time of 0 or more")
	for _, bad := range []float64{-1, 0.5} {
		_, f = run(t, h, "sim.cluster", nil, num(bad), value.PackageOf(pkgA))
		wantFault(t, "sim.cluster", f, source.BadArgument, "sim.Cluster needs a whole count of 0 or more")
	}
	_, f = run(t, h, "sim.cluster", nil, num(MaxItems+1), value.PackageOf(pkgA))
	wantFault(t, "sim.cluster", f, source.RunLimit, "sim.Cluster of more than 1000000 nodes")
	for id, what := range map[string]string{"sim.crash": "sim.Crash of", "sim.restart": "sim.Restart of", "sim.remove": "sim.Remove of"} {
		_, f = run(t, h, id, nil, nd(0))
		wantFault(t, id, f, source.Nobody, what+" sim.Nobody")
	}
}

func TestNet(t *testing.T) {
	h := newFake()
	rec := value.RecordOf(&ir.Record{Name: "Ping", Pkg: pkgA}, nil)
	ok(t, h, "net.send", rec, nd(1), nd(2))
	ok(t, h, "net.send", rec, nd(1), nd(1))
	ok(t, h, "net.latency", tm(0), tm(0))
	ok(t, h, "net.latency", tm(10), tm(30))
	ok(t, h, "net.loss", num(0))
	ok(t, h, "net.loss", num(0.25))
	ok(t, h, "net.loss", num(1))
	ok(t, h, "net.partition", lst(nd(1)), lst(nd(2), nd(3)))
	ok(t, h, "net.partition", lst(), lst(nd(2)))
	ok(t, h, "net.heal")
	ok(t, h, "net.dropNext", txt("Ping"), nd(1), nd(2))
	wantLog(t, h, "send Ping 1 2", "send Ping 1 1", "latency 0 0", "latency 10 30", "loss 0", "loss 0.25", "loss 1",
		"partition [1] [2 3]", "partition [] [2]", "heal", "dropNext Ping 1 2")
	cases := []struct {
		id   string
		args []value.Value
		msg  string
		code source.Code
	}{
		{"net.send", []value.Value{rec, nd(0), nd(2)}, "net.Send from sim.Nobody", source.Nobody},
		{"net.send", []value.Value{rec, nd(1), nd(0)}, "net.Send to sim.Nobody", source.Nobody},
		{"net.latency", []value.Value{tm(-1), tm(5)}, "net.Latency needs 0 ≤ a ≤ b", source.BadArgument},
		{"net.latency", []value.Value{tm(6), tm(5)}, "net.Latency needs 0 ≤ a ≤ b", source.BadArgument},
		{"net.loss", []value.Value{num(-0.1)}, "net.Loss needs 0 ≤ p ≤ 1", source.BadArgument},
		{"net.loss", []value.Value{num(1.1)}, "net.Loss needs 0 ≤ p ≤ 1", source.BadArgument},
		{"net.partition", []value.Value{lst(nd(0)), lst(nd(1))}, "net.Partition with sim.Nobody", source.Nobody},
		{"net.partition", []value.Value{lst(nd(1)), lst(nd(2), nd(0))}, "net.Partition with sim.Nobody", source.Nobody},
		{"net.dropNext", []value.Value{txt("P"), nd(0), nd(1)}, "net.DropNext from sim.Nobody", source.Nobody},
		{"net.dropNext", []value.Value{txt("P"), nd(1), nd(0)}, "net.DropNext to sim.Nobody", source.Nobody},
	}
	n := len(h.log)
	for _, c := range cases {
		_, f := run(t, h, c.id, nil, c.args...)
		wantFault(t, c.id, f, c.code, c.msg)
	}
	if len(h.log) != n {
		t.Errorf("faulting calls acted: %v", h.log[n:])
	}
}

func TestTimeNow(t *testing.T) {
	h := newFake()
	h.now = 1234
	same(t, "now", ok(t, h, "time.now"), tm(1234))
}

// counting draws: every random native consumes exactly one draw.
func TestRandom(t *testing.T) {
	cases := []struct {
		id   string
		args []value.Value
	}{
		{"random.pick", []value.Value{nums(1, 2, 3)}},
		{"random.pick", []value.Value{nums(9)}},
		{"random.between", []value.Value{num(1), num(5)}},
		{"random.between", []value.Value{num(2), num(2)}},
		{"random.timeBetween", []value.Value{tm(10), tm(30)}},
		{"random.timeBetween", []value.Value{tm(10), tm(10)}},
		{"random.chance", []value.Value{num(0.5)}},
		{"random.chance", []value.Value{num(-3)}},
		{"random.chance", []value.Value{num(7)}},
	}
	for _, c := range cases {
		h := newFake()
		ref := rng.New(42)
		ok(t, h, c.id, c.args...)
		ref.Next()
		if h.r.Next() != ref.Next() {
			t.Errorf("%s %v: did not consume exactly one draw", c.id, c.args)
		}
	}
	h := newFake()
	ref := rng.New(42)
	v := ok(t, h, "random.pick", nums(10, 20, 30, 40))
	same(t, "pick", v, num(float64([]int{10, 20, 30, 40}[ref.Index(4)])))
	h = newFake()
	ref = rng.New(42)
	same(t, "between", ok(t, h, "random.between", num(1), num(5)), num(ref.Between(1, 5)))
	h = newFake()
	ref = rng.New(42)
	same(t, "timeBetween", ok(t, h, "random.timeBetween", tm(10), tm(30)), tm(ref.TimeBetween(10, 30)))
	h = newFake()
	ref = rng.New(42)
	same(t, "chance", ok(t, h, "random.chance", num(0.5)), value.Bool(ref.Chance(0.5)))
	same(t, "chance 0", ok(t, newFake(), "random.chance", num(0)), value.Bool(false))
	same(t, "chance 1", ok(t, newFake(), "random.chance", num(1)), value.Bool(true))
	for i := 0; i < 50; i++ {
		x := ok(t, h, "random.timeBetween", tm(10), tm(30))
		if x.Int < 10 || x.Int >= 30 {
			t.Fatalf("out of range %d", x.Int)
		}
	}

	// faults: no draw
	h = newFake()
	_, f := run(t, h, "random.pick", nil, lst())
	wantFault(t, "random.pick", f, source.IndexRange, "random.Pick of an empty list")
	_, f = run(t, h, "random.between", nil, num(2), num(1))
	wantFault(t, "random.between", f, source.BadArgument, "random.Between needs a ≤ b")
	_, f = run(t, h, "random.timeBetween", nil, tm(2), tm(1))
	wantFault(t, "random.timeBetween", f, source.BadArgument, "random.TimeBetween needs a ≤ b")
	if h.r.Next() != rng.New(42).Next() {
		t.Error("a faulting call drew")
	}
	_, f = run(t, newFake(), "random.between", nil, num(-math.MaxFloat64), num(math.MaxFloat64))
	wantFault(t, "random.between", f, source.NumberRange, "the result is too large for a number")
}

func TestLists(t *testing.T) {
	h := newFake()
	same(t, "size", ok(t, h, "lists.size", nums(1, 2, 3)), num(3))
	same(t, "size 0", ok(t, h, "lists.size", lst()), num(0))
	orig := nums(1, 2)
	same(t, "append", ok(t, h, "lists.append", num(3), orig), nums(1, 2, 3))
	same(t, "orig kept", orig, nums(1, 2))
	same(t, "extend", ok(t, h, "lists.extend", nums(1), nums(2, 3)), nums(1, 2, 3))
	same(t, "extend empty", ok(t, h, "lists.extend", lst(), lst()), lst())
	same(t, "without", ok(t, h, "lists.without", num(2), nums(2, 1, 2, 3)), nums(1, 3))
	same(t, "without none", ok(t, h, "lists.without", num(9), nums(1)), nums(1))
	same(t, "take", ok(t, h, "lists.take", num(2), nums(5, 6, 7)), nums(5, 6))
	same(t, "take 0", ok(t, h, "lists.take", num(0), nums(5, 6, 7)), lst())
	same(t, "take more", ok(t, h, "lists.take", num(10), nums(5)), nums(5))
	same(t, "take huge", ok(t, h, "lists.take", num(1e300), nums(5)), nums(5))
	same(t, "range", ok(t, h, "lists.range", num(2), num(5)), nums(2, 3, 4, 5))
	same(t, "range one", ok(t, h, "lists.range", num(-1), num(-1)), nums(-1))
	same(t, "range empty", ok(t, h, "lists.range", num(3), num(2)), lst())
	same(t, "sort", ok(t, h, "lists.sort", nums(3, 1, 2, -1)), nums(-1, 1, 2, 3))
	same(t, "sort time", ok(t, h, "lists.sort", lst(tm(5), tm(1))), lst(tm(1), tm(5)))
	same(t, "sort text", ok(t, h, "lists.sort", lst(txt("b"), txt("a"))), lst(txt("a"), txt("b")))
	same(t, "sort empty", ok(t, h, "lists.sort", lst()), lst())
	in := nums(2, 1)
	ok(t, h, "lists.sort", in)
	same(t, "sort keeps input", in, nums(2, 1))

	_, f := run(t, h, "lists.take", nil, num(-1), nums(1))
	wantFault(t, "lists.take", f, source.BadArgument, "lists.Take needs a whole count of 0 or more")
	_, f = run(t, h, "lists.take", nil, num(1.5), nums(1))
	wantFault(t, "lists.take", f, source.BadArgument, "lists.Take needs a whole count of 0 or more")
	_, f = run(t, h, "lists.range", nil, num(1.5), num(3))
	wantFault(t, "lists.range", f, source.BadArgument, "lists.Range needs whole numbers")
	_, f = run(t, h, "lists.range", nil, num(1), num(math.Inf(1)))
	wantFault(t, "lists.range", f, source.BadArgument, "lists.Range needs whole numbers")
	_, f = run(t, h, "lists.range", nil, num(1), num(MaxItems+1))
	wantFault(t, "lists.range", f, source.RunLimit, "lists.Range would make a list of more than 1000000 items")
	same(t, "range max", ok(t, h, "lists.range", num(1), num(MaxItems)), ok(t, h, "lists.range", num(1), num(MaxItems)))
	_, f = run(t, h, "lists.sort", nil, lst(num(1), txt("a")))
	wantFault(t, "lists.sort", f, source.BadArgument, "lists.Sort needs items that are all numbers, all times or all text")
	_, f = run(t, h, "lists.sort", nil, lst(value.Bool(true)))
	wantFault(t, "lists.sort", f, source.BadArgument, "lists.Sort needs items that are all numbers, all times or all text")
	big := ok(t, h, "lists.range", num(1), num(MaxItems))
	_, f = run(t, h, "lists.append", nil, num(1), big)
	wantFault(t, "lists.append", f, source.RunLimit, "lists.Append would make a list of more than 1000000 items")
	_, f = run(t, h, "lists.extend", nil, big, nums(1))
	wantFault(t, "lists.extend", f, source.RunLimit, "lists.Extend would make a list of more than 1000000 items")
}

func TestMaps(t *testing.T) {
	h := newFake()
	m := mp(txt("b"), num(1), txt("a"), num(2), txt("c"), num(1))
	same(t, "size", ok(t, h, "maps.size", m), num(3))
	same(t, "size 0", ok(t, h, "maps.size", value.MapValue(&value.Map{})), num(0))
	same(t, "keys", ok(t, h, "maps.keys", m), lst(txt("a"), txt("b"), txt("c")))
	same(t, "keys empty", ok(t, h, "maps.keys", value.MapValue(&value.Map{})), lst())
	same(t, "without", ok(t, h, "maps.without", txt("b"), m), mp(txt("a"), num(2), txt("c"), num(1)))
	same(t, "without absent", ok(t, h, "maps.without", txt("z"), m), m)
	same(t, "keysByValue", ok(t, h, "maps.keysByValue", m), lst(txt("b"), txt("c"), txt("a")))
	nm := mp(nd(3), num(5), nd(1), num(5), nd(2), num(0))
	same(t, "keysByValue nodes", ok(t, h, "maps.keysByValue", nm), lst(nd(2), nd(1), nd(3)))
	same(t, "keysByValue empty", ok(t, h, "maps.keysByValue", value.MapValue(&value.Map{})), lst())
}

func TestMath(t *testing.T) {
	h := newFake()
	same(t, "floor", ok(t, h, "math.floor", num(-1.5)), num(-2))
	same(t, "ceil", ok(t, h, "math.ceil", num(1.2)), num(2))
	if v := ok(t, h, "math.ceil", num(-0.5)); math.Signbit(v.Num) {
		t.Error("ceil gave -0")
	}
	same(t, "min", ok(t, h, "math.min", num(2), num(3)), num(2))
	same(t, "max", ok(t, h, "math.max", num(2), num(3)), num(3))
	if v := ok(t, h, "math.max", num(math.Copysign(0, -1)), num(math.Copysign(0, -1))); math.Signbit(v.Num) {
		t.Error("max gave -0")
	}
	if v := ok(t, h, "math.min", num(-0.0), num(0)); math.Signbit(v.Num) {
		t.Error("min gave -0")
	}
	for _, c := range []struct{ a, b, want float64 }{{7, 3, 1}, {-7, 3, 2}, {7, -3, -2}, {-7, -3, -1}, {6, 3, 0}, {5.5, 2, 1.5}, {0, 5, 0}} {
		same(t, "remainder", ok(t, h, "math.remainder", num(c.a), num(c.b)), num(c.want))
	}
	_, f := run(t, h, "math.remainder", nil, num(1), num(0))
	wantFault(t, "math.remainder", f, source.DivisionByZero, "division by zero")
	_, f = run(t, h, "math.remainder", nil, num(0), num(0))
	wantFault(t, "math.remainder", f, source.DivisionByZero, "division by zero")
	_, f = run(t, h, "math.remainder", nil, num(1e308), num(1e-308))
	wantFault(t, "math.remainder", f, source.NumberRange, "the result is too large for a number")

	same(t, "hash empty", ok(t, h, "math.hash", txt("")), num(float64(14695981039346656037>>11)/(1<<53)))
	// FNV-1a 64 of "a" is af63dc4c8601ec8c.
	same(t, "hash a", ok(t, h, "math.hash", txt("a")), num(float64(uint64(0xaf63dc4c8601ec8c)>>11)/(1<<53)))
	same(t, "hash foobar", ok(t, h, "math.hash", txt("foobar")), num(float64(uint64(0x85944171f73967e8)>>11)/(1<<53)))
	for _, s := range []string{"", "a", "héllo", "zzzzzzzz"} {
		if x := ok(t, h, "math.hash", txt(s)).Num; x < 0 || x >= 1 {
			t.Errorf("hash %q = %v out of [0,1)", s, x)
		}
	}
	if ok(t, h, "math.hash", txt("é")).Num == ok(t, h, "math.hash", txt("e")).Num {
		t.Error("hash ignores bytes")
	}
}

func TestText(t *testing.T) {
	h := newFake()
	same(t, "from number", ok(t, h, "text.from", num(1.50)), txt("1.5"))
	same(t, "from whole", ok(t, h, "text.from", num(3)), txt("3"))
	same(t, "from text", ok(t, h, "text.from", txt("hi")), txt("hi"))
	same(t, "from time", ok(t, h, "text.from", tm(1500000)), txt("1500ms"))
	same(t, "from node", ok(t, h, "text.from", nd(2)), txt("n2"))
	same(t, "from nobody", ok(t, h, "text.from", nd(0)), txt("nobody"))
	same(t, "from bool", ok(t, h, "text.from", value.Bool(true)), txt("true"))
	same(t, "from list", ok(t, h, "text.from", lst(txt("a"), num(1))), txt(`{"a", 1}`))
	same(t, "join", ok(t, h, "text.join", lst(txt("a"), txt("bc"), txt(""))), txt("abc"))
	same(t, "join empty", ok(t, h, "text.join", lst()), txt(""))
	same(t, "size", ok(t, h, "text.size", txt("héé€")), num(4))
	same(t, "size 0", ok(t, h, "text.size", txt("")), num(0))
	big := make([]byte, MaxItems)
	for i := range big {
		big[i] = 'x'
	}
	_, f := run(t, h, "text.join", nil, lst(txt(string(big)), txt("y")))
	wantFault(t, "text.join", f, source.RunLimit, "text.Join would make a text of more than 1000000 characters")
	same(t, "join max", ok(t, h, "text.join", lst(txt(string(big)))), txt(string(big)))
}

func TestDisk(t *testing.T) {
	h := newFake()
	listText := types.List{Elem: types.TText}
	wc := mkCall(listText, types.TText, types.TNode)
	v := lst(txt("a"))
	if _, f := run(t, h, "disk.write", wc, v, txt("pages"), nd(1)); f != nil {
		t.Fatal(f)
	}
	wantLog(t, h, "write 1 pages")
	if s := h.disk["1pages"]; s.t != types.Type(listText) {
		t.Errorf("stored type %v", s.t)
	}
	rc := mkCall(types.TText, types.TNode, listText)
	got, f := run(t, h, "disk.read", rc, txt("pages"), nd(1), lst())
	if f != nil {
		t.Fatal(f)
	}
	same(t, "read", got, v)
	got, _ = run(t, h, "disk.read", rc, txt("other"), nd(1), lst(txt("d")))
	same(t, "read default", got, lst(txt("d")))
	got, _ = run(t, h, "disk.read", rc, txt("pages"), nd(2), lst(txt("d")))
	same(t, "other node", got, lst(txt("d")))

	mc := mkCall(types.TText, types.TNode, types.Map{Key: types.TText, Val: types.TText})
	_, f = run(t, h, "disk.read", mc, txt("pages"), nd(1), value.MapValue(&value.Map{}))
	wantFault(t, "disk.read", f, source.DiskType, `disk value at "pages" is a list of text, not a map of text to text`)
	h.acc = func(s, t types.Type) bool { return true }
	if _, f = run(t, h, "disk.read", mc, txt("pages"), nd(1), value.MapValue(&value.Map{})); f != nil {
		t.Error("accepts should win")
	}

	_, f = run(t, h, "disk.write", wc, v, txt("k"), nd(0))
	wantFault(t, "disk.write", f, source.Nobody, "disk.Write on sim.Nobody")
	_, f = run(t, h, "disk.read", rc, txt("k"), nd(0), lst())
	wantFault(t, "disk.read", f, source.Nobody, "disk.Read on sim.Nobody")
}
