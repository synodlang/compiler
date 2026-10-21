package trace

// Scene is a presentation of a run: shapes with keyframed properties on a
// timeline of integer milliseconds. The scene builder (internal/scene)
// produces it and EncodeScene writes it as scene.json (format
// synod-scene/1).
type Scene struct {
	// Duration is the length of the presentation in ms.
	Duration int64
	// Theme lists the colors used by name, in output order.
	Theme []ThemeColor
	// Shapes are in creation order.
	Shapes []SceneShape
	// Markers are sorted by (At, Seq).
	Markers []Marker
	// Slots has one entry per fact processed: the presentation time of
	// fact Seq.
	Slots []SlotEntry
}

// ThemeColor is a named color with its hex value ("#rrggbb").
type ThemeColor struct{ Name, Hex string }

// Shape kinds.
const (
	KindCircle = "circle"
	KindRect   = "rect"
	KindLabel  = "label"
	KindLine   = "line"
	KindGroup  = "group"
)

// SceneShape is one shape. Which of R, W, H, Radius, From, To and Items are
// written depends on Kind: circle r; rect w, h, radius; label none; line
// from, to (shape ids); group items (shape ids).
type SceneShape struct {
	ID, Kind string
	R, W, H  float64
	Radius   float64
	From, To string
	Items    []string
	Keys     []Key
	// End is the time the shape stops existing; written only when HasEnd.
	End    int64
	HasEnd bool
}

// Property bits of Key.Set, in output order.
const (
	PX uint16 = 1 << iota
	PY
	PScale
	POpacity
	PRotation
	PFill
	PStroke
	PText
	PEffect
)

// Key is a keyframe: the properties whose bit is set in Set take the given
// value at At.
type Key struct {
	At                             int64
	Set                            uint16
	X, Y, Scale, Opacity, Rotation float64
	Fill, Stroke, Text, Effect     string
}

// Marker is a point of the timeline that a player may show: Kind is pause,
// caption, broken or failed (readers ignore other kinds). Seq is the fact
// that raised it and only orders markers; it is not written.
type Marker struct {
	At   int64
	Seq  int
	Kind string
	Text string
}

// SlotEntry says when the fact with sequence number Seq is presented.
type SlotEntry struct {
	Seq int
	At  int64
}
