package scene

import (
	"sort"

	"github.com/synodlang/compiler/internal/ir"
	"github.com/synodlang/compiler/internal/value"
)

// instance is a reactive block of a draw (spec/09): it runs once when the
// shape is drawn and again whenever state it read changes, always in
// creation order. Shapes the block draws without attaching them are owned by
// the instance and deleted before each re-run, together with the instances
// the block created.
type instance struct {
	id    int32
	block *ir.Block
	frame []value.Value
	// owner is the drawn shape: it is busy while the block's tweens run.
	owner int
	pos   ir.Pos
	cur   *ir.Proc
	// reads is the read-set of the last run; indexed every key ever put in
	// b.index for this instance.
	reads    map[readKey]struct{}
	indexed  map[readKey]struct{}
	owned    []int
	children []*instance
	dead     bool
}

// read records that the running reactive block reads k.
func (b *builder) read(k readKey) {
	in := b.track
	if in == nil {
		return
	}
	if _, ok := in.reads[k]; ok {
		return
	}
	in.reads[k] = struct{}{}
	if _, ok := in.indexed[k]; !ok {
		in.indexed[k] = struct{}{}
		b.index[k] = append(b.index[k], in.id)
	}
}

// newInstance creates the instance of a draw with a reactive block and runs
// it once, as part of the activation that is running.
func (b *builder) newInstance(st *ir.Draw, e *ex, owner int) *value.Fault {
	in := &instance{id: int32(len(b.instances)), block: st.Reactive,
		frame: append([]value.Value(nil), e.fr...), owner: owner, pos: st.Pos(), cur: b.cur,
		indexed: map[readKey]struct{}{}}
	b.instances = append(b.instances, in)
	if b.inst != nil {
		b.inst.children = append(b.inst.children, in)
	}
	return b.runInstance(in, true)
}

// runInstance runs the block of in with tracking. A re-run is a new
// activation at the site of the draw.
func (b *builder) runInstance(in *instance, first bool) *value.Fault {
	sAct, sPos, sCur, sInst, sTrack := b.act, b.pos, b.cur, b.inst, b.track
	b.track, b.inst, b.cur = in, in, in.cur
	if !first {
		b.act, b.depth = b.begin(in.pos), 0
	}
	in.reads = map[readKey]struct{}{}
	_, _, flt := b.execBlock(&ex{fr: append([]value.Value(nil), in.frame...)}, in.block)
	b.act, b.pos, b.cur, b.inst, b.track = sAct, sPos, sCur, sInst, sTrack
	return flt
}

// cleanup deletes what an instance made in its last run: its child
// instances and its unattached shapes.
func (b *builder) cleanup(in *instance) {
	for _, c := range in.children {
		b.retire(c)
	}
	in.children = nil
	for _, i := range in.owned {
		if !b.attached[i] && b.world.live(i) {
			b.world.delete(i, b.now)
		}
	}
	in.owned = nil
}

// retire ends an instance for good.
func (b *builder) retire(in *instance) {
	if in.dead {
		return
	}
	in.dead = true
	b.cleanup(in)
}

// rerunReactive re-runs the instances whose read-set meets ch, in creation
// order.
func (b *builder) rerunReactive(ch []readKey) *value.Fault {
	if len(ch) == 0 || len(b.instances) == 0 {
		return nil
	}
	var hit []*instance
	for _, k := range ch {
		for _, id := range b.index[k] {
			in := b.instances[id]
			if in.dead {
				continue
			}
			if _, ok := in.reads[k]; ok {
				hit = append(hit, in)
			}
		}
	}
	if len(hit) == 0 {
		return nil
	}
	sort.Slice(hit, func(i, j int) bool { return hit[i].id < hit[j].id })
	var prev *instance
	for _, in := range hit {
		if in == prev || in.dead {
			continue
		}
		prev = in
		if !b.world.live(in.owner) {
			b.retire(in)
			continue
		}
		b.cleanup(in)
		if flt := b.runInstance(in, false); flt != nil {
			return flt
		}
	}
	return nil
}
