package scene

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"unicode/utf8"

	"github.com/synodlang/compiler/internal/trace"
)

// worldErr is a look fault raised by the world. The interpreter turns it
// into a value.Fault at the position of the call. Code is "bad-argument" or
// "run-limit".
type worldErr struct{ Code, Msg string }

func (e *worldErr) Error() string { return e.Code + ": " + e.Msg }

func badArg(format string, a ...any) *worldErr {
	return &worldErr{"bad-argument", fmt.Sprintf(format, a...)}
}

// Attachment keys: what a shape can be attached to ("draw … for key").
const (
	akNode uint8 = iota
	akText
	akNumber
)

// attKey is the key of an attachment: a node, a text or a number.
type attKey struct {
	Kind uint8
	Node int
	Num  float64
	Text string
}

func nodeKey(n int) attKey       { return attKey{Kind: akNode, Node: n} }
func textKey(s string) attKey    { return attKey{Kind: akText, Text: s} }
func numberKey(f float64) attKey { return attKey{Kind: akNumber, Num: f} }

type attID struct {
	Name string
	Key  attKey
}

// shapeSpec describes a new shape. Kind is a trace.Kind* constant. Circle
// uses R; rect W and H; label Text; line From and To (shape indexes); icon
// Text (the icon name); counter Count; group none (members are added by
// world.group).
type shapeSpec struct {
	Kind     string
	R, W, H  float64
	Text     string
	From, To int
	Count    float64
}

// iconNames is the icon set (implementation/icons.json).
var iconNames = map[string]bool{
	"server": true, "database": true, "disk": true, "user": true, "client": true,
	"clock": true, "lock": true, "mail": true, "check": true, "cross": true,
	"warning": true, "crown": true, "bolt": true, "cloud": true, "key": true, "eye": true,
}

// effectLen is the duration in ms (before pace) of each effect.
var effectLen = map[string]int64{
	"glow": 600, "indicate": 600, "flash": 500, "circumscribe": 900, "ripple": 700, "sweep": 800,
}

// worldTheme are the theme values the world bakes into shapes.
type worldTheme struct {
	// Radius is the corner radius of rects, LinkOpacity the opacity of a
	// new line, Fill the layout scale.
	Radius, LinkOpacity, Fill float64
	// Spring selects the back-out curve for x, y, scale, rotation and the
	// camera.
	Spring bool
}

// defaultWorldTheme is the world theme of std/theme/default.
func defaultWorldTheme() worldTheme { return worldTheme{Radius: 12, LinkOpacity: 1, Fill: 1} }

type shape struct {
	idx  int
	kind string
	key  string
	r    float64
	w, h float64
	from int
	to   int
	// items are the members of a group.
	items []int
	keys  []trace.Key
	born  int64
	// replacement is set on a shape that replaced another: not brand-new.
	replacement bool
	deleted     bool
	end         int64
	// group is the index of the group this shape belongs to, or -1.
	group int
	// anchor is the Place constraint anchor, or -1; dx and dy the offset.
	anchor    int
	dx, dy    float64
	followers []int
	pinned    bool
	// radius is the corner radius of a rect.
	radius float64
	// busy is the end of the latest tween on this shape.
	busy       int64
	referenced bool
	isCenter   bool
	// card is set once the shape is attached under "card": only then do
	// its deletion, grouping and pinning change the layout.
	card bool
	// node and msg are the first node the shape is attached to and the first
	// message it flies for (0 means none: node ids and message ids that
	// matter start at 1; hasMsg tells message 0 apart).
	node   int
	msg    int
	hasMsg bool
}

type flight struct {
	shape, from, to int
	start           int64
	sx, sy          float64
	// ax, ay is the world position of from when the flight started, ka the
	// unclamped distance the shape leaves it at, and l, r, t, d the extents of the
	// flown shape.
	ax, ay, ka float64
	l, r, t, d float64
}

type landInfo struct {
	slot    int64
	dropped bool
}

// worldConfig carries the run limits (0 means the default) and the theme
// values the world bakes in (nil means std/theme/default).
type worldConfig struct {
	MaxShapes, MaxKeys int
	Theme              *worldTheme
}

// world is the set of shapes and everything that moves them: timelines,
// Place constraints, groups, attachments, layout and flights. All times are
// presentation ms. Methods take now explicitly. A nil error means success;
// a *worldErr is a look fault (bad-argument or run-limit).
//
// A shape is identified by its index (the Int of value.Shape). Deleted
// shapes stay as tombstones: every method that acts on one does nothing.
type world struct {
	c         *clock
	maxShapes int
	maxKeys   int
	th        worldTheme
	spring    bool

	// cam are the camera keys of look.Focus.
	cam []trace.CameraKey

	shapes []*shape
	nkeys  int
	limit  *worldErr

	att      map[attID]int
	attNames map[attKey][]string

	layout      string
	layoutDirty bool
	memberSeen  int

	pending   map[int]*flight
	pendOrder []int
	// pendBy lists, per shape, the messages it has been sent flying for
	// (entries may be stale; pending is the truth).
	pendBy map[int][]int
	landed map[int]landInfo

	centerIdx int
}

