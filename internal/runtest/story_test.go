package runtest

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/synodlang/compiler/internal/sim"
	"github.com/synodlang/compiler/internal/trace"
	"github.com/synodlang/compiler/internal/value"
)

// Story checks (plan 5.12): every demo tells the story its comments and the
// lesson notes promise. Each predicate is evaluated on the facts of a run
// and on the node state replayed from them.

const (
	ms = int64(1000)
	s  = 1000 * ms
)

// nodeState is a node as the facts leave it.
type nodeState struct {
	up      bool
	removed bool
	runs    string // name of the last package of the node's Runs
	fields  map[string]value.Value
}

// world is the state of every node by id.
type world map[int]*nodeState

// at replays the facts with T <= t (all of them when t < 0) and returns the
// state of the nodes.
func at(r *sim.Result, t int64) world {
	w := world{}
	for _, f := range r.Facts {
		if t >= 0 && f.T > t {
			break
		}
		switch f.Ev {
		case "sim.Added", "sim.Restarted":
			n := w[f.Node]
			if n == nil {
				n = &nodeState{}
				w[f.Node] = n
			}
			n.up, n.removed = true, false
			n.runs = f.Runs[len(f.Runs)-1].Name
			n.fields = map[string]value.Value{}
			i := 0
			for _, p := range f.Runs {
				for _, fld := range p.NodeFields {
					if fld.Exported {
						n.fields[p.Name+"."+fld.Name] = f.State[i]
						i++
					}
				}
			}
		case "sim.Changed":
			w[f.Node].fields[f.Field.Pkg.Name+"."+f.Field.Name] = f.New
		case "sim.Crashed":
			w[f.Node].up = false
		case "sim.Removed":
			w[f.Node].up, w[f.Node].removed = false, true
		}
	}
	return w
}

// field returns the value of "pkg.Field" of node id.
func (w world) field(id int, name string) value.Value {
	n := w[id]
	if n == nil {
		return value.Value{}
	}
	return n.fields[name]
}

// live returns the ids of the nodes that are up, in order.
func (w world) live() []int {
	var out []int
	for id := 1; id <= len(w)+1; id++ {
		if n := w[id]; n != nil && n.up {
			out = append(out, id)
		}
	}
	return out
}

// hasNode reports whether list value v holds node id.
func hasNode(v value.Value, id int) bool {
	if v.K != value.KList {
		return false
	}
	for _, it := range v.List().Items {
		if it.K == value.KNode && it.NodeID() == id {
			return true
		}
	}
	return false
}

// emitted returns the facts that emitted a record of the type.
func emitted(r *sim.Result, typ string) []trace.Fact {
	var out []trace.Fact
	for _, f := range r.Facts {
		if f.Ev == "sim.Emitted" && f.Record.Type.Name == typ {
			out = append(out, f)
		}
	}
	return out
}

// sent returns the net.Sent facts of the message type.
func sent(r *sim.Result, typ string) []trace.Fact { return netFacts(r, "net.Sent", typ) }

func netFacts(r *sim.Result, ev, typ string) []trace.Fact {
	var out []trace.Fact
	for _, f := range r.Facts {
		if f.Ev == ev && f.Msg.Type.Name == typ {
			out = append(out, f)
		}
	}
	return out
}

// rec returns the field of a record by name.
func rec(r *value.Record, name string) value.Value {
	for i, f := range r.Type.Fields {
		if f.Name == name {
			return r.Fields[i]
		}
	}
	panic("no field " + name)
}

// firstNode returns the id of the first node added that runs the package.
func nodeOf(r *sim.Result, pkg string) []int {
	var out []int
	for _, f := range r.Facts {
		if f.Ev == "sim.Added" && f.Runs[len(f.Runs)-1].Name == pkg {
			out = append(out, f.Node)
		}
	}
	return out
}

// probs collects what a story predicate finds wrong.
type probs []string

func (c *probs) errorf(format string, a ...any) { *c = append(*c, fmt.Sprintf(format, a...)) }

// noBroken reports an invariant that broke.
func (c *probs) noBroken(r *sim.Result) {
	if n := countEv(r, "sim.Broken"); n != 0 {
		c.errorf("an invariant broke %d times", n)
	}
}

