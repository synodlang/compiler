package trace

import (
	"io"
	"math"

	"github.com/synodlang/compiler/internal/value"
)

// EncodeScene writes scene.json: no spaces, keys in the order of the format
// spec, each shape, marker and trace entry on a line of its own. It fails on
// a non-finite number, on -0 and on an unknown shape kind.
func EncodeScene(w io.Writer, s *Scene) error {
	e := &encoder{}
	if err := e.scene(s); err != nil {
		return err
	}
	_, err := w.Write(e.w.b)
	return err
}

func (e *encoder) num(f float64) error {
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return encodeError("non-finite number")
	}
	if f == 0 && math.Signbit(f) {
		return encodeError("negative zero")
	}
	e.w.raw(value.FormatNumber(f))
	return nil
}

func (e *encoder) numKey(k string, f float64) error {
	e.w.key(k, false)
	return e.num(f)
}

func (e *encoder) scene(s *Scene) error {
	w := &e.w
	w.raw(`{"format":"synod-scene/1","duration":`)
	w.int(s.Duration)
	w.raw(`,"canvas":{"w":1600,"h":1000},"theme":{`)
	for i, c := range s.Theme {
		w.key(c.Name, i == 0)
		w.str(c.Hex)
	}
	w.byte('}')
	if err := e.style(s.Style); err != nil {
		return err
	}
	w.raw(`,"shapes":`)
	if err := e.lines(len(s.Shapes), func(i int) error { return e.shape(&s.Shapes[i]) }); err != nil {
		return err
	}
	w.raw(`,"markers":`)
	if err := e.lines(len(s.Markers), func(i int) error {
		m := &s.Markers[i]
		w.raw(`{"at":`)
		w.int(m.At)
		w.key("kind", false)
		w.str(m.Kind)
		w.key("text", false)
		w.str(m.Text)
		if m.Kind == "pause" && len(m.Choices) > 0 {
			w.key("choices", false)
			w.strs(m.Choices)
		}
		w.byte('}')
		return nil
	}); err != nil {
		return err
	}
	if len(s.Camera) > 0 {
		w.raw(`,"camera":`)
		if err := e.lines(len(s.Camera), func(i int) error {
			c := &s.Camera[i]
			w.raw(`{"at":`)
			w.int(c.At)
			if err := e.numKey("x", c.X); err != nil {
				return err
			}
			if err := e.numKey("y", c.Y); err != nil {
				return err
			}
			if err := e.numKey("zoom", c.Zoom); err != nil {
				return err
			}
			w.byte('}')
			return nil
		}); err != nil {
			return err
		}
	}
	w.raw(`,"trace":`)
	if err := e.lines(len(s.Slots), func(i int) error {
		t := &s.Slots[i]
		w.raw(`{"seq":`)
		w.int(int64(t.Seq))
		w.raw(`,"at":`)
		w.int(t.At)
		w.byte('}')
		return nil
	}); err != nil {
		return err
	}
	w.raw("}\n")
	return nil
}

// style writes `,"style":{…}` with the fields that differ from the default
// style, in the order of the format spec; nothing when none does.
func (e *encoder) style(st *Style) error {
	if st == nil {
		return nil
	}
	d := DefaultStyle()
	w := &e.w
	first := true
	open := func(k string) {
		if first {
			w.raw(`,"style":{`)
		}
		w.key(k, first)
		first = false
	}
	str := func(k, v, def string) {
		if v != def {
			open(k)
			w.str(v)
		}
	}
	var err error
	num := func(k string, v, def float64) {
		if v != def && err == nil {
			open(k)
			err = e.num(v)
		}
	}
	str("sans", st.Sans, d.Sans)
	str("mono", st.Mono, d.Mono)
	num("titleSize", st.TitleSize, d.TitleSize)
	num("labelSize", st.LabelSize, d.LabelSize)
	num("valueSize", st.ValueSize, d.ValueSize)
	num("captionSize", st.CaptionSize, d.CaptionSize)
	num("titleWeight", st.TitleWeight, d.TitleWeight)
	num("labelWeight", st.LabelWeight, d.LabelWeight)
	num("tracking", st.Tracking, d.Tracking)
	num("stroke", st.Stroke, d.Stroke)
	num("glow", st.Glow, d.Glow)
	num("shadow", st.Shadow, d.Shadow)
	num("vignette", st.Vignette, d.Vignette)
	num("sketch", st.Sketch, d.Sketch)
	num("drift", st.Drift, d.Drift)
	str("easing", st.Easing, d.Easing)
	if !first {
		w.byte('}')
	}
	return err
}