func newWorld(c *clock, cfg worldConfig) *world {
	if cfg.MaxShapes <= 0 {
		cfg.MaxShapes = 1_000_000
	}
	if cfg.MaxKeys <= 0 {
		cfg.MaxKeys = 10_000_000
	}
	th := defaultWorldTheme()
	if cfg.Theme != nil {
		th = *cfg.Theme
	}
	return &world{
		c: c, maxShapes: cfg.MaxShapes, maxKeys: cfg.MaxKeys, th: th, spring: th.Spring,
		att: map[attID]int{}, attNames: map[attKey][]string{},
		pending: map[int]*flight{}, pendBy: map[int][]int{}, landed: map[int]landInfo{},
		centerIdx: -1, layoutDirty: true,
	}
}

func (w *world) addKey() {
	w.nkeys++
	if w.nkeys > w.maxKeys && w.limit == nil {
		w.limit = &worldErr{"run-limit", fmt.Sprintf("more than %d keyframes", w.maxKeys)}
	}
}

// takeErr returns the sticky run-limit error, if any.
func (w *world) takeErr() error {
	if w.limit != nil {
		return w.limit
	}
	return nil
}

// sh returns the shape with index i.
func (w *world) sh(i int) *shape { return w.shapes[i] }

// live reports whether shape i exists and is not deleted.
func (w *world) live(i int) bool { return i >= 0 && i < len(w.shapes) && !w.shapes[i].deleted }

// kindHas reports whether shapes of kind have property p.
func kindHas(kind string, p uint32) bool {
	switch p {
	case trace.PX, trace.PY, trace.PCurve, trace.PTrail:
		return kind != trace.KindLine
	case trace.PFill:
		return kind != trace.KindLine
	case trace.PStroke:
		return kind != trace.KindLabel && kind != trace.KindCounter
	case trace.PText:
		return kind == trace.KindLabel
	case trace.PIcon:
		return kind == trace.KindIcon
	case trace.PCount:
		return kind == trace.KindCounter
	case trace.PRole, trace.PFace:
		return kind == trace.KindLabel || kind == trace.KindCounter
	}
	return true
}

// newShape creates a shape at the scene centre at time now: opacity 1, scale
// 1, rotation 0; a label is filled "Ink" and has its text; a line is stroked
// "Line" and has no position keys. key is the shape-id key (see idGen).
func (w *world) newShape(sp shapeSpec, key string, now int64) (int, error) {
	if len(w.shapes) >= w.maxShapes {
		return -1, &worldErr{"run-limit", fmt.Sprintf("more than %d shapes", w.maxShapes)}
	}
	switch {
	case sp.Kind == trace.KindCircle && (sp.R < 0 || !finite(sp.R)):
		return -1, badArg("a circle's radius cannot be negative")
	case sp.Kind == trace.KindRect && (sp.W < 0 || sp.H < 0 || !finite(sp.W) || !finite(sp.H)):
		return -1, badArg("a rectangle's size cannot be negative")
	case sp.Kind == trace.KindIcon && !iconNames[sp.Text]:
		return -1, badArg("there is no icon %q", sp.Text)
	case sp.Kind == trace.KindCounter && (sp.Count < 0 || !finite(sp.Count)):
		return -1, badArg("a counter cannot show a negative number")
	}
	s := &shape{idx: len(w.shapes), kind: sp.Kind, key: key, r: sp.R, w: sp.W, h: sp.H,
		from: -1, to: -1, born: now, group: -1, anchor: -1, radius: w.th.Radius}
	if sp.Kind == trace.KindLine {
		s.from, s.to = sp.From, sp.To
		w.shapes[sp.From].referenced = true
		w.shapes[sp.To].referenced = true
	}
	w.shapes = append(w.shapes, s)
	k := w.keyAt(s, now)
	if sp.Kind != trace.KindLine {
		setPV(k, trace.PX, pv{N: CenterX})
		setPV(k, trace.PY, pv{N: CenterY})
	}
	setPV(k, trace.POpacity, pv{N: 1})
	switch sp.Kind {
	case trace.KindLabel:
		setPV(k, trace.PFill, pv{S: "Ink"})
		setPV(k, trace.PText, pv{S: sp.Text})
	case trace.KindLine:
		setPV(k, trace.POpacity, pv{N: w.th.LinkOpacity})
		setPV(k, trace.PStroke, pv{S: "Line"})
	case trace.KindIcon:
		setPV(k, trace.PStroke, pv{S: "Ink"})
		setPV(k, trace.PIcon, pv{S: sp.Text})
	case trace.KindCounter:
		setPV(k, trace.PFill, pv{S: "Ink"})
		setPV(k, trace.PCount, pv{N: sp.Count})
	}
	return s.idx, w.takeErr()
}

