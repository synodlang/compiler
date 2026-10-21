package trace

import (
	"bytes"
	"flag"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/synodlang/compiler/internal/ir"
	"github.com/synodlang/compiler/internal/source"
	"github.com/synodlang/compiler/internal/value"
)

var update = flag.Bool("update", false, "rewrite testdata goldens")

// fixture: package paxos with node fields and records; package std/net.
var (
	pxNet   = &ir.Package{Path: "std/net", Name: "net", IsStd: true}
	pxPaxos = &ir.Package{Path: "examples/paxos", Name: "paxos"}
	prepare = &ir.Record{Pkg: pxPaxos, Name: "Prepare", Exported: true}
	fPromis = &ir.Field{Pkg: pxPaxos, Name: "Promised", Exported: true}
	fSecret = &ir.Field{Pkg: pxPaxos, Name: "secret", Exported: false}
	fLog    = &ir.Field{Pkg: pxPaxos, Name: "Log", Exported: true}
)

func init() {
	prepare.Fields = []*ir.Field{
		{Pkg: pxPaxos, Record: prepare, Name: "number", Exported: true, Index: 0},
		{Pkg: pxPaxos, Record: prepare, Name: "secret", Exported: false, Index: 1},
	}
	pxPaxos.Records = []*ir.Record{prepare}
	pxPaxos.NodeFields = []*ir.Field{fPromis, fSecret, fLog}
	pxPaxos.Runs = []*ir.Package{pxNet, pxPaxos}
}

func prep(n float64, secret string) *value.Record {
	return value.RecordOf(prepare, []value.Value{value.Number(n), value.Text(secret)}).Record()
}

func enc(t *testing.T, facts []Fact, o Options) string {
	t.Helper()
	var b bytes.Buffer
	h := Header{Seed: 7, Program: "examples/paxos", Packages: []string{"paxos", "std/net", "std/sim"}}
	if err := Encode(&b, h, facts, o); err != nil {
		t.Fatal(err)
	}
	return b.String()
}

const wantHeader = `{"format":"synod-trace/1","synod":"0.2","seed":7,"program":"examples/paxos","packages":["paxos","std/net","std/sim"]}` + "\n"

