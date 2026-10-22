package scene

import (
	"fmt"
	"strings"

	"github.com/synodlang/compiler/internal/ir"
	"github.com/synodlang/compiler/internal/source"
	"github.com/synodlang/compiler/internal/trace"
	"github.com/synodlang/compiler/internal/value"
)

// Options configure Build; zero fields take the defaults shown.
type Options struct {
	// Pace is the playback speed in thousandths: 1000 is real speed, 2000
	// twice as fast. Valid values are 250 to 4000; 0 means 1000, and any
	// other value is clamped into the range.
	Pace int
	// Mode is Ordered (default) or Proportional.
	Mode Mode
	// MaxShapes bounds the shapes (1_000_000), MaxTimersPerInstant the
	// timers that fire at one presentation instant (100_000), MaxDepth the
	// call depth (value.MaxCallDepth, 500) and MaxKeys the keyframes
	// (10_000_000). Going over is a run-limit look fault.
	MaxShapes, MaxTimersPerInstant, MaxDepth, MaxKeys int
}

// Input is what Build presents.
type Input struct {
	// Prog is the program whose observers draw the run. Its IR shares
	// packages, records and fields with the program that produced Facts.
	Prog *ir.Program
	// Look is the default look (std/look/basic) when !UsesLook(Prog), else
	// nil. Its observers run before those of Prog.
	Look *ir.Program
	// Facts are the facts of the run (from the simulator or trace.Decode).
	Facts []trace.Fact
	// Trace names files and packages the way trace.jsonl does; it gives the
	// paths of shape ids, markers and faults, and the package qualifiers of
	// color names.
	Trace trace.Options
}

// Failure is a look fault: a run-time error in look context. It stops the
// scene at the fact being drawn (or, for a timer, at the last fact before
// it); the scene returned with it holds everything drawn up to then and a
// "failed" marker. It is never part of the trace.
type Failure struct {
	// Seq is the sequence number (from 1) of the fact being presented, 0
	// before the first fact; T its simulated time in microseconds.
	Seq int
	T   int64
	// Fault is the error; its position is a display path of the IR (use
	// Trace.Paths to print it the way the trace does).
	Fault *value.Fault
}

// UsesLook reports whether p uses std/look (spec/06 section 6.6): when it
// does, the program brings its own looks and the default look is not added.
func UsesLook(p *ir.Program) bool {
	for _, pk := range p.Packages {
		if pk.Path == "std/look" {
			return true
		}
	}
	return false
}

// builder is the state of one Build: the look interpreter on top of the
// engine (clock, replay store, world).
type builder struct {
	in    Input
	o     Options
	facts []trace.Fact
	obs   []*ir.Handler
	qual  func(*ir.Package) string

	byPath map[string]*ir.Package
	clock  *clock
	store  *store
	world  *world
	ids    idGen
	hexes  map[string]string
	inits  map[*ir.Field]value.Value
	host   lookHost

	timers   timerHeap
	timerSeq int
	instAt   int64
	instN    int

	// now is the presentation time of the activation that is running; seq
	// and curT are the sequence number and time of the last fact started.
	now  int64
	seq  int
	curT int64

	// The activation that is running.
	act   *activation
	pos   ir.Pos // the site of act
	cur   *ir.Proc
	depth int
	inst  *instance
	track *instance

	instances []*instance
	index     map[readKey][]int32
	attached  map[int]bool

	slots   []trace.SlotEntry
	markers []trace.Marker
	failure *Failure
}

// Build presents the facts of a run as a scene. When a look fails it returns
// the scene drawn up to that point together with the Failure; otherwise the
// Failure is nil. Build is deterministic: the same input gives the same
// scene, byte for byte once encoded.
func Build(in Input, o Options) (*trace.Scene, *Failure) {
	b := newBuilder(in, o)
	b.run()
	return b.finish(), b.failure
}