// center returns the singleton look.Center, creating it (an invisible empty
// group at the centre, born at 0) on first use. It is written to the scene
// only if a line or group refers to it.
func (w *world) center() (int, error) {
	if w.centerIdx >= 0 {
		return w.centerIdx, nil
	}
	i, err := w.newShape(shapeSpec{Kind: trace.KindGroup}, "center", 0)
	if err != nil {
		return -1, err
	}
	w.shapes[i].isCenter = true
	w.centerIdx = i
	return i, nil
}

// fresh reports whether shape s is brand-new at now: drawn this instant and
// not a replacement (the morph rule).
func (w *world) fresh(s *shape, now int64) bool { return s.born == now && !s.replacement }

func (w *world) morphDur(s *shape, now int64) int64 {
	if w.fresh(s, now) {
		return 0
	}
	return w.c.scale(Morph)
}

// validate checks a property value against the kind and ranges of spec/09.
func validate(s *shape, p uint32, v pv) error {
	if !kindHas(s.kind, p) {
		return badArg("a %s has no such property", s.kind)
	}
	if isNum(p) {
		if !finite(v.N) {
			return badArg("a tween value must be a finite number")
		}
		if p == trace.POpacity && (v.N < 0 || v.N > 1) {
			return badArg("opacity must be between 0 and 1")
		}
		if p == trace.PScale && v.N < 0 {
			return badArg("scale cannot be negative")
		}
		if (p == trace.PDraw || p == trace.PHalo) && (v.N < 0 || v.N > 1) {
			return badArg("draw and halo must be between 0 and 1")
		}
		if p == trace.PCount && v.N < 0 {
			return badArg("a count cannot be negative")
		}
	}
	if p == trace.PIcon && !iconNames[v.S] {
		return badArg("there is no icon %q", v.S)
	}
	return nil
}

// tween is the public look.Tween: property p (one trace.P* bit) of shape i
// goes to v over d ms (already scaled by pace; negative is bad-argument).
// For x and y the target is in the shape's own space (relative to its group;
// use localPosOf to aim at another shape). Tweening x or y breaks a Place
// constraint, takes the shape out of the layout and cancels a pending flight.
func (w *world) tween(i int, p uint32, v pv, now, d int64) error {
	s := w.shapes[i]
	if d < 0 {
		return badArg("a duration cannot be negative")
	}
	if err := validate(s, p, v); err != nil {
		return err
	}
	if s.deleted {
		return nil
	}
	if p == trace.PX || p == trace.PY {
		w.moveSelf(s)
	}
	w.apply(s, p, v, now, d)
	return w.takeErr()
}

// set changes a property by the morph rule: at once on a brand-new shape,
// otherwise over Morph/pace. Used for Fill, Stroke, Place, Orbit, layout moves
// and reactive property changes. It does not pin.
func (w *world) set(i int, p uint32, v pv, now int64) error {
	s := w.shapes[i]
	if err := validate(s, p, v); err != nil {
		return err
	}
	if s.deleted {
		return nil
	}
	w.apply(s, p, v, now, w.morphDur(s, now))
	return w.takeErr()
}

// apply writes p on s, fanning Fill and Stroke of a group out to its
// non-label members and moving the followers of a moved anchor.
func (w *world) apply(s *shape, p uint32, v pv, now, d int64) {
	if s.deleted {
		return
	}
	if s.kind == trace.KindGroup && (p == trace.PDraw || p == trace.PHalo) {
		for _, m := range s.items {
			w.apply(w.shapes[m], p, v, now, d)
		}
		return
	}
	if s.kind == trace.KindGroup && (p == trace.PFill || p == trace.PStroke) {
		for _, m := range s.items {
			ms := w.shapes[m]
			if (ms.kind == trace.KindLabel || ms.kind == trace.KindCounter) && ms.group >= 0 {
				continue
			}
			if !kindHas(ms.kind, p) {
				continue
			}
			w.apply(ms, p, v, now, d)
		}
		return
	}
	w.writeProp(s, p, v, now, d)
	if (p == trace.PX || p == trace.PY) && len(s.followers) > 0 {
		for _, f := range s.followers {
			fs := w.shapes[f]
			off := fs.dx
			if p == trace.PY {
				off = fs.dy
			}
			w.apply(fs, p, pv{N: rnd(v.N + off)}, now, d)
		}
	}
}

// moveSelf is what moving a shape by its own doing (Tween x/y, Orbit,
// Flight, Place) does: break its constraint, pin it, cancel its flight.
func (w *world) moveSelf(s *shape) {
	w.breakAnchor(s)
	w.pin(s)
	w.cancelFlight(s)
}

func (w *world) pin(s *shape) {
	if !s.pinned {
		s.pinned = true
		if s.card {
			w.layoutDirty = true
		}
	}
}

