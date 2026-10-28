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
	// Style holds the theme values that differ from std/theme/default; nil
	// when none does (the "style" object is then not written).
	Style *Style
	// Shapes are in creation order.
	Shapes []SceneShape
	// Markers are sorted by (At, Seq).
	Markers []Marker
	// Camera is the camera path of look.Focus, strictly increasing in At;
	// empty when no Focus ran (the "camera" array is then not written).
	Camera []CameraKey
	// Slots has one entry per fact processed: the presentation time of
	// fact Seq.
	Slots []SlotEntry
}

// ThemeColor is a named color with its hex value ("#rrggbb").
type ThemeColor struct{ Name, Hex string }

// Shape kinds.
const (
	KindCircle  = "circle"
	KindRect    = "rect"
	KindLabel   = "label"
	KindLine    = "line"
	KindGroup   = "group"
	KindIcon    = "icon"
	KindCounter = "counter"
)

// IconSize is the size of an icon shape, written as its prop "size".
const IconSize = 32

// Style is the "style" object of scene.json. Encoding writes only the
// fields that differ from DefaultStyle.
type Style struct {
	Sans, Mono                                   string
	TitleSize, LabelSize, ValueSize, CaptionSize float64
	TitleWeight, LabelWeight                     float64
	Tracking, Stroke, Glow, Shadow, Vignette     float64
	Sketch, Drift                                float64
	Easing                                       string
}

// DefaultStyle is the style of std/theme/default.
func DefaultStyle() Style {
	return Style{Sans: "Inter", Mono: "JetBrains Mono",
		TitleSize: 28, LabelSize: 16, ValueSize: 15, CaptionSize: 20,
		TitleWeight: 600, LabelWeight: 500, Stroke: 2, Glow: 24, Easing: "smooth"}
}

// CameraKey is a key of the camera: it looks at (X, Y) at Zoom at time At.
type CameraKey struct {
	At         int64
	X, Y, Zoom float64
}

// SceneShape is one shape. Which of R, W, H, Radius, From, To and Items are
// written depends on Kind: circle r; rect w, h, radius; label none; line
// from, to (shape ids); group items (shape ids); icon and counter none.
type SceneShape struct {
	ID, Kind string
	R, W, H  float64
	Radius   float64
	From, To string
	Items    []string
	Keys     []Key
	// Node ("n3") is the node the shape is attached to and Msg the id of the
	// message it flies for; each is written only when its Has flag is set.
	Node    string
	HasNode bool
	Msg     int
	HasMsg  bool
	// End is the time the shape stops existing; written only when HasEnd.
	End    int64
	HasEnd bool
}

// Property bits of Key.Set, in output order.
const (
	PX uint32 = 1 << iota
	PY
	PScale
	POpacity
	PRotation
	PFill
	PStroke
	PText
	PEffect
	// Properties of format 0.3, in output order after PEffect.
	PCurve
	PTrail
	PDash
	PDraw
	PHalo
	PIcon
	PCount
	PRole
	PFace
)

// PPropsV3 is the set of the bits of the properties added in format 0.3.
const PPropsV3 = PCurve | PTrail | PDash | PDraw | PHalo | PIcon | PCount | PRole | PFace

// Key is a keyframe: the properties whose bit is set in Set take the given
// value at At.
type Key struct {
	At                             int64
	Set                            uint32
	X, Y, Scale, Opacity, Rotation float64
	Fill, Stroke, Text, Effect     string
	// 0.3: Curve (bend of later moves), Trail (ms), Dash as [DashOn, DashOff]
	// (both 0 is the empty list), Draw, Halo, Count; Icon, Role, Face.
	Curve, Trail, DashOn, DashOff float64
	Draw, Halo, Count             float64
	Icon, Role, Face              string
}

// Marker is a point of the timeline that a player may show: Kind is pause,
// caption, broken or failed (readers ignore other kinds). Seq is the fact
// that raised it and only orders markers; it is not written.
type Marker struct {
	At   int64
	Seq  int
	Kind string
	Text string
	// Choices are the choices of a pause marker; written only when not empty.
	Choices []string
}

// SlotEntry says when the fact with sequence number Seq is presented.
type SlotEntry struct {
	Seq int
	At  int64
}