// story is the predicate of one demo. variants are extra programs (the demo
// with one timing patched) that must tell their own story with the same seed.
type story struct {
	name  string
	check func(c *probs, r *sim.Result)
	// seeded reports that the demo calls sim.Seed (a seed can be searched).
	seeded   bool
	variants []variant
}

type variant struct {
	name  string
	prog  func(t testing.TB) *program
	check func(c *probs, r *sim.Result)
}

var stories = []story{
	{name: "tcp", check: storyTCP},
	{name: "gossip", check: storyGossip, seeded: true},
	{name: "paxos", check: storyPaxos, seeded: true},
	{name: "raft", check: storyRaft, seeded: true},
	{name: "twopc", check: storyTwopc, seeded: true, variants: []variant{{"crash at 80ms", twopcLate, storyTwopcLate}}},
	{name: "ring", check: storyRing},
	{name: "cassandra", check: storyCassandra, seeded: true},
	{name: "webapp", check: storyWebapp, seeded: true},
	{name: "wal", check: storyWal},
}

// holds runs the story of st on its program with the seed (nil: the demo's
// own) and returns the problems found.
func (st story) holds(t testing.TB, seed *uint64) probs {
	var c probs
	r := examples(t)[st.name].run(seed)
	if r.Failed != nil {
		c.errorf("failed: %v", r.Failed)
		return c
	}
	st.check(&c, r)
	for _, v := range st.variants {
		vr := v.prog(t).run(seed)
		if vr.Failed != nil {
			c.errorf("%s: failed: %v", v.name, vr.Failed)
			continue
		}
		var vc probs
		v.check(&vc, vr)
		for _, m := range vc {
			c.errorf("%s: %s", v.name, m)
		}
	}
	return c
}

// TestStories checks every demo on its own seed.
func TestStories(t *testing.T) {
	for _, st := range stories {
		t.Run(st.name, func(t *testing.T) {
			for _, m := range st.holds(t, nil) {
				t.Error(m)
			}
		})
	}
}

// TestFindSeeds is the helper for a demo that fails its story: with
// SYNOD_FIND_SEEDS=1 it logs the smallest seed from 1 to 1000 for which the
// story holds, for every seeded demo (plan 5.12).
func TestFindSeeds(t *testing.T) {
	if os.Getenv("SYNOD_FIND_SEEDS") == "" {
		t.Skip("set SYNOD_FIND_SEEDS=1 to search seeds")
	}
	for _, st := range stories {
		if !st.seeded {
			continue
		}
		own := examples(t)[st.name].run(nil).Header.Seed
		found := uint64(0)
		for sd := uint64(1); sd <= 1000 && found == 0; sd++ {
			if len(st.holds(t, u64(sd))) == 0 {
				found = sd
			}
		}
		t.Logf("%s: demo seed %d, story holds for it: %v, smallest seed that satisfies it: %d", st.name, own, len(st.holds(t, nil)) == 0, found)
	}
}

func storyTCP(c *probs, r *sim.Result) {
	w := at(r, 100*ms-1)
	for _, id := range []int{1, 2} {
		if got := w.field(id, "tcp.State"); got.Str != "ESTABLISHED" {
			c.errorf("n%d is %q before 100ms", id, got.Str)
		}
	}
	asked := emitted(r, "Asked")
	if len(asked) != 1 || asked[0].T != 5*ms {
		c.errorf("lesson.Asked facts: %v", asked)
		return
	}
	syn := netFacts(r, "net.Arrived", "Syn")
	if len(syn) != 1 || syn[0].T <= asked[0].T {
		c.errorf("the SYN does not arrive after the 5ms question")
	}
}