func (w *world) breakAnchor(s *shape) {
	if s.anchor < 0 {
		return
	}
	a := w.shapes[s.anchor]
	for i, f := range a.followers {
		if f == s.idx {
			a.followers = append(a.followers[:i:i], a.followers[i+1:]...)
			break
		}
	}
	s.anchor = -1
}

func (w *world) cancelFlight(s *shape) {
	ids, ok := w.pendBy[s.idx]
	if !ok {
		return
	}
	for _, id := range ids {
		if f, ok := w.pending[id]; ok && f.shape == s.idx {
			delete(w.pending, id)
		}
	}
	delete(w.pendBy, s.idx)
}

// busyUntil is the latest end of a tween on shape i or on its group members
// (0 when idle or deleted). Effects and pending flights do not count.
func (w *world) busyUntil(i int) int64 {
	s := w.shapes[i]
	if s.deleted {
		return 0
	}
	b := s.busy
	for _, m := range s.items {
		if x := w.busyUntil(m); x > b {
			b = x
		}
	}
	return b
}

// worldPos is the world position of shape i at time now, rounded to 0.01. A
// line has no position keys: its position is the midpoint of its ends.
func (w *world) worldPos(i int, now int64) (x, y float64) {
	s := w.shapes[i]
	if s.kind == trace.KindLine {
		ax, ay := w.worldPos(s.from, now)
		bx, by := w.worldPos(s.to, now)
		return rnd(float64(ax+bx) / 2), rnd(float64(ay+by) / 2)
	}
	x = w.valueAt(s.keys, trace.PX, now).N
	y = w.valueAt(s.keys, trace.PY, now).N
	if s.group >= 0 {
		gx, gy := w.worldPos(s.group, now)
		x, y = x+gx, y+gy
	}
	return rnd(x), rnd(y)
}

// localPosOf is the position, in the space of shape s (relative to its
// group), of the world point of shape target at now. It is what a Tween of
// "x" or "y" to a shape aims at.
func (w *world) localPosOf(s, target int, now int64) (x, y float64) {
	return w.toLocal(s, target, now, nil)
}

func (w *world) toLocal(s, target int, now int64, pt *[2]float64) (float64, float64) {
	var x, y float64
	if pt != nil {
		x, y = pt[0], pt[1]
	} else {
		x, y = w.worldPos(target, now)
	}
	if g := w.shapes[s].group; g >= 0 {
		gx, gy := w.worldPos(g, now)
		x, y = x-gx, y-gy
	}
	return rnd(x), rnd(y)
}

func (w *world) toLocalPt(s int, x, y float64, now int64) (float64, float64) {
	return w.toLocal(s, -1, now, &[2]float64{x, y})
}

// sizeOf is the size of shape s in the Place size model: circle 2r by 2r;
// rect w by h; label 8 per character by 18; group the bounding box of its
// members; line and center 0.
func (w *world) sizeOf(s *shape) (float64, float64) {
	switch s.kind {
	case trace.KindCircle:
		return float64(2 * s.r), float64(2 * s.r)
	case trace.KindRect:
		return s.w, s.h
	case trace.KindLabel:
		t, _ := finalVal(s.keys, trace.PText)
		return float64(LabelCharW * float64(utf8.RuneCountInString(t.S))), LabelH
	case trace.KindIcon:
		return trace.IconSize, trace.IconSize
	case trace.KindCounter:
		c, _ := finalVal(s.keys, trace.PCount)
		return float64(LabelCharW * float64(counterColumns(c.N))), LabelH
	case trace.KindGroup:
		l, r, t, d := w.extentsOf(s)
		return l + r, t + d
	}
	return 0, 0
}

// extentsOf is how far the box of shape s reaches from its position to the
// left, right, top and down. A shape is centred on its position; a group
// reaches as far as the bounding box of its members.
func (w *world) extentsOf(s *shape) (l, r, t, d float64) {
	if s.kind != trace.KindGroup {
		sw, sh := w.sizeOf(s)
		return float64(sw / 2), float64(sw / 2), float64(sh / 2), float64(sh / 2)
	}
	if len(s.items) == 0 {
		return 0, 0, 0, 0
	}
	minX, minY := math.Inf(1), math.Inf(1)
	maxX, maxY := math.Inf(-1), math.Inf(-1)
	for _, m := range s.items {
		ms := w.shapes[m]
		mw, mh := w.sizeOf(ms)
		x, _ := finalVal(ms.keys, trace.PX)
		y, _ := finalVal(ms.keys, trace.PY)
		minX = math.Min(minX, x.N-float64(mw/2))
		maxX = math.Max(maxX, x.N+float64(mw/2))
		minY = math.Min(minY, y.N-float64(mh/2))
		maxY = math.Max(maxY, y.N+float64(mh/2))
	}
	return -minX, maxX, -minY, maxY
}

