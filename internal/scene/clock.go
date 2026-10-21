package scene

// Mode is the presentation mode.
type Mode uint8

// The modes: Ordered spaces the facts by their gaps; Proportional spreads
// them over ProportionalLen ms by simulated time.
const (
	Ordered Mode = iota
	Proportional
)

// clock decides the presentation slot of each fact (plan 2.4). Time is int64
// ms; pace is in thousandths (1000 is real speed, 2000 twice as fast).
//
// Use: for each fact, s := c.schedule(...); then c.commit(s, f.T); after a
// net.Sent, c.noteSent(id, s).
type clock struct {
	pace int64
	mode Mode
	// end is the time of the last fact in microseconds (proportional mode).
	end int64

	cursor  int64
	prevT   int64
	started bool
	sent    map[int]int64
}

// newClock makes a clock. pace 0 means 1000. endT is the time of the last
// fact in microseconds; only proportional mode reads it.
func newClock(pace int, mode Mode, endT int64) *clock {
	if pace <= 0 {
		pace = 1000
	}
	return &clock{pace: int64(pace), mode: mode, end: endT, sent: map[int]int64{}}
}

// scale converts a presentation duration in ms to ms at this pace, rounding
// down: ms*1000/pace.
func (c *clock) scale(ms int64) int64 { return ms * 1000 / c.pace }

// now is the slot of the last committed fact (0 before the first).
func (c *clock) now() int64 { return c.cursor }

// schedule returns the slot of the next fact: tUs is its time in
// microseconds; busy is busyUntil of the shapes it touches (0 for none);
// landing says the fact is a net.Arrived or net.Dropped, whose message id is
// msgID. It does not change the clock.
//
// Ordered rules: 1 the first fact is at 0; 2 the gap to the previous fact is
// multiplied by GapScale and capped at MaxIdle (before pace); 3 touched
// shapes must be idle (busy); 4 a landing is at least MinFlight after its
// send; 5 slots are at least MinStep apart. Proportional mode keeps rules 1
// and 5 only in the weak form "not before the cursor".
func (c *clock) schedule(tUs int64, busy int64, landing bool, msgID int) int64 {
	if !c.started {
		return 0
	}
	if c.mode == Proportional {
		var slot int64
		if c.end > 0 {
			slot = tUs * ProportionalLen / c.end
		}
		s := c.scale(slot)
		if s < c.cursor {
			s = c.cursor
		}
		return s
	}
	gap := (tUs - c.prevT) * GapScale / 1000
	if gap < 0 {
		gap = 0
	}
	if gap > MaxIdle {
		gap = MaxIdle
	}
	s := c.cursor + c.scale(gap)
	if busy > s {
		s = busy
	}
	if landing {
		if at, ok := c.sent[msgID]; ok {
			if m := at + c.scale(MinFlight); m > s {
				s = m
			}
		}
	}
	if m := c.cursor + c.scale(MinStep); m > s {
		s = m
	}
	return s
}

// commit records the slot of the fact just scheduled.
func (c *clock) commit(slot, tUs int64) {
	c.cursor, c.prevT, c.started = slot, tUs, true
}

// noteSent records the slot of a net.Sent; sentSlot reads it.
func (c *clock) noteSent(id int, slot int64) { c.sent[id] = slot }

func (c *clock) sentSlot(id int) (int64, bool) {
	s, ok := c.sent[id]
	return s, ok
}