func newBuilder(in Input, o Options) *builder {
	if o.Pace == 0 {
		o.Pace = 1000
	}
	o.Pace = max(250, min(4000, o.Pace))
	if o.MaxTimersPerInstant <= 0 {
		o.MaxTimersPerInstant = 100_000
	}
	if o.MaxDepth <= 0 {
		o.MaxDepth = value.MaxCallDepth
	}
	b := &builder{in: in, o: o, byPath: map[string]*ir.Package{},
		hexes: map[string]string{}, inits: map[*ir.Field]value.Value{},
		index: map[readKey][]int32{}, attached: map[int]bool{}}
	b.host.b = b
	b.qual = in.Trace.Qual
	if b.qual == nil {
		b.qual = func(p *ir.Package) string { return p.Name }
	}
	b.facts = maskFacts(in.Facts)
	var end int64
	if n := len(b.facts); n > 0 {
		end = b.facts[n-1].T
	}
	b.clock = newClock(o.Pace, o.Mode, end)
	b.store = newStore()
	b.world = newWorld(b.clock, worldConfig{MaxShapes: o.MaxShapes, MaxKeys: o.MaxKeys})
	for _, p := range []*ir.Program{in.Look, in.Prog} {
		if p == nil {
			continue
		}
		for _, pk := range p.Packages {
			if _, ok := b.byPath[pk.Path]; !ok {
				b.byPath[pk.Path] = pk
			}
		}
		b.obs = append(b.obs, p.Observers...)
	}
	return b
}

// tpath is the trace path of a display path.
func (b *builder) tpath(display string) string {
	if t, ok := b.in.Trace.Paths[display]; ok {
		return t
	}
	return display
}

// tpos is "path:line:col" with the trace path.
func (b *builder) tpos(p ir.Pos) string {
	return fmt.Sprintf("%s:%d:%d", b.tpath(p.File), p.Line, p.Col)
}

// begin starts an activation of the site at p.
func (b *builder) begin(p ir.Pos) *activation {
	b.pos = p
	return b.ids.begin(b.tpath(p.File), p.Line, p.Col)
}

// run presents every fact, then the timers of the tail.
func (b *builder) run() {
	for i := range b.facts {
		f := &b.facts[i]
		seq := i + 1
		ch := b.store.changes(*f)
		var s int64
		for {
			s = b.schedule(f, ch)
			if t := b.timers.peek(); t != nil && t.at <= s {
				if !b.fireTimer() {
					return
				}
				continue
			}
			break
		}
		b.clock.commit(s, f.T)
		b.slots = append(b.slots, trace.SlotEntry{Seq: seq, At: s})
		b.seq, b.curT, b.now = seq, f.T, s
		b.store.apply(*f)
		switch f.Ev {
		case "net.Sent":
			b.clock.noteSent(f.MsgID, s)
		case "net.Arrived":
			b.world.land(f.MsgID, s, false)
		case "net.Dropped":
			b.world.land(f.MsgID, s, true)
		}
		b.markFact(f, seq, s)
		if flt := b.observe(f); flt != nil {
			b.lookFault(flt)
			return
		}
		if flt := b.rerunReactive(ch); flt != nil {
			b.lookFault(flt)
			return
		}
		if flt := b.reflow(b.pos); flt != nil {
			b.lookFault(flt)
			return
		}
	}
	limit := b.clock.now() + b.clock.scale(TimerTail)
	for {
		t := b.timers.peek()
		if t == nil || t.at > limit {
			return
		}
		if !b.fireTimer() {
			return
		}
	}
}

// schedule decides the slot of fact f: ch are the store keys it changes.
func (b *builder) schedule(f *trace.Fact, ch []readKey) int64 {
	var busy int64
	w := b.world
	touch := func(n int) {
		if n <= 0 {
			return
		}
		for _, i := range w.attachedTo(nodeKey(n)) {
			if x := w.busyUntil(i); x > busy {
				busy = x
			}
		}
	}
	touch(f.Node)
	touch(f.From)
	touch(f.To)
	for _, n := range f.A {
		touch(n)
	}
	for _, n := range f.B {
		touch(n)
	}
	for _, k := range ch {
		for _, id := range b.index[k] {
			in := b.instances[id]
			if in.dead {
				continue
			}
			if _, ok := in.reads[k]; !ok {
				continue
			}
			if x := w.busyUntil(in.owner); x > busy {
				busy = x
			}
		}
	}
	landing := f.Ev == "net.Arrived" || f.Ev == "net.Dropped"
	return b.clock.schedule(f.T, busy, landing, f.MsgID)
}