// edgeClip is the least distance along the unit vector (ux, uy) from the
// centre of a box of half size (hw, hh) at which a shape with extents l, r,
// t, d (to the left, right, top and down of its position) is PlaceGap clear
// of the box on the side u points to. An axis with u = 0 gives no bound; u
// = 0 on both gives 0.
func edgeClip(hw, hh, l, r, t, d, ux, uy float64) float64 {
	k := math.Inf(1)
	if ux != 0 {
		e := r
		if ux < 0 {
			e = l
		}
		k = math.Min(k, float64(hw+e+PlaceGap)/math.Abs(ux))
	}
	if uy != 0 {
		e := d
		if uy < 0 {
			e = t
		}
		k = math.Min(k, float64(hh+e+PlaceGap)/math.Abs(uy))
	}
	if math.IsInf(k, 1) {
		return 0
	}
	return k
}

// flightDir is the distance from A to B and the unit vector from A to B
// (zero for A = B).
func flightDir(ax, ay, bx, by float64) (dist, ux, uy float64) {
	dx, dy := bx-ax, by-ay
	dist = math.Sqrt(float64(dx*dx) + float64(dy*dy))
	if dist == 0 {
		return 0, 0, 0
	}
	return dist, dx / dist, dy / dist
}

// flightKs are the distances from the two centres at which the flown shape
// leaves a and lands on b. When they exceed dist both shrink in proportion.
func flightKs(ka, kb, dist float64) (float64, float64) {
	if sum := ka + kb; sum > dist {
		f := dist / sum
		return float64(ka * f), float64(kb * f)
	}
	return ka, kb
}

// place is look.Place s above|below|beside t (rel is "above", "below" or
// "beside"): s gets an offset from t and follows every later x and y change
// of t, with the same times, until s is moved itself or placed again. s moves
// by the morph rule and is pinned. A cycle (or s == t) is bad-argument.
func (w *world) place(i int, rel string, t int, now int64) error {
	s, a := w.shapes[i], w.shapes[t]
	if i == t {
		return badArg("a shape cannot be placed relative to itself")
	}
	for x := a; ; {
		if x.idx == i {
			return badArg("Place would make a cycle of shapes")
		}
		if x.anchor < 0 {
			break
		}
		x = w.shapes[x.anchor]
	}
	if s.deleted || a.deleted {
		return nil
	}
	w.moveSelf(s)
	ws, hs := w.sizeOf(s)
	wt, ht := w.sizeOf(a)
	var dx, dy float64
	switch rel {
	case "above":
		dy = -(float64(ht/2) + PlaceGap + float64(hs/2))
	case "below":
		dy = float64(ht/2) + PlaceGap + float64(hs/2)
	default:
		dx = float64(wt/2) + PlaceGap + float64(ws/2)
	}
	dx, dy = rnd(dx), rnd(dy)
	s.anchor, s.dx, s.dy = t, dx, dy
	a.followers = append(a.followers, i)
	ax, ay := w.worldPos(t, now)
	lx, ly := w.toLocalPt(i, ax+dx, ay+dy, now)
	d := w.morphDur(s, now)
	w.apply(s, trace.PX, pv{N: lx}, now, d)
	w.apply(s, trace.PY, pv{N: ly}, now, d)
	w.copyFutureKeys(s, a, now)
	return w.takeErr()
}

// copyFutureKeys mirrors the x and y keys of anchor a after now onto
// follower s, with its offset, so s follows a tween that is already running.
func (w *world) copyFutureKeys(s, a *shape, now int64) {
	for _, k := range append([]trace.Key(nil), a.keys...) {
		if k.At <= now {
			continue
		}
		if k.Set&trace.PX != 0 {
			w.putProp(s, k.At, trace.PX, pv{N: rnd(k.X + s.dx)})
		}
		if k.Set&trace.PY != 0 {
			w.putProp(s, k.At, trace.PY, pv{N: rnd(k.Y + s.dy)})
		}
	}
}

// orbit is look.Orbit s around c at fraction f of a turn: x = cx + 300 sin,
// y = cy - 300 cos, by the morph rule; it pins s.
func (w *world) orbit(i, c int, f float64, now int64) error {
	s := w.shapes[i]
	if !finite(f) {
		return badArg("a fraction of a turn must be a finite number")
	}
	if s.deleted || w.shapes[c].deleted {
		return nil
	}
	cx, cy := w.worldPos(c, now)
	x, y := orbitPoint(cx, cy, f)
	w.moveSelf(s)
	lx, ly := w.toLocalPt(i, x, y, now)
	d := w.morphDur(s, now)
	w.apply(s, trace.PX, pv{N: lx}, now, d)
	w.apply(s, trace.PY, pv{N: ly}, now, d)
	return w.takeErr()
}

