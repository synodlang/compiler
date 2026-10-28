package stdlib

import (
	"strconv"
	"strings"

	"github.com/synodlang/compiler/internal/types"
)

// ThemeName is one row of the theme table (spec/09 section 9.5): a name that
// every theme defines once, with the type and the domain of its value.
type ThemeName struct {
	Name string
	Type types.Type
	// Min and Max bound a number (inclusive) when Choices is empty.
	Min, Max float64
	// Choices lists the numbers (as written) or the texts a value may take;
	// empty for a color and for a number in a range.
	Choices []string
}

// DefaultThemePath is the import path of the theme of a program whose main
// file imports none.
const DefaultThemePath = "std/theme/default"

// FontFamilies is the closed font set (decision 118).
var FontFamilies = []string{"Inter", "JetBrains Mono", "Source Serif 4", "Shantell Sans"}

// ThemeNames is the theme table in the order of spec/09 section 9.5 and of
// std/theme.
var ThemeNames = buildThemeNames()

func buildThemeNames() []ThemeName {
	var out []ThemeName
	color := func(names ...string) {
		for _, n := range names {
			out = append(out, ThemeName{Name: n, Type: types.TColor})
		}
	}
	rng := func(name string, lo, hi float64) {
		out = append(out, ThemeName{Name: name, Type: types.TNumber, Min: lo, Max: hi})
	}
	pick := func(name string, t types.Type, choices ...string) {
		out = append(out, ThemeName{Name: name, Type: t, Choices: choices})
	}
	color("Paper", "Ink", "Line", "Quiet", "Accent", "Good", "Warning", "Critical",
		"Message1", "Message2", "Message3", "Message4")
	rng("Vignette", 0, 1)
	rng("Sketch", 0, 1)
	rng("Glow", 0, 48)
	rng("Shadow", 0, 1)
	rng("StrokeWidth", 0.5, 6)
	rng("CornerRadius", 0, 40)
	pick("NodeShape", types.TText, "card", "circle")
	pick("NodeFill", types.TText, "solid", "outline")
	rng("LinkOpacity", 0, 1)
	pick("Easing", types.TText, "smooth", "spring")
	rng("Drift", 0, 24)
	pick("Sans", types.TText, FontFamilies...)
	pick("Mono", types.TText, FontFamilies...)
	rng("TitleSize", 12, 64)
	rng("LabelSize", 10, 40)
	rng("ValueSize", 10, 40)
	rng("CaptionSize", 12, 48)
	pick("TitleWeight", types.TNumber, "400", "500", "600", "700")
	pick("LabelWeight", types.TNumber, "400", "500", "600", "700")
	rng("Tracking", -0.05, 0.2)
	rng("Margin", 0, 100)
	rng("Fill", 0.5, 1.5)
	return out
}

// ThemeNameOf returns the row called name, or nil.
func ThemeNameOf(name string) *ThemeName {
	for i := range ThemeNames {
		if ThemeNames[i].Name == name {
			return &ThemeNames[i]
		}
	}
	return nil
}

// Domain describes the values the row accepts, for a diagnostic: "a color",
// "a number from 0 to 48", `"card" or "circle"`.
func (t *ThemeName) Domain() string {
	switch {
	case t.Type == types.TColor:
		return "a color such as #151A2E"
	case len(t.Choices) > 0 && t.Type == types.TText:
		q := make([]string, len(t.Choices))
		for i, c := range t.Choices {
			q[i] = strconv.Quote(c)
		}
		if len(q) > 2 {
			return "one of " + strings.Join(q[:len(q)-1], ", ") + " or " + q[len(q)-1]
		}
		return strings.Join(q, " or ")
	case len(t.Choices) > 0:
		return "one of " + strings.Join(t.Choices[:len(t.Choices)-1], ", ") + " or " + t.Choices[len(t.Choices)-1]
	}
	return "a number from " + strconv.FormatFloat(t.Min, 'f', -1, 64) + " to " + strconv.FormatFloat(t.Max, 'f', -1, 64)
}

// AcceptsNumber reports whether n is in the domain of a number row.
func (t *ThemeName) AcceptsNumber(n float64) bool {
	if len(t.Choices) > 0 {
		for _, c := range t.Choices {
			if v, err := strconv.ParseFloat(c, 64); err == nil && v == n {
				return true
			}
		}
		return false
	}
	return n >= t.Min && n <= t.Max
}

// AcceptsText reports whether s is in the domain of a text row.
func (t *ThemeName) AcceptsText(s string) bool {
	for _, c := range t.Choices {
		if c == s {
			return true
		}
	}
	return false
}