func TestEncodeEvents(t *testing.T) {
	o := Options{Paths: map[string]string{"../x/paxos.synod": "paxos/paxos.synod"}}
	msgs := prep(101, "hid")
	m := (&value.Map{}).With(value.Node(2), value.Number(1.5)).With(value.Node(1), value.Text("a\"b"))
	state := []value.Value{value.Number(0), value.ListOf(nil)}
	cases := []struct {
		name string
		f    Fact
		want string
	}{
		{"started", Fact{Step: 0, T: 0, Ev: "sim.Started", Node: -1}, `{"seq":1,"t":0,"ev":"sim.Started"}`},
		{"ended", Fact{T: 5, Ev: "sim.Ended", Node: -1, Reason: "stop"}, `{"seq":1,"t":5,"ev":"sim.Ended","reason":"stop"}`},
		{"added", Fact{T: 1, Ev: "sim.Added", Node: 1, Runs: pxPaxos.Runs, State: state},
			`{"seq":1,"t":1,"ev":"sim.Added","node":"n1","runs":["std/net","examples/paxos"],"state":{"paxos.Promised":0,"paxos.Log":[]}}`},
		{"restarted", Fact{T: 1, Ev: "sim.Restarted", Node: 3, Runs: []*ir.Package{pxNet}},
			`{"seq":1,"t":1,"ev":"sim.Restarted","node":"n3","runs":["std/net"],"state":{}}`},
		{"removed", Fact{T: 2, Ev: "sim.Removed", Node: 2}, `{"seq":1,"t":2,"ev":"sim.Removed","node":"n2"}`},
		{"crashed", Fact{T: 2, Ev: "sim.Crashed", Node: 2}, `{"seq":1,"t":2,"ev":"sim.Crashed","node":"n2"}`},
		{"fired", Fact{T: 2, Ev: "sim.Fired", Node: 2}, `{"seq":1,"t":2,"ev":"sim.Fired","node":"n2"}`},
		{"changed", Fact{T: 31877, Ev: "sim.Changed", Node: 3, Field: fPromis, Old: value.Number(0), New: value.Number(101)},
			`{"seq":1,"t":31877,"ev":"sim.Changed","node":"n3","field":"paxos.Promised","old":0,"new":101}`},
		{"injected", Fact{T: 9, Ev: "sim.Injected", Node: 1, Record: msgs},
			`{"seq":1,"t":9,"ev":"sim.Injected","node":"n1","record":{"type":"paxos.Prepare","fields":{"number":101,"secret":"…"}}}`},
		{"emitted", Fact{T: 9, Ev: "sim.Emitted", Node: 2, Record: msgs},
			`{"seq":1,"t":9,"ev":"sim.Emitted","node":"n2","record":{"type":"paxos.Prepare","fields":{"number":101,"secret":"…"}}}`},
		{"emitted nobody", Fact{T: 9, Ev: "sim.Emitted", Node: 0, Record: msgs},
			`{"seq":1,"t":9,"ev":"sim.Emitted","node":null,"record":{"type":"paxos.Prepare","fields":{"number":101,"secret":"…"}}}`},
		{"broken", Fact{T: 90000, Ev: "sim.Broken", Node: -1, Rule: "../x/paxos.synod:109: always (a)", RulePos: ir.Pos{File: "../x/paxos.synod", Line: 109, Col: 1}},
			`{"seq":1,"t":90000,"ev":"sim.Broken","rule":"paxos/paxos.synod:109: always (a)"}`},
		{"failed pos", Fact{T: 4, Ev: "sim.Failed", Node: -1, Fault: &value.Fault{Code: source.IndexRange, Message: "index 3 is \"out\"", Pos: ir.Pos{File: "../x/paxos.synod", Line: 40, Col: 5}, HasPos: true}},
			`{"seq":1,"t":4,"ev":"sim.Failed","code":"index-range","message":"index 3 is \"out\"","pos":"paxos/paxos.synod:40:5"}`},
		{"failed no pos", Fact{T: 600000000, Ev: "sim.Failed", Node: -1, Fault: &value.Fault{Code: source.TimeLimit, Message: "limit"}},
			`{"seq":1,"t":600000000,"ev":"sim.Failed","code":"time-limit","message":"limit","pos":null}`},
		{"sent", Fact{T: 20431, Ev: "net.Sent", Node: 1, Msg: msgs, MsgID: 5, From: 1, To: 3},
			`{"seq":1,"t":20431,"ev":"net.Sent","node":"n1","msg":{"id":5,"type":"paxos.Prepare","fields":{"number":101,"secret":"…"}},"from":"n1","to":"n3"}`},
		{"arrived", Fact{T: 31877, Ev: "net.Arrived", Node: 3, Msg: msgs, MsgID: 5, From: 1, To: 3},
			`{"seq":1,"t":31877,"ev":"net.Arrived","node":"n3","msg":{"id":5,"type":"paxos.Prepare","fields":{"number":101,"secret":"…"}},"from":"n1","to":"n3"}`},
		{"dropped", Fact{T: 7, Ev: "net.Dropped", Node: 3, Msg: msgs, MsgID: 6, From: 1, To: 3, Reason: "partitioned"},
			`{"seq":1,"t":7,"ev":"net.Dropped","node":"n3","msg":{"id":6,"type":"paxos.Prepare","fields":{"number":101,"secret":"…"}},"from":"n1","to":"n3","reason":"partitioned"}`},
		{"partitioned", Fact{T: 8, Ev: "net.Partitioned", Node: -1, A: []int{1, 2}, B: []int{3}},
			`{"seq":1,"t":8,"ev":"net.Partitioned","a":["n1","n2"],"b":["n3"]}`},
		{"healed", Fact{T: 8, Ev: "net.Healed", Node: -1}, `{"seq":1,"t":8,"ev":"net.Healed"}`},
		{"disk", Fact{T: 8, Ev: "disk.Written", Node: 2, Key: "pa\tge\x01"}, `{"seq":1,"t":8,"ev":"disk.Written","node":"n2","key":"pa\tge\u0001"}`},
		{"values", Fact{T: 1, Ev: "sim.Changed", Node: 1, Field: fLog,
			Old: value.ListOf([]value.Value{value.Time(20000), value.Node(3), value.Node(0), value.Bool(true), value.Number(1e21), value.Number(-0.5)}),
			New: value.MapValue(m)},
			`{"seq":1,"t":1,"ev":"sim.Changed","node":"n1","field":"paxos.Log","old":[{"time":20000},{"node":"n3"},{"node":null},true,1e+21,-0.5],"new":{"map":[[{"node":"n1"},"a\"b"],[{"node":"n2"},1.5]]}}`},
		{"escapes", Fact{T: 1, Ev: "disk.Written", Node: 1, Key: "\\ \n \r \x1f é </> \u2028 \xff"},
			"{\"seq\":1,\"t\":1,\"ev\":\"disk.Written\",\"node\":\"n1\",\"key\":\"\\\\ \\n \\r \\u001f é </> \u2028 \xef\xbf\xbd\"}"},
		{"nested record", Fact{T: 1, Ev: "sim.Injected", Node: 1, Record: value.RecordOf(prepare, []value.Value{value.MapValue(&value.Map{}), value.Text("")}).Record()},
			`{"seq":1,"t":1,"ev":"sim.Injected","node":"n1","record":{"type":"paxos.Prepare","fields":{"number":{"map":[]},"secret":"…"}}}`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := enc(t, []Fact{c.f}, o)
			if want := wantHeader + c.want + "\n"; got != want {
				t.Errorf("got\n%s\nwant\n%s", got, want)
			}
		})
	}
}

