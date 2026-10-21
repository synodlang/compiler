package scene

import (
	"github.com/synodlang/compiler/internal/ir"
	"github.com/synodlang/compiler/internal/value"
)

// timer is a presentation timer: an "after" of a look, due at presentation
// time at. Timers fire in (at, seq) order.
type timer struct {
	at    int64
	seq   int
	body  *ir.Block
	frame []value.Value
	pos   ir.Pos
	cur   *ir.Proc
}

// timerHeap is a binary min-heap of timers ordered by (at, seq).
type timerHeap struct{ a []*timer }

func timerLess(x, y *timer) bool {
	if x.at != y.at {
		return x.at < y.at
	}
	return x.seq < y.seq
}

func (h *timerHeap) len() int { return len(h.a) }

// peek returns the earliest timer, or nil.
func (h *timerHeap) peek() *timer {
	if len(h.a) == 0 {
		return nil
	}
	return h.a[0]
}

func (h *timerHeap) push(t *timer) {
	h.a = append(h.a, t)
	i := len(h.a) - 1
	for i > 0 {
		p := (i - 1) / 2
		if !timerLess(h.a[i], h.a[p]) {
			break
		}
		h.a[i], h.a[p] = h.a[p], h.a[i]
		i = p
	}
}

func (h *timerHeap) pop() *timer {
	top := h.a[0]
	n := len(h.a) - 1
	h.a[0] = h.a[n]
	h.a[n] = nil
	h.a = h.a[:n]
	i := 0
	for {
		l, r, m := 2*i+1, 2*i+2, i
		if l < n && timerLess(h.a[l], h.a[m]) {
			m = l
		}
		if r < n && timerLess(h.a[r], h.a[m]) {
			m = r
		}
		if m == i {
			break
		}
		h.a[i], h.a[m] = h.a[m], h.a[i]
		i = m
	}
	return top
}