// fireTimer runs the earliest timer; false means a look fault stopped the
// scene.
func (b *builder) fireTimer() bool {
	t := b.timers.pop()
	if t.at == b.instAt && b.instN > 0 {
		b.instN++
	} else {
		b.instAt, b.instN = t.at, 1
	}
	b.now = t.at
	if b.instN > b.o.MaxTimersPerInstant {
		b.lookFault(value.NewFault(source.RunLimit, t.pos, "more than %d timers fired at one instant", b.o.MaxTimersPerInstant).
			WithLabel(fmt.Sprintf("timer over %d at one instant", b.o.MaxTimersPerInstant)))
		return false
	}
	b.act, b.cur, b.inst, b.track, b.depth = b.begin(t.pos), t.cur, nil, nil, 0
	_, _, flt := b.execBlock(&ex{fr: append([]value.Value(nil), t.frame...)}, t.body)
	if flt == nil {
		flt = b.reflow(t.pos)
	}
	if flt != nil {
		b.lookFault(flt)
		return false
	}
	return true
}

// reflow re-flows the layout; a world error becomes a fault at p.
func (b *builder) reflow(p ir.Pos) *value.Fault {
	if err := b.world.reflow(b.now, b.store); err != nil {
		return b.worldFault(err, p)
	}
	return nil
}

// worldFault turns an error of the world into a look fault at p.
func (b *builder) worldFault(err error, p ir.Pos) *value.Fault {
	if we, ok := err.(*worldErr); ok {
		return value.NewFault(source.Code(we.Code), p, "%s", we.Msg)
	}
	panic(fmt.Sprintf("scene: unexpected error %v", err))
}

// lookFault records the failure and its marker.
func (b *builder) lookFault(f *value.Fault) {
	b.failure = &Failure{Seq: b.seq, T: b.curT, Fault: f}
	pos := ""
	if f.HasPos {
		pos = b.tpos(f.Pos) + ": "
	}
	b.markers = append(b.markers, trace.Marker{At: b.now, Seq: b.seq, Kind: "failed",
		Text: "look " + pos + "error[" + string(f.Code) + "]: " + f.Message})
}

// markFact adds the markers fact f raises.
func (b *builder) markFact(f *trace.Fact, seq int, at int64) {
	add := func(kind, text string, choices ...string) {
		b.markers = append(b.markers, trace.Marker{At: at, Seq: seq, Kind: kind, Text: text, Choices: choices})
	}
	switch f.Ev {
	case "sim.Emitted":
		r := f.Record
		if r == nil || r.Type.Pkg.Path != "std/lesson" {
			return
		}
		field, kind := "", ""
		switch r.Type.Name {
		case "Asked":
			field, kind = "Question", "pause"
		case "Noted":
			field, kind = "Text", "caption"
		default:
			return
		}
		for i, fl := range r.Type.Fields {
			if fl.Name == field && r.Fields[i].K == value.KText {
				var choices []string
				if kind == "pause" {
					choices = askedChoices(r)
				}
				add(kind, r.Fields[i].Str, choices...)
			}
		}
	case "sim.Broken":
		rule := trace.RuleText(*f, b.in.Trace)
		if i := strings.Index(rule, ": always "); i >= 0 {
			rule = rule[i+2:]
		}
		add("broken", rule)
	case "sim.Failed":
		if f.Fault == nil {
			return
		}
		pos := ""
		if f.Fault.HasPos {
			pos = b.tpos(f.Fault.Pos) + ": "
		}
		add("failed", pos+"error["+string(f.Fault.Code)+"]: "+f.Fault.Message)
	}
}

// finish stops unlanded flights and converts the world to a scene.
func (b *builder) finish() *trace.Scene {
	last := b.clock.now()
	b.world.finalize(last)
	return b.world.emit(emitInput{Slots: b.slots, Markers: b.markers, Cursor: last, Hex: b.colorHex})
}

// askedChoices returns the texts in the Choices field of an Asked record.
func askedChoices(r *value.Record) []string {
	var out []string
	for i, fl := range r.Type.Fields {
		if fl.Name == "Choices" && r.Fields[i].K == value.KList {
			for _, it := range r.Fields[i].List().Items {
				if it.K == value.KText {
					out = append(out, it.Str)
				}
			}
		}
	}
	return out
}