// group is look.Group: a new group shape (key as for newShape) whose members'
// x and y become relative to it. The group starts at the first live member's
// world position (the centre when there is none). A member already in a group
// is bad-argument; deleted members and repeats are skipped. Members leave the
// layout.
func (w *world) group(items []int, key string, now int64) (int, error) {
	var ms []int
	seen := map[int]bool{}
	for _, m := range items {
		if seen[m] {
			continue
		}
		seen[m] = true
		if w.shapes[m].deleted {
			continue
		}
		if w.shapes[m].group >= 0 {
			return -1, badArg("a shape belongs to at most one group")
		}
		ms = append(ms, m)
	}
	gx, gy := CenterX, CenterY
	if len(ms) > 0 {
		gx, gy = w.worldPos(ms[0], now)
	}
	g, err := w.newShape(shapeSpec{Kind: trace.KindGroup}, key, now)
	if err != nil {
		return -1, err
	}
	gs := w.shapes[g]
	k := w.keyAt(gs, now)
	k.X, k.Y = gx, gy
	for _, m := range ms {
		s := w.shapes[m]
		s.group = g
		s.referenced = true
		if s.card {
			w.layoutDirty = true
		}
		gs.items = append(gs.items, m)
		for j := range s.keys {
			sk := &s.keys[j]
			if sk.Set&trace.PX != 0 {
				sk.X = rnd(sk.X - gx)
			}
			if sk.Set&trace.PY != 0 {
				sk.Y = rnd(sk.Y - gy)
			}
		}
		if s.anchor >= 0 && w.shapes[s.anchor].group != g {
			// a constraint to a shape outside the group cannot follow: drop it
			w.breakAnchor(s)
		}
	}
	return g, w.takeErr()
}

// effect is look.Effect: key effect name at now and "none" at now + the
// length of the effect scaled by pace. A running effect is replaced. name
// must be one of the effects (bad-argument otherwise).
func (w *world) effect(i int, name string, now int64) error {
	dur, ok := effectLen[name]
	if !ok {
		return badArg("look.Effect needs \"glow\", \"indicate\", \"flash\", \"circumscribe\", \"ripple\" or \"sweep\", not %q", name)
	}
	s := w.shapes[i]
	if s.deleted {
		return nil
	}
	w.removeAfter(s, trace.PEffect, now)
	w.putProp(s, now, trace.PEffect, pv{S: name})
	w.putProp(s, now+w.c.scale(dur), trace.PEffect, pv{S: "none"})
	return w.takeErr()
}

// counterColumns is the number of digit columns of a counter showing the
// whole number nearest n (at least 1).
func counterColumns(n float64) int {
	return len(strconv.FormatFloat(math.Floor(n+0.5), 'f', 0, 64))
}

// setStep sets a step property (curve, trail, dash, role, face) from now on:
// the keys of p after now are dropped and a key is written at now unless the
// value already holds there. Shapes that lack the property do nothing; a
// dash on a group goes to its members that have an outline.
func (w *world) setStep(i int, p uint32, v pv, now int64) error {
	s := w.shapes[i]
	if s.deleted {
		return nil
	}
	if p == trace.PDash && s.kind == trace.KindGroup {
		for _, m := range s.items {
			if k := w.shapes[m].kind; k != trace.KindLabel && k != trace.KindCounter {
				if err := w.setStep(m, p, v, now); err != nil {
					return err
				}
			}
		}
		return w.takeErr()
	}
	if s.kind == trace.KindLine && (p == trace.PCurve || p == trace.PTrail) {
		return nil // a line has no position of its own to bend or trail
	}
	if !kindHas(s.kind, p) {
		return badArg("a %s has no such property", s.kind)
	}
	// from now on: drop the keys of p at now and after, and write a key only
	// when the value that held before differs
	w.removeAfter(s, p, now-1)
	if cur := w.valueAt(s.keys, p, now); !equalPV(cur, v) {
		w.putProp(s, now, p, v)
	}
	return w.takeErr()
}

// focus is look.Focus: the camera moves to (x, y) at zoom z over d ms
// (already scaled by pace). The camera keys after now are dropped; a key at
// now holds the camera as it is there (when a move follows), then comes the
// target at now+d.
func (w *world) focus(x, y, z float64, now, d int64) error {
	if d < 0 {
		return badArg("a duration cannot be negative")
	}
	cur := w.camAt(now)
	n := len(w.cam)
	for n > 0 && w.cam[n-1].At > now {
		n--
	}
	ran := n < len(w.cam)
	w.nkeys -= len(w.cam) - n
	w.cam = w.cam[:n]
	target := trace.CameraKey{X: x, Y: y, Zoom: z}
	if !ran && cur.X == x && cur.Y == y && cur.Zoom == z {
		return nil
	}
	put := func(at int64, k trace.CameraKey) {
		k.At = at
		if n := len(w.cam); n > 0 && w.cam[n-1].At == at {
			w.cam[n-1] = k
			return
		}
		w.cam = append(w.cam, k)
		w.addKey()
	}
	if d == 0 {
		// an instant change: pin the previous camera just before now
		if n := len(w.cam); n > 0 && w.cam[n-1].At < now-1 {
			put(now-1, cur)
		}
		put(now, target)
		return w.takeErr()
	}
	put(now, cur)
	put(now+d, target)
	return w.takeErr()
}