func storyGossip(c *probs, r *sim.Result) {
	// Quiet before the crash: nobody suspects anybody between 5s and 8s.
	for tm := 5 * s; tm <= 8*s; tm += 100 * ms {
		w := at(r, tm)
		for _, id := range w.live() {
			if d := w.field(id, "gossip.Down"); d.K == value.KList && len(d.List().Items) != 0 {
				c.errorf("n%d has a non-empty Down at %s", id, value.FormatTime(tm))
				return
			}
		}
	}
	// Every live node marks n3 down before 16s.
	w := at(r, 16*s-1)
	if len(w.live()) != 5 {
		c.errorf("live nodes %v", w.live())
		return
	}
	for _, id := range w.live() {
		if !hasNode(w.field(id, "gossip.Down"), 3) {
			c.errorf("n%d has not marked n3 down before 16s", id)
		}
	}
	// n3 returns with generation 2 and nobody keeps it down at 25s.
	end := at(r, 25*s)
	if g := end.field(3, "gossip.Generation"); g.Num != 2 {
		c.errorf("n3 generation %v", g.Num)
	}
	for _, id := range end.live() {
		if hasNode(end.field(id, "gossip.Down"), 3) {
			c.errorf("n%d still has n3 down at 25s", id)
		}
	}
	c.noBroken(r)
}

func storyPaxos(c *probs, r *sim.Result) {
	vals := learned(r)
	if len(vals) == 0 {
		c.errorf("nothing learned")
		return
	}
	for _, v := range vals {
		if v != vals[0] {
			c.errorf("learned %v", vals)
			return
		}
	}
	c.noBroken(r)
	// The late proposer n3 meets the accepted value and adopts it: its Accept
	// carries "x".
	for _, f := range sent(r, "Accept") {
		if f.From == 3 {
			if v := rec(f.Msg, "value").Str; v != "x" {
				c.errorf("n3 proposes %q, not x", v)
			}
			return
		}
	}
	c.errorf("n3 sent no Accept")
}

func storyRaft(c *probs, r *sim.Result) {
	leaders := func(w world, except int) (id int, term float64) {
		for _, n := range w.live() {
			if n != except && w[n].fields["raft.Role"].Str == "leader" {
				return n, w.field(n, "raft.Term").Num
			}
		}
		return 0, 0
	}
	old, oldTerm := leaders(at(r, 2*s-1), 0)
	if old == 0 {
		c.errorf("no leader before 2s")
		return
	}
	nl, newTerm := leaders(at(r, 5*s-1), old)
	if nl == 0 || newTerm <= oldTerm {
		c.errorf("new leader n%d term %v, old n%d term %v", nl, newTerm, old, oldTerm)
		return
	}
	end := at(r, -1)
	if role := end.field(old, "raft.Role").Str; role != "follower" {
		c.errorf("the old leader is %q at the end", role)
	}
	if term := end.field(old, "raft.Term").Num; term != newTerm {
		c.errorf("the old leader has term %v, the new leader %v", term, newTerm)
	}
	c.noBroken(r)
}

func storyTwopc(c *probs, r *sim.Result) {
	for _, f := range sent(r, "Decision") {
		if f.T < 45*ms {
			c.errorf("a decision was sent at %s", value.FormatTime(f.T))
		}
	}
	yes := 0
	for _, f := range sent(r, "Vote") {
		if rec(f.Msg, "yes").Int == 1 {
			yes++
		}
	}
	if yes != 3 {
		c.errorf("%d yes votes", yes)
	}
	w := at(r, 3*s)
	prepared := 0
	for _, id := range nodeOf(r, "twopc")[1:] {
		if w.field(id, "twopc.State").Str == "prepared" {
			prepared++
		}
	}
	if prepared < 1 {
		c.errorf("no participant is prepared at 3s")
	}
	if len(r.Invariants) == 0 {
		c.errorf("no invariant")
	}
	for _, inv := range r.Invariants {
		if !inv.Holds || inv.Breaks != 0 {
			c.errorf("invariant %s broke", inv.Rule)
		}
	}
}

// twopcLate is the 2PC demo with the coordinator crash moved to 80ms.
func twopcLate(t testing.TB) *program {
	dir := t.TempDir()
	copyTreeTo(t, examplesDir, dir)
	demo := filepath.Join(dir, "twopc", "demo.synod")
	b, err := os.ReadFile(demo)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "after 45ms") {
		t.Fatal("the crash is not where the test expects it")
	}
	if err := os.WriteFile(demo, []byte(strings.Replace(string(b), "after 45ms", "after 80ms", 1)), 0o644); err != nil {
		t.Fatal(err)
	}
	return check(t, dir)["twopc"]
}

