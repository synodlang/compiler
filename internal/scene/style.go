package scene

import (
	"github.com/synodlang/compiler/internal/ir"
	"github.com/synodlang/compiler/internal/trace"
	"github.com/synodlang/compiler/internal/value"
)

// The theme (spec/09 section 9.5). The program's theme is evaluated at compile
// time (ir.Program.Theme); the values below are those of std/theme/default,
// used for every name a program's theme does not carry (a nil Theme, or a
// test building an IR by hand) and to leave out of the scene the style keys
// that equal the default. A test keeps them in step with
// std/theme/default/default.synod.

// themeColorNames are the twelve color names of a theme in catalog order.
var themeColorNames = []string{"Paper", "Ink", "Line", "Quiet", "Accent", "Good", "Warning", "Critical",
	"Message1", "Message2", "Message3", "Message4"}

var defaultColors = map[string]string{
	"Paper": "#151A2E", "Ink": "#EDE9E1", "Line": "#3A4366", "Quiet": "#8A90A8",
	"Accent": "#F4A340", "Good": "#5CC8B8", "Warning": "#E8C35A", "Critical": "#E86F5A",
	"Message1": "#F4A340", "Message2": "#7FB2F0", "Message3": "#C79BEA", "Message4": "#F08FB0",
}

var themeNumNames = []string{"Vignette", "Sketch", "Glow", "Shadow", "StrokeWidth", "CornerRadius",
	"LinkOpacity", "Drift", "TitleSize", "LabelSize", "ValueSize", "CaptionSize", "TitleWeight",
	"LabelWeight", "Tracking", "Margin", "Fill"}

var defaultNums = map[string]float64{
	"Vignette": 0, "Sketch": 0, "Glow": 24, "Shadow": 0, "StrokeWidth": 2, "CornerRadius": 12,
	"LinkOpacity": 1, "Drift": 0, "TitleSize": 28, "LabelSize": 16, "ValueSize": 15, "CaptionSize": 20,
	"TitleWeight": 600, "LabelWeight": 500, "Tracking": 0, "Margin": 4, "Fill": 1,
}

var themeTextNames = []string{"NodeShape", "NodeFill", "Easing", "Sans", "Mono"}

var defaultTexts = map[string]string{
	"NodeShape": "card", "NodeFill": "solid", "Easing": "smooth", "Sans": "Inter", "Mono": "JetBrains Mono",
}

// theme is the theme of the program being presented.
type theme struct{ t *ir.Theme }

func (th theme) color(name string) string {
	if v, ok := th.t.Get(name); ok {
		if h, ok := v.V.(string); ok {
			return h
		}
	}
	return defaultColors[name]
}

func (th theme) num(name string) float64 {
	if v, ok := th.t.Get(name); ok {
		if f, ok := v.V.(float64); ok {
			return f
		}
	}
	return defaultNums[name]
}

func (th theme) text(name string) string {
	if v, ok := th.t.Get(name); ok {
		if s, ok := v.V.(string); ok {
			return s
		}
	}
	return defaultTexts[name]
}

// world gives the values the world bakes into shapes.
func (th theme) world() worldTheme {
	return worldTheme{Radius: th.num("CornerRadius"), LinkOpacity: th.num("LinkOpacity"),
		Fill: th.num("Fill"), Spring: th.text("Easing") == "spring"}
}

// base gives the eight theme colors of the scene in catalog order.
func (th theme) base() []trace.ThemeColor {
	out := make([]trace.ThemeColor, 8)
	for i := range out {
		out[i] = trace.ThemeColor{Name: themeColorNames[i], Hex: th.color(themeColorNames[i])}
	}
	return out
}

// style gives the style object of the scene, or nil when every key holds its
// default.
func (th theme) style() *trace.Style {
	s := th.styleValue()
	if s == trace.DefaultStyle() {
		return nil
	}
	return &s
}

// styleValue is the style of the theme, complete.
func (th theme) styleValue() trace.Style {
	return trace.Style{
		Sans: th.text("Sans"), Mono: th.text("Mono"),
		TitleSize: th.num("TitleSize"), LabelSize: th.num("LabelSize"),
		ValueSize: th.num("ValueSize"), CaptionSize: th.num("CaptionSize"),
		TitleWeight: th.num("TitleWeight"), LabelWeight: th.num("LabelWeight"),
		Tracking: th.num("Tracking"), Stroke: th.num("StrokeWidth"), Glow: th.num("Glow"),
		Shadow: th.num("Shadow"), Vignette: th.num("Vignette"), Sketch: th.num("Sketch"),
		Drift: th.num("Drift"), Easing: th.text("Easing"),
	}
}

func init() {
	for _, n := range themeColorNames {
		Table["theme."+n] = themeColorNative(n)
	}
	for _, n := range themeNumNames {
		Table["theme."+n] = themeNumNative(n)
	}
	for _, n := range themeTextNames {
		Table["theme."+n] = themeTextNative(n)
	}
}

// themeColorNative is theme.<Name> for a color: the color named Name, whose
// hex is the program theme's. A theme color keeps its bare name in the scene.
func themeColorNative(name string) Func {
	return func(b *builder, c *ir.Call, args []value.Value) (value.Value, *value.Fault) {
		if _, ok := b.hexes[name]; !ok {
			b.hexes[name] = b.th.color(name)
		}
		return value.Color(name), nil
	}
}

func themeNumNative(name string) Func {
	return func(b *builder, c *ir.Call, args []value.Value) (value.Value, *value.Fault) {
		return value.Number(b.th.num(name)), nil
	}
}

func themeTextNative(name string) Func {
	return func(b *builder, c *ir.Call, args []value.Value) (value.Value, *value.Fault) {
		return value.Text(b.th.text(name)), nil
	}
}

// fnv1a32 is the 32-bit FNV-1a hash of s (its UTF-8 bytes).
func fnv1a32(s string) uint32 {
	h := uint32(2166136261)
	for i := 0; i < len(s); i++ {
		h ^= uint32(s[i])
		h *= 16777619
	}
	return h
}

// messageColorName is the theme color look.MessageColor gives a message of
// the record type typeName (as the trace writes it): Message1 to Message4,
// number 1 + (FNV-1a-32(typeName) mod 4).
func messageColorName(typeName string) string {
	return "Message" + string(rune('1'+fnv1a32(typeName)%4))
}
