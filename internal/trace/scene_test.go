package trace

import (
	"bytes"
	"math"
	"strings"
	"testing"

	"github.com/synodlang/compiler/internal/ir"
)

func encScene(t *testing.T, s *Scene) string {
	t.Helper()
	var b bytes.Buffer
	if err := EncodeScene(&b, s); err != nil {
		t.Fatal(err)
	}
	return b.String()
}

func TestEncodeSceneLayout(t *testing.T) {
	s := &Scene{
		Duration: 1850,
		Theme:    []ThemeColor{{"Paper", "#fafaf7"}, {"Ink", "#1b1b1f"}},
		Shapes: []SceneShape{
			{ID: "s0123456789ab", Kind: KindCircle, R: 6, Keys: []Key{
				{At: 0, Set: PX | PY | POpacity | PFill, X: 800, Y: 500, Opacity: 1, Fill: "Accent"},
				{At: 300, Set: PX | PY | PScale | POpacity | PRotation | PEffect, X: 812.5, Y: 400.25, Scale: 2, Opacity: 0.5, Rotation: -90, Effect: "glow"},
			}, End: 900, HasEnd: true},
			{ID: "s000000000002", Kind: KindRect, W: 96, H: 56, Radius: 12, Keys: []Key{
				{At: 0, Set: PX | PY | POpacity | PStroke, X: 1, Y: 2, Opacity: 1, Stroke: "raftlook.Hot"},
			}},
			{ID: "s000000000003", Kind: KindLabel, Keys: []Key{
				{At: 0, Set: PX | PY | POpacity | PFill | PText, X: 1, Y: 2, Opacity: 1, Fill: "Ink", Text: "a \"q\"\\\n\t\x01 é"},
			}},
			{ID: "s000000000004", Kind: KindLine, From: "s0123456789ab", To: "s000000000002"},
			{ID: "s000000000005", Kind: KindGroup, Items: []string{"s0123456789ab", "s000000000003"}},
		},
		Markers: []Marker{{At: 0, Seq: 1, Kind: "caption", Text: "hi"}, {At: 5, Seq: 2, Kind: "broken", Text: "x"}},
		Slots:   []SlotEntry{{1, 0}, {2, 30}},
	}
	want := `{"format":"synod-scene/1","duration":1850,"canvas":{"w":1600,"h":1000},"theme":{"Paper":"#fafaf7","Ink":"#1b1b1f"},"shapes":[
{"id":"s0123456789ab","kind":"circle","props":{"r":6},"keys":[{"at":0,"x":800,"y":500,"opacity":1,"fill":"Accent"},{"at":300,"x":812.5,"y":400.25,"scale":2,"opacity":0.5,"rotation":-90,"effect":"glow"}],"end":900},
{"id":"s000000000002","kind":"rect","props":{"w":96,"h":56,"radius":12},"keys":[{"at":0,"x":1,"y":2,"opacity":1,"stroke":"raftlook.Hot"}]},
{"id":"s000000000003","kind":"label","props":{},"keys":[{"at":0,"x":1,"y":2,"opacity":1,"fill":"Ink","text":"a \"q\"\\\n\t\u0001 é"}]},
{"id":"s000000000004","kind":"line","props":{"from":"s0123456789ab","to":"s000000000002"},"keys":[]},
{"id":"s000000000005","kind":"group","props":{"items":["s0123456789ab","s000000000003"]},"keys":[]}
],"markers":[
{"at":0,"kind":"caption","text":"hi"},
{"at":5,"kind":"broken","text":"x"}
],"trace":[
{"seq":1,"at":0},
{"seq":2,"at":30}
]}
`
	if got := encScene(t, s); got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestEncodeSceneEmptyArrays(t *testing.T) {
	got := encScene(t, &Scene{Theme: []ThemeColor{}})
	want := `{"format":"synod-scene/1","duration":0,"canvas":{"w":1600,"h":1000},"theme":{},"shapes":[],"markers":[],"trace":[]}` + "\n"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestEncodeSceneEndOptional(t *testing.T) {
	sh := SceneShape{ID: "a", Kind: KindLabel, End: 0, HasEnd: true}
	if got := encScene(t, &Scene{Shapes: []SceneShape{sh}}); !strings.Contains(got, `"keys":[],"end":0}`) {
		t.Fatalf("end 0 missing: %s", got)
	}
	sh.HasEnd = false
	if got := encScene(t, &Scene{Shapes: []SceneShape{sh}}); strings.Contains(got, `"end"`) {
		t.Fatalf("unexpected end: %s", got)
	}
}

func TestEncodeSceneRejects(t *testing.T) {
	bad := map[string]float64{"NaN": math.NaN(), "+Inf": math.Inf(1), "-Inf": math.Inf(-1), "-0": math.Copysign(0, -1)}
	for name, f := range bad {
		for _, sh := range []SceneShape{
			{ID: "a", Kind: KindCircle, R: f},
			{ID: "a", Kind: KindRect, W: 1, H: f},
			{ID: "a", Kind: KindLabel, Keys: []Key{{Set: PRotation, Rotation: f}}},
			{ID: "a", Kind: KindLabel, Keys: []Key{{Set: PX, X: f}}},
		} {
			var b bytes.Buffer
			if err := EncodeScene(&b, &Scene{Shapes: []SceneShape{sh}}); err == nil {
				t.Errorf("%s accepted in %+v", name, sh)
			}
		}
	}
	// A value whose bit is not set is not written, whatever it holds.
	if got := encScene(t, &Scene{Shapes: []SceneShape{{ID: "a", Kind: KindLabel, Keys: []Key{{Set: PX, X: 1, Y: math.NaN()}}}}}); !strings.Contains(got, `{"at":0,"x":1}`) {
		t.Fatalf("got %s", got)
	}
	var b bytes.Buffer
	if err := EncodeScene(&b, &Scene{Shapes: []SceneShape{{ID: "a", Kind: "blob"}}}); err == nil {
		t.Error("unknown kind accepted")
	}
}

func TestRuleText(t *testing.T) {
	f := Fact{Ev: "sim.Broken", Rule: "dir/a.synod:3: always x", RulePos: ir.Pos{File: "dir/a.synod", Line: 3, Col: 1}}
	o := Options{Paths: map[string]string{"dir/a.synod": "m/a.synod"}}
	if got := RuleText(f, o); got != "m/a.synod:3: always x" {
		t.Fatal(got)
	}
}
