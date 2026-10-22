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
	w.raw(`},"shapes":`)
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
	case KindLabel:
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
		bit  uint16
		name string
		v    float64
	}{{PX, "x", k.X}, {PY, "y", k.Y}, {PScale, "scale", k.Scale}, {POpacity, "opacity", k.Opacity}, {PRotation, "rotation", k.Rotation}}
	strs := [...]struct {
		bit  uint16
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
	w.byte('}')
	return nil
}