func TestEncodeSeqAndQual(t *testing.T) {
	o := Options{Qual: func(p *ir.Package) string { return "Q" + p.Name }}
	got := enc(t, []Fact{
		{T: 0, Ev: "sim.Started", Node: -1},
		{T: 1, Ev: "sim.Changed", Node: 1, Field: fPromis, Old: value.Number(0), New: value.Number(1)},
		{T: 2, Ev: "net.Healed", Node: -1},
	}, o)
	want := wantHeader +
		`{"seq":1,"t":0,"ev":"sim.Started"}` + "\n" +
		`{"seq":2,"t":1,"ev":"sim.Changed","node":"n1","field":"Qpaxos.Promised","old":0,"new":1}` + "\n" +
		`{"seq":3,"t":2,"ev":"net.Healed"}` + "\n"
	if got != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}
}

func TestEncodeErrors(t *testing.T) {
	bad := []Fact{
		{Ev: "nope", Node: -1},
		{Ev: "net.Sent", Node: 1},
		{Ev: "sim.Failed", Node: -1},
		{Ev: "sim.Changed", Node: 1, Old: value.Number(1), New: value.Number(1)},
		{Ev: "sim.Changed", Node: 1, Field: fPromis, Old: value.Number(math.Inf(1)), New: value.Number(1)},
		{Ev: "sim.Added", Node: 1, Runs: pxPaxos.Runs},
	}
	for i, f := range bad {
		if err := Encode(&bytes.Buffer{}, Header{}, []Fact{f}, Options{}); err == nil {
			t.Errorf("case %d: want an error", i)
		}
	}
}

func TestEncodeDeterministic(t *testing.T) {
	m := &value.Map{}
	for i := 9; i >= 0; i-- {
		m = m.With(value.Node(i+1), value.Number(float64(i)))
	}
	f := []Fact{{T: 1, Ev: "sim.Changed", Node: 1, Field: fLog, Old: value.MapValue(m), New: value.MapValue(m)}}
	if a, b := enc(t, f, Options{}), enc(t, f, Options{}); a != b || !strings.HasPrefix(a, wantHeader) {
		t.Error("not deterministic")
	}
}

func golden(t *testing.T, name, got string) {
	t.Helper()
	p := filepath.Join("testdata", name)
	if *update {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if string(want) != got {
		t.Errorf("%s differs from golden; run with -update\n--- got\n%s", name, got)
	}
}
