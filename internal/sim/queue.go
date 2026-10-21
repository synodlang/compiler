package sim

import (
	"github.com/synodlang/compiler/internal/ir"
	"github.com/synodlang/compiler/internal/value"
)

// evKind is the kind of a queued event.
type evKind uint8

const (
	evStart   evKind = iota // a node's start
	evDeliver               // sim.Delivered of a record injected by main
	evReceive               // a message reaches its target
	evDrop                  // a message that was lost on its way
	evTimer                 // an after block
)

// event is one entry of the queue. Only the fields of its kind are used.
type event struct {
	at   int64
	seq  uint64
	kind evKind
	// node is the target (evStart, evDeliver, evReceive, evDrop) or the node
	// of a timer (0 for a timer of main); inc is the node's incarnation when
	// the event was queued.
	node, inc int
	rec       *value.Record
	from, id  int
	reason    string
	body      *ir.Block
	frame     []value.Value
	// pos is the code that queued the event.
	pos ir.Pos
}

// queue is a binary heap ordered by (at, seq).
type queue struct {
	h   []*event
	seq uint64
}

func less(a, b *event) bool {
	if a.at != b.at {
		return a.at < b.at
	}
	return a.seq < b.seq
}

// push adds e, giving it the next sequence number.
func (q *queue) push(e *event) {
	q.seq++
	e.seq = q.seq
	q.h = append(q.h, e)
	i := len(q.h) - 1
	for i > 0 {
		p := (i - 1) / 2
		if !less(q.h[i], q.h[p]) {
			break
		}
		q.h[i], q.h[p] = q.h[p], q.h[i]
		i = p
	}
}

// peek returns the first event without removing it.
func (q *queue) peek() (*event, bool) {
	if len(q.h) == 0 {
		return nil, false
	}
	return q.h[0], true
}

// pop removes and returns the first event.
func (q *queue) pop() *event {
	top := q.h[0]
	n := len(q.h) - 1
	q.h[0] = q.h[n]
	q.h[n] = nil
	q.h = q.h[:n]
	i := 0
	for {
		l, r, m := 2*i+1, 2*i+2, i
		if l < n && less(q.h[l], q.h[m]) {
			m = l
		}
		if r < n && less(q.h[r], q.h[m]) {
			m = r
		}
		if m == i {
			break
		}
		q.h[i], q.h[m] = q.h[m], q.h[i]
		i = m
	}
	return top
}
