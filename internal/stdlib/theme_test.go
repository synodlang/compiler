package stdlib

import (
	"testing"

	"github.com/synodlang/compiler/internal/types"
)

func TestThemeNameDomains(t *testing.T) {
	if len(ThemeNames) != 34 {
		t.Fatalf("%d theme names, want 34", len(ThemeNames))
	}
	seen := map[string]bool{}
	for _, n := range ThemeNames {
		if seen[n.Name] {
			t.Errorf("%s twice", n.Name)
		}
		seen[n.Name] = true
		if ThemeNameOf(n.Name) != &ThemeNames[len(seen)-1] {
			t.Errorf("ThemeNameOf(%s)", n.Name)
		}
	}
	if ThemeNameOf("Sparkle") != nil {
		t.Error("an unknown name has a row")
	}
	num := func(name string, ok []float64, bad []float64) {
		n := ThemeNameOf(name)
		if n.Type != types.TNumber {
			t.Fatalf("%s is not a number", name)
		}
		for _, x := range ok {
			if !n.AcceptsNumber(x) {
				t.Errorf("%s rejects %v", name, x)
			}
		}
		for _, x := range bad {
			if n.AcceptsNumber(x) {
				t.Errorf("%s accepts %v", name, x)
			}
		}
	}
	num("Glow", []float64{0, 24, 48}, []float64{-1, 48.5})
	num("Tracking", []float64{-0.05, 0, 0.2}, []float64{-0.06, 0.21})
	num("StrokeWidth", []float64{0.5, 6}, []float64{0.4, 7})
	num("TitleWeight", []float64{400, 500, 600, 700}, []float64{300, 450, 800})
	if n := ThemeNameOf("Sans"); !n.AcceptsText("Shantell Sans") || n.AcceptsText("Comic Sans") {
		t.Error("Sans domain")
	}
	if n := ThemeNameOf("NodeShape"); !n.AcceptsText("circle") || n.AcceptsText("square") || n.Domain() != `"card" or "circle"` {
		t.Errorf("NodeShape domain %q", n.Domain())
	}
	if d := ThemeNameOf("Glow").Domain(); d != "a number from 0 to 48" {
		t.Errorf("Glow domain %q", d)
	}
}