// lines writes an array of n items, one per line; an empty array is "[]".
func (e *encoder) lines(n int, item func(i int) error) error {
	w := &e.w
	if n == 0 {
		w.raw("[]")
		return nil
	}
	w.raw("[\n")
	for i := 0; i < n; i++ {
		if i > 0 {
			w.raw(",\n")
		}
		if err := item(i); err != nil {
			return err
		}
	}
	w.raw("\n]")
	return nil
}

func (e *encoder) shape(s *SceneShape) error {
	w := &e.w
	w.raw(`{"id":`)
	w.str(s.ID)
	w.key("kind", false)
	w.str(s.Kind)
	if s.HasNode {
		w.key("node", false)
		w.str(s.Node)
	}
	if s.HasMsg {
		w.key("msg", false)
		w.int(int64(s.Msg))
	}
	w.raw(`,"props":{`)
	switch s.Kind {
	case KindCircle:
		w.raw(`"r":`)
		if err := e.num(s.R); err != nil {
			return err
		}
	case KindRect:
		w.raw(`"w":`)
		if err := e.num(s.W); err != nil {
			return err
		}
		if err := e.numKey("h", s.H); err != nil {
			return err
		}
		if err := e.numKey("radius", s.Radius); err != nil {
			return err
		}
	case KindLabel, KindCounter:
	case KindIcon:
		w.raw(`"size":32`)
	case KindLine:
		w.raw(`"from":`)
		w.str(s.From)
		w.key("to", false)
		w.str(s.To)
	case KindGroup:
		w.raw(`"items":`)
		w.strs(s.Items)
	default:
		return encodeError("unknown shape kind " + s.Kind)
	}
	w.raw(`},"keys":[`)
	for i := range s.Keys {
		if i > 0 {
			w.byte(',')
		}
		if err := e.sceneKey(&s.Keys[i]); err != nil {
			return err
		}
	}
	w.byte(']')
	if s.HasEnd {
		w.raw(`,"end":`)
		w.int(s.End)
	}
	w.byte('}')
	return nil
}

func (e *encoder) sceneKey(k *Key) error {
	w := &e.w
	w.raw(`{"at":`)
	w.int(k.At)
	nums := [...]struct {
		bit  uint32
		name string
		v    float64
	}{{PX, "x", k.X}, {PY, "y", k.Y}, {PScale, "scale", k.Scale}, {POpacity, "opacity", k.Opacity}, {PRotation, "rotation", k.Rotation}}
	strs := [...]struct {
		bit  uint32
		name string
		v    string
	}{{PFill, "fill", k.Fill}, {PStroke, "stroke", k.Stroke}, {PText, "text", k.Text}, {PEffect, "effect", k.Effect}}
	for _, p := range nums {
		if k.Set&p.bit != 0 {
			if err := e.numKey(p.name, p.v); err != nil {
				return err
			}
		}
	}
	for _, p := range strs {
		if k.Set&p.bit != 0 {
			w.key(p.name, false)
			w.str(p.v)
		}
	}
	if k.Set&PPropsV3 != 0 {
		if err := e.keyV3(k); err != nil {
			return err
		}
	}
	w.byte('}')
	return nil
}

// keyV3 writes the properties of format 0.3 of a key: curve, trail, dash,
// draw, halo, icon, count, role, face.
func (e *encoder) keyV3(k *Key) error {
	w := &e.w
	for _, p := range [...]struct {
		bit  uint32
		name string
		v    float64
	}{{PCurve, "curve", k.Curve}, {PTrail, "trail", k.Trail}} {
		if k.Set&p.bit != 0 {
			if err := e.numKey(p.name, p.v); err != nil {
				return err
			}
		}
	}
	if k.Set&PDash != 0 {
		w.key("dash", false)
		if k.DashOn == 0 && k.DashOff == 0 {
			w.raw("[]")
		} else {
			w.byte('[')
			if err := e.num(k.DashOn); err != nil {
				return err
			}
			w.byte(',')
			if err := e.num(k.DashOff); err != nil {
				return err
			}
			w.byte(']')
		}
	}
	for _, p := range [...]struct {
		bit  uint32
		name string
		v    float64
	}{{PDraw, "draw", k.Draw}, {PHalo, "halo", k.Halo}} {
		if k.Set&p.bit != 0 {
			if err := e.numKey(p.name, p.v); err != nil {
				return err
			}
		}
	}
	if k.Set&PIcon != 0 {
		w.key("icon", false)
		w.str(k.Icon)
	}
	if k.Set&PCount != 0 {
		if err := e.numKey("count", k.Count); err != nil {
			return err
		}
	}
	if k.Set&PRole != 0 {
		w.key("role", false)
		w.str(k.Role)
	}
	if k.Set&PFace != 0 {
		w.key("face", false)
		w.str(k.Face)
	}
	return nil
}
