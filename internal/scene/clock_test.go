package scene

import "testing"

func TestScale(t *testing.T) {
	for _, tc := range []struct {
		pace     int
		ms, want int64
	}{
		{1000, 300, 300}, {2000, 300, 150}, {4000, 300, 75}, {250, 300, 1200},
		{3000, 100, 33}, {1000, 0, 0},
	} {
		if got := newClock(tc.pace, Ordered, 0).scale(tc.ms); got != tc.want {
			t.Errorf("pace %d scale(%d) = %d, want %d", tc.pace, tc.ms, got, tc.want)
		}
	}
	if newClock(0, Ordered, 0).scale(100) != 100 {
		t.Error("pace 0 means 1000")
	}
}

// step schedules and commits one fact.
func step(c *clock, tUs, busy int64, landing bool, id int) int64 {
	s := c.schedule(tUs, busy, landing, id)
	c.commit(s, tUs)
	return s
}

func TestScheduleRules(t *testing.T) {
	for _, pace := range []int{250, 1000, 2000, 4000} {
		c := newClock(pace, Ordered, 0)
		sc := c.scale
		// rule 1: the first fact is at 0, whatever its time
		if s := step(c, 5_000_000, 0, false, 0); s != 0 {
			t.Fatalf("pace %d: first slot %d", pace, s)
		}
		// rule 5: a fact at the same time is MinStep later
		s2 := step(c, 5_000_000, 0, false, 0)
		if s2 != sc(MinStep) {
			t.Errorf("pace %d: min step %d want %d", pace, s2, sc(MinStep))
		}
		// rule 2: gap 10 ms of simulated time -> 80 ms
		s3 := step(c, 5_010_000, 0, false, 0)
		if s3 != s2+sc(80) {
			t.Errorf("pace %d: gap %d want %d", pace, s3, s2+sc(80))
		}
		// rule 2 cap: an hour of silence is MaxIdle
		s4 := step(c, 3_600_000_000, 0, false, 0)
		if s4 != s3+sc(MaxIdle) {
			t.Errorf("pace %d: cap %d want %d", pace, s4, s3+sc(MaxIdle))
		}
		// rule 3: busy shapes delay the slot
		busy := s4 + sc(5000)
		s5 := step(c, 3_600_001_000, busy, false, 0)
		if s5 != busy {
			t.Errorf("pace %d: busy %d want %d", pace, s5, busy)
		}
		// rule 4: a landing waits MinFlight after its send
		c.noteSent(7, s5)
		s6 := step(c, 3_600_002_000, 0, true, 7)
		if s6 != s5+sc(MinFlight) {
			t.Errorf("pace %d: flight %d want %d", pace, s6, s5+sc(MinFlight))
		}
		if at, ok := c.sentSlot(7); !ok || at != s5 {
			t.Errorf("sentSlot = %d, %v", at, ok)
		}
		// a landing of an unknown message is not delayed by rule 4
		s7 := step(c, 3_600_003_000, 0, true, 99)
		if s7 != s6+sc(MinStep) {
			t.Errorf("pace %d: unknown landing %d", pace, s7)
		}
	}
}

func TestScheduleStrictlyIncreasing(t *testing.T) {
	c := newClock(4000, Ordered, 0)
	prev := int64(-1)
	for i := 0; i < 50; i++ {
		s := step(c, int64(i)*100, 0, false, 0)
		if s <= prev {
			t.Fatalf("slot %d not after %d", s, prev)
		}
		prev = s
	}
}

func TestScheduleProportional(t *testing.T) {
	end := int64(10_000_000)
	for _, pace := range []int{250, 1000, 2000, 4000} {
		c := newClock(pace, Proportional, end)
		if s := step(c, 1_000_000, 0, false, 0); s != 0 {
			t.Fatalf("first %d", s)
		}
		want := c.scale(ProportionalLen / 2)
		if s := step(c, 5_000_000, 0, false, 0); s != want {
			t.Errorf("pace %d: mid %d want %d", pace, s, want)
		}
		// equal slots allowed, never backwards
		if s := step(c, 5_000_000, 0, false, 0); s != want {
			t.Errorf("pace %d: equal slot %d want %d", pace, s, want)
		}
		if s := step(c, 10_000_000, 1<<40, true, 1); s != c.scale(ProportionalLen) {
			t.Errorf("pace %d: end %d (busy and flight rules do not apply)", pace, s)
		}
	}
	// End = 0: every slot is 0
	c := newClock(1000, Proportional, 0)
	for i := 0; i < 3; i++ {
		if s := step(c, 0, 0, false, 0); s != 0 {
			t.Fatalf("End=0 slot %d", s)
		}
	}
}