// camAt is the camera at time t: before the first key the scene centre at
// zoom 1; between two keys the curve of the theme, rounded to 0.01.
func (w *world) camAt(t int64) trace.CameraKey {
	cur := trace.CameraKey{At: t, X: CenterX, Y: CenterY, Zoom: 1}
	ks := w.cam
	i := sort.Search(len(ks), func(i int) bool { return ks[i].At > t })
	if i == 0 {
		return cur
	}
	a := ks[i-1]
	if i == len(ks) || a.At == t {
		return trace.CameraKey{At: t, X: a.X, Y: a.Y, Zoom: a.Zoom}
	}
	b := ks[i]
	e := ease(w.spring, float64(t-a.At)/float64(b.At-a.At))
	f := func(p, q float64) float64 { return rnd(p + float64(float64(q-p)*e)) }
	return trace.CameraKey{At: t, X: f(a.X, b.X), Y: f(a.Y, b.Y), Zoom: f(a.Zoom, b.Zoom)}
}

// delete is look.delete: shape i (and its group members) ends now and stays
// attached as a tombstone.
func (w *world) delete(i int, now int64) { w.deleteAt(i, now) }

// deleteAt ends shape i at time at (>= now). The shape is a tombstone from
// now on; keys after at are dropped; constraints and flights are released.
func (w *world) deleteAt(i int, at int64) {
	s := w.shapes[i]
	if s.deleted {
		return
	}
	s.deleted = true
	s.end = at
	if j := firstAfter(s.keys, at); j < len(s.keys) {
		w.nkeys -= len(s.keys) - j
		s.keys = s.keys[:j]
	}
	w.breakAnchor(s)
	for _, f := range s.followers {
		w.shapes[f].anchor = -1
	}
	s.followers = nil
	w.cancelFlight(s)
	if s.card {
		w.layoutDirty = true
	}
	for _, m := range s.items {
		w.deleteAt(m, at)
	}
}

// lookup finds the shape attached to key k under name (possibly deleted).
func (w *world) lookup(name string, k attKey) (int, bool) {
	i, ok := w.att[attID{name, k}]
	return i, ok
}

// attachedTo lists the live shapes attached under any name to key k, in the
// order the names were first attached.
func (w *world) attachedTo(k attKey) []int {
	var out []int
	for _, n := range w.attNames[k] {
		if i, ok := w.att[attID{n, k}]; ok && !w.shapes[i].deleted {
			out = append(out, i)
		}
	}
	return out
}

// attach is draw … for k under name. If another live shape is attached
// there, it is replaced (replace). A tombstone is simply overwritten.
func (w *world) attach(name string, k attKey, s int, now int64) error {
	id := attID{name, k}
	if old, ok := w.att[id]; ok {
		if old != s && !w.shapes[old].deleted && !w.shapes[s].deleted {
			w.replace(old, s, now)
		}
	} else {
		w.attNames[k] = append(w.attNames[k], name)
	}
	w.att[id] = s
	if sh := w.shapes[s]; k.Kind == akNode && k.Node > 0 && sh.node == 0 {
		sh.node = k.Node
	}
	if name == "card" {
		w.shapes[s].card = true
		w.layoutDirty = true
	}
	return w.takeErr()
}

// replace makes shape nw take the place of old at now (plan 2.7): nw takes
// the x, y, scale, rotation and opacity of old, and fill, stroke and text
// where both have them; constraints and the pinned flag move to nw (the
// layout slot follows the attachment). If old was drawn at this same instant
// it is deleted silently and nw shows at once; otherwise old fades out and is
// deleted after Morph/pace while nw fades in. nw is not brand-new afterwards.
func (w *world) replace(old, nw int, now int64) {
	o, n := w.shapes[old], w.shapes[nw]
	n.replacement = true
	same := o.born == now
	d := w.c.scale(Morph)
	for _, p := range []uint32{trace.PX, trace.PY, trace.PScale, trace.PRotation, trace.POpacity} {
		if !kindHas(o.kind, p) || !kindHas(n.kind, p) {
			continue
		}
		if _, has := finalVal(o.keys, p); !has {
			continue // scale and rotation at their defaults
		}
		ov := w.valueAt(o.keys, p, now)
		if p == trace.POpacity && !same {
			w.putProp(n, now, p, pv{N: 0})
			w.writeProp(n, p, ov, now, d)
		} else {
			w.putProp(n, now, p, ov)
		}
	}
	for _, p := range []uint32{trace.PFill, trace.PStroke, trace.PText} {
		if !kindHas(o.kind, p) || !kindHas(n.kind, p) || o.kind == trace.KindGroup || n.kind == trace.KindGroup {
			continue
		}
		ov, ok := finalVal(o.keys, p)
		if !ok {
			continue
		}
		own, has := finalVal(n.keys, p)
		switch {
		case !has:
			w.putProp(n, now, p, ov)
		case !same && !equalPV(own, ov):
			w.putProp(n, now, p, ov)
			w.writeProp(n, p, own, now, d)
		}
	}
	// constraints and pinning move to the new shape
	for _, f := range o.followers {
		w.shapes[f].anchor = nw
		n.followers = append(n.followers, f)
	}
	o.followers = nil
	if o.anchor >= 0 {
		a := w.shapes[o.anchor]
		for i, f := range a.followers {
			if f == old {
				a.followers[i] = nw
			}
		}
		n.anchor, n.dx, n.dy = o.anchor, o.dx, o.dy
		o.anchor = -1
	}
	if o.pinned {
		n.pinned = true
	}
	if same {
		w.deleteAt(old, now)
		return
	}
	w.writeProp(o, trace.POpacity, pv{N: 0}, now, d)
	w.deleteAt(old, now+d)
}

