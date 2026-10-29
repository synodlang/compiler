package runtest

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/synodlang/compiler/internal/scene"
)

// fitShape is the part of a scene shape the fit test reads.
type fitShape struct {
	ID    string `json:"id"`
	Kind  string `json:"kind"`
	Node  string `json:"node"`
	End   *int64 `json:"end"`
	Props struct {
		Items []string `json:"items"`
		Size  float64  `json:"size"`
		W     float64  `json:"w"`
		H     float64  `json:"h"`
	} `json:"props"`
	Keys []struct {
		X       *float64 `json:"x"`
		Y       *float64 `json:"y"`
		Opacity *float64 `json:"opacity"`
		Text    *string  `json:"text"`
	} `json:"keys"`
}

// fitRowChars is the longest state row the test measures: rows that print a
// whole map (cassandra's Known, wal's Table) are wider than any slot and are
// the known limit of the layout (spec/09 §9.4), so they are not measured.
const fitRowChars = 24

type fitBox struct {
	name           string
	x0, y0, x1, y1 float64
}

func (a fitBox) overlaps(b fitBox) bool {
	return a.x0 < b.x1 && b.x0 < a.x1 && a.y0 < b.y1 && b.y0 < a.y1
}

// TestExamplesFitCanvas: in every example scene, at its last key, every node
// card, its state rows and every icon lie inside the canvas, no two cards
// overlap, and no state row overlaps a card (the guard for the canvas and
// ring constants of internal/scene, spec/09 §9.2 and §9.4).
func TestExamplesFitCanvas(t *testing.T) {
	files, err := filepath.Glob(filepath.Join("..", "..", "testdata", "run", "examples", "*", "scene.json"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no example scenes: %v", err)
	}
	for _, f := range files {
		name := filepath.Base(filepath.Dir(f))
		t.Run(name, func(t *testing.T) {
			b, err := os.ReadFile(f)
			if err != nil {
				t.Fatal(err)
			}
			var sc struct {
				Canvas struct{ W, H float64 } `json:"canvas"`
				Shapes []fitShape             `json:"shapes"`
			}
			if err := json.Unmarshal(b, &sc); err != nil {
				t.Fatal(err)
			}
			if sc.Canvas.W != scene.CanvasW || sc.Canvas.H != scene.CanvasH {
				t.Fatalf("canvas %vx%v, want %dx%d", sc.Canvas.W, sc.Canvas.H, scene.CanvasW, scene.CanvasH)
			}
			member := map[string]bool{}
			for _, s := range sc.Shapes {
				for _, m := range s.Props.Items {
					member[m] = true
				}
			}
			var cards, rows, icons []fitBox
			for _, s := range sc.Shapes {
				if s.End != nil || member[s.ID] || len(s.Keys) == 0 {
					continue
				}
				var x, y, op float64 = 0, 0, 1
				text := ""
				for _, k := range s.Keys {
					if k.X != nil {
						x = *k.X
					}
					if k.Y != nil {
						y = *k.Y
					}
					if k.Opacity != nil {
						op = *k.Opacity
					}
					if k.Text != nil {
						text = *k.Text
					}
				}
				if op == 0 {
					continue
				}
				switch {
				case s.Kind == "group" && s.Node != "":
					cards = append(cards, fitBox{"card " + s.Node, x - 48, y - 28, x + 48, y + 28})
				case s.Kind == "label" && text != "" && len([]rune(text)) <= fitRowChars:
					w, h := scene.LabelCharW*float64(len([]rune(text))), scene.LabelH
					rows = append(rows, fitBox{"row " + text, x - w/2, y - h/2, x + w/2, y + h/2})
				case s.Kind == "icon":
					h := s.Props.Size / 2
					icons = append(icons, fitBox{"icon " + s.ID, x - h, y - h, x + h, y + h})
				}
			}
			if len(cards) == 0 {
				t.Fatal("no node cards found")
			}
			inside := func(b fitBox) {
				if b.x0 < 0 || b.y0 < 0 || b.x1 > scene.CanvasW || b.y1 > scene.CanvasH {
					t.Errorf("%s (%.0f,%.0f)-(%.0f,%.0f) leaves the %dx%d canvas", b.name, b.x0, b.y0, b.x1, b.y1, scene.CanvasW, scene.CanvasH)
				}
			}
			for _, bs := range [][]fitBox{cards, rows, icons} {
				for _, b := range bs {
					inside(b)
				}
			}
			for i, a := range cards {
				for _, b := range cards[i+1:] {
					if a.overlaps(b) {
						t.Errorf("%s overlaps %s", a.name, b.name)
					}
				}
				for _, r := range rows {
					if a.overlaps(r) {
						t.Errorf("%s overlaps %s", r.name, a.name)
					}
				}
			}
		})
	}
}