// storyTwopcLate: with the crash at 80ms the decision reaches the
// participants.
func storyTwopcLate(c *probs, r *sim.Result) {
	if n := len(netFacts(r, "net.Arrived", "Decision")); n < 3 {
		c.errorf("%d decisions arrived", n)
	}
}

func storyRing(c *probs, r *sim.Result) {
	owners := [2]map[string]int{{}, {}}
	for _, f := range emitted(r, "Placed") {
		batch := 0
		if f.T >= 2*s {
			batch = 1
		}
		owners[batch][rec(f.Record, "Key").Str] = rec(f.Record, "Owner").NodeID()
	}
	if len(owners[0]) != 6 || len(owners[1]) != 6 {
		c.errorf("placements %v", owners)
		return
	}
	moved := 0
	for k, o := range owners[0] {
		if owners[1][k] != o {
			moved++
		}
	}
	if moved < 1 || moved == 6 {
		c.errorf("%d of 6 keys moved", moved)
	}
}

func storyCassandra(c *probs, r *sim.Result) {
	var coord, victim int
	for _, f := range r.Facts {
		if f.Ev == "sim.Injected" && f.Record.Type.Name == "Write" && coord == 0 {
			coord = f.Node
		}
		if f.Ev == "sim.Crashed" && victim == 0 {
			victim = f.Node
		}
	}
	if coord == 0 || victim == 0 {
		c.errorf("coordinator %d victim %d", coord, victim)
		return
	}
	if !hasNode(at(r, 12*s-1).field(coord, "gossip.Down"), victim) {
		c.errorf("n%d has not marked n%d down before 12s", coord, victim)
	}
	if n := len(emitted(r, "Written")); n != 2 {
		c.errorf("%d Written events", n)
	}
	done := emitted(r, "ReadDone")
	if len(done) != 1 || rec(done[0].Record, "Value").Str != "Grace" {
		c.errorf("ReadDone: %d events, not one with Grace", len(done))
	}
	var hints []float64
	for _, f := range r.Facts {
		if f.Ev == "sim.Changed" && f.Node == coord && f.Field.Name == "PendingHints" {
			hints = append(hints, f.New.Num)
			if f.T >= 30*s {
				c.errorf("hints changed at %s", value.FormatTime(f.T))
			}
		}
	}
	if fmt.Sprint(hints) != "[1 0]" {
		c.errorf("PendingHints went %v", hints)
	}
	data := at(r, -1).field(victim, "cassandra.Data")
	if v, ok := data.Map().Get(value.Text("user:42")); !ok || v.Str != "Grace" {
		c.errorf("the victim's Data is %s", value.Format(data, value.FormatOpts{}))
	}
	c.noBroken(r)
}

func storyWebapp(c *probs, r *sim.Result) {
	hits := 0
	for _, f := range emitted(r, "Served") {
		if rec(f.Record, "Hit").Int == 1 && f.T < 2*s {
			hits++
		}
		if id := rec(f.Record, "Id").Num; id > 100 {
			c.errorf("request %v (started after 2s) was served at %s", id, value.FormatTime(f.T))
		}
	}
	if hits == 0 {
		c.errorf("no cache hit before 2s")
	}
	store := nodeOf(r, "cache")
	if len(store) != 1 {
		c.errorf("cache nodes %v", store)
		return
	}
	drops := 0
	for _, f := range netFacts(r, "net.Dropped", "Lookup") {
		if f.To == store[0] && f.Reason == "crashed" {
			drops++
		}
	}
	if drops == 0 {
		c.errorf("no lookup was dropped by the crashed cache")
	}
}

func storyWal(c *probs, r *sim.Result) {
	rv := emitted(r, "Recovered")
	if len(rv) != 2 || rv[1].T != 3500*ms || rec(rv[1].Record, "Replayed").Num != 4 {
		c.errorf("Recovered: %d events, not a second with Replayed 4 at 3.5s", len(rv))
	}
	table := at(r, -1).field(1, "wal.Table").Map()
	for i := 1; i <= 12; i++ {
		_, ok := table.Get(value.Text(fmt.Sprintf("k%d", i)))
		if want := i != 11; ok != want {
			c.errorf("k%d present: %v", i, ok)
		}
	}
}