// flightStart is look.Flight s from a to b for message msgID: s jumps to the
// edge of a now (the box of s PlaceGap clear of the box of a, on the line to
// b) and is registered to travel to the edge of b, landing when
// land(msgID, …) is called. If the message already landed, s jumps to the
// end point. It breaks s's constraint and pins it.
func (w *world) flightStart(s, a, b, msgID int, now int64) error {
	sh := w.shapes[s]
	if sh.deleted || w.shapes[a].deleted || w.shapes[b].deleted {
		return nil
	}
	if !sh.hasMsg {
		sh.msg, sh.hasMsg = msgID, true
	}
	w.moveSelf(sh)
	ax, ay := w.worldPos(a, now)
	bx, by := w.worldPos(b, now)
	l, r, t, d := w.extentsOf(sh)
	aw, ah := w.sizeOf(w.shapes[a])
	bw, bh := w.sizeOf(w.shapes[b])
	dist, ux, uy := flightDir(ax, ay, bx, by)
	ka := edgeClip(float64(aw/2), float64(ah/2), r, l, d, t, ux, uy)
	kb := edgeClip(float64(bw/2), float64(bh/2), l, r, t, d, ux, uy)
	kc, _ := flightKs(ka, kb, dist)
	sx, sy := w.toLocalPt(s, ax+float64(ux*kc), ay+float64(uy*kc), now)
	w.apply(sh, trace.PX, pv{N: sx}, now, 0)
	w.apply(sh, trace.PY, pv{N: sy}, now, 0)
	f := &flight{shape: s, from: a, to: b, start: now, sx: sx, sy: sy,
		ax: ax, ay: ay, ka: ka, l: l, r: r, t: t, d: d}
	if li, ok := w.landed[msgID]; ok {
		w.endFlight(f, now, li.dropped)
		return w.takeErr()
	}
	if _, dup := w.pending[msgID]; !dup {
		w.pendOrder = append(w.pendOrder, msgID)
	}
	w.pending[msgID] = f
	w.pendBy[s] = append(w.pendBy[s], msgID)
	return nil
}

// endFlight writes the end keys of f at slot: the edge of the target (the
// box of the shape PlaceGap clear of the box of the target, on the line from
// the start position of a), or the midpoint between the start and end points
// when dropped.
func (w *world) endFlight(f *flight, slot int64, dropped bool) {
	sh := w.shapes[f.shape]
	bx, by := w.worldPos(f.to, slot)
	bw, bh := w.sizeOf(w.shapes[f.to])
	dist, ux, uy := flightDir(f.ax, f.ay, bx, by)
	kb := edgeClip(float64(bw/2), float64(bh/2), f.l, f.r, f.t, f.d, ux, uy)
	_, kb = flightKs(f.ka, kb, dist)
	ex, ey := w.toLocalPt(f.shape, bx-float64(ux*kb), by-float64(uy*kb), slot)
	if dropped {
		ex = rnd(float64(f.sx+ex) / 2)
		ey = rnd(float64(f.sy+ey) / 2)
	}
	t0 := f.start
	if slot < t0 {
		slot = t0
	}
	w.apply(sh, trace.PX, pv{N: ex}, t0, slot-t0)
	w.apply(sh, trace.PY, pv{N: ey}, t0, slot-t0)
}

// land records that message id landed (arrived, or dropped) at slot, and
// finishes its pending flight: end keys at slot at the target, or halfway
// when dropped. Call it for every net.Arrived and net.Dropped before the
// observers run.
func (w *world) land(id int, slot int64, dropped bool) {
	w.landed[id] = landInfo{slot, dropped}
	if f, ok := w.pending[id]; ok {
		delete(w.pending, id)
		if !w.shapes[f.shape].deleted {
			w.endFlight(f, slot, dropped)
		}
	}
}

// finalize stops every flight that never landed halfway at slot (the scene's
// last slot), in the order the flights started.
func (w *world) finalize(slot int64) {
	for _, id := range w.pendOrder {
		if f, ok := w.pending[id]; ok {
			delete(w.pending, id)
			if !w.shapes[f.shape].deleted {
				w.endFlight(f, slot, true)
			}
		}
	}
}
