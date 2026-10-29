package runtest

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/synodlang/compiler/internal/scene"
)

// fitShape is the part of a scene shape the fit test reads.
type fitShape struct {
	ID    string `json:"id"`
	Kind  string `json:"kind"`
	Node  string `json:"node"`
	Msg   int    `json:"msg"`
	End   *int64 `json:"end"`
	Props struct {
		Items []string `json:"items"`
		Size  float64  `json:"size"`
		W     float64  `json:"w"`
		H     float64  `json:"h"`
		R     float64  `json:"r"`
	} `json:"props"`
	Keys []struct {
		At      int64    `json:"at"`
		X       *float64 `json:"x"`
		Y       *float64 `json:"y"`
		Opacity *float64 `json:"opacity"`
		Text    *string  `json:"text"`
	} `json:"keys"`
}

// fitRowChars is the longest state row (spec/09 §9.3, sim.StateRows): the
// builder cuts a longer one, so no row of a scene is wider than this.
const fitRowChars = scene.StateRowMax

// fitClear is the least clear space between a state row and a card or an
// icon: the Place gap of spec/09 §9.2 (a row under its own card sits exactly
// there), less the 0.01 rounding of positions.
const fitClear = scene.PlaceGap - 0.05

// The lesson note and the reason of a run sit at fixed heights on the canvas
// edge (look/basic, looks/lumen, looks/doodle); they are laid over the stage,
// so the test measures them against cards and icons but not against rows.
const (
	fitNoteY   = 604.0
	fitReasonY = 46.0
)

type fitBox struct {
	name           string
	x0, y0, x1, y1 float64
}

// gap is the space between two boxes: negative when they overlap.
func (a fitBox) gap(b fitBox) float64 {
	dx := math.Max(a.x0-b.x1, b.x0-a.x1)
	dy := math.Max(a.y0-b.y1, b.y0-a.y1)
	if dx < 0 && dy < 0 {
		return math.Max(dx, dy)
	}
	return math.Hypot(math.Max(dx, 0), math.Max(dy, 0))
}

// fitState is one shape at one moment: the last value of each key at or
// before the moment.
type fitState struct {
	s         *fitShape
	x, y, op  float64
	text      string
	isVisible bool
}

func stateAt(s *fitShape, at int64) fitState {
	st := fitState{s: s, op: 1}
	if len(s.Keys) == 0 || s.Keys[0].At > at || (s.End != nil && *s.End <= at) {
		return st
	}
	for _, k := range s.Keys {
		if k.At > at {
			break
		}
		if k.X != nil {
			st.x = *k.X
		}
		if k.Y != nil {
			st.y = *k.Y
		}
		if k.Opacity != nil {
			st.op = *k.Opacity
		}
		if k.Text != nil {
			st.text = *k.Text
		}
	}
	st.isVisible = st.op > 0
	return st
}

// fitMetrics are the numbers reported for a scene at its last key.
type fitMetrics struct {
	bbox         fitBox
	minClear     float64
	longestRow   int
	rows, cards  int
	icons        int
	longestLabel string
}

// fitScene checks one scene file at one moment (at) and returns its metrics.
func fitScene(t *testing.T, sc []fitShape, canvasW, canvasH float64, at int64) fitMetrics {
	t.Helper()
	byID := map[string]*fitShape{}
	member := map[string]bool{}
	for i := range sc {
		byID[sc[i].ID] = &sc[i]
		for _, m := range sc[i].Props.Items {
			member[m] = true
		}
	}
	var cards, rows, edges, icons []fitBox
	for i := range sc {
		s := &sc[i]
		st := stateAt(s, at)
		if !st.isVisible || member[s.ID] || s.Msg != 0 {
			continue
		}
		switch {
		case s.Kind == "group" && s.Node != "":
			b := fitBox{name: "card " + s.Node, x0: math.Inf(1), y0: math.Inf(1), x1: math.Inf(-1), y1: math.Inf(-1)}
			for _, id := range s.Props.Items {
				m := byID[id]
				ms := stateAt(m, at)
				var w, h float64
				switch m.Kind {
				case "rect":
					w, h = m.Props.W, m.Props.H
				case "circle":
					w, h = 2*m.Props.R, 2*m.Props.R
				case "label":
					w, h = scene.LabelCharW*float64(len([]rune(ms.text))), scene.LabelH
				case "icon":
					w, h = m.Props.Size, m.Props.Size
				default:
					continue
				}
				b.x0 = math.Min(b.x0, st.x+ms.x-w/2)
				b.x1 = math.Max(b.x1, st.x+ms.x+w/2)
				b.y0 = math.Min(b.y0, st.y+ms.y-h/2)
				b.y1 = math.Max(b.y1, st.y+ms.y+h/2)
			}
			if !math.IsInf(b.x0, 1) {
				cards = append(cards, b)
			}
		case s.Kind == "label" && st.text != "":
			w, h := scene.LabelCharW*float64(len([]rune(st.text))), scene.LabelH
			b := fitBox{"row " + st.text, st.x - w/2, st.y - h/2, st.x + w/2, st.y + h/2}
			if st.y == fitNoteY || st.y == fitReasonY {
				b.name = "edge " + st.text
				edges = append(edges, b)
			} else {
				rows = append(rows, b)
			}
		case s.Kind == "circle" && s.Node != "":
			// a head drawn above a card (looks/doodle)
			icons = append(icons, fitBox{"head " + s.Node, st.x - s.Props.R, st.y - s.Props.R, st.x + s.Props.R, st.y + s.Props.R})
		case s.Kind == "icon":
			h := s.Props.Size / 2
			icons = append(icons, fitBox{"icon " + s.ID, st.x - h, st.y - h, st.x + h, st.y + h})
		}
	}
	if len(cards) == 0 {
		t.Fatal("no node cards found")
	}
	m := fitMetrics{minClear: math.Inf(1), rows: len(rows), cards: len(cards), icons: len(icons)}
	m.bbox = fitBox{x0: math.Inf(1), y0: math.Inf(1), x1: math.Inf(-1), y1: math.Inf(-1)}
	// (a) everything inside the canvas
	for _, bs := range [][]fitBox{cards, rows, edges, icons} {
		for _, b := range bs {
			if b.x0 < 0 || b.y0 < 0 || b.x1 > canvasW || b.y1 > canvasH {
				t.Errorf("@%d %s (%.0f,%.0f)-(%.0f,%.0f) leaves the %.0fx%.0f canvas", at, b.name, b.x0, b.y0, b.x1, b.y1, canvasW, canvasH)
			}
			m.bbox.x0, m.bbox.y0 = math.Min(m.bbox.x0, b.x0), math.Min(m.bbox.y0, b.y0)
			m.bbox.x1, m.bbox.y1 = math.Max(m.bbox.x1, b.x1), math.Max(m.bbox.y1, b.y1)
		}
	}
	// no two cards overlap
	for i, a := range cards {
		for _, b := range cards[i+1:] {
			if a.gap(b) < 0 {
				t.Errorf("@%d %s overlaps %s", at, a.name, b.name)
			}
		}
	}
	for _, e := range edges {
		for _, bs := range [][]fitBox{cards, icons} {
			for _, b := range bs {
				if g := e.gap(b); g < fitClear {
					t.Errorf("@%d %s is %.1f from %s, the least is %.0f", at, e.name, g, b.name, scene.PlaceGap)
				}
			}
		}
	}
	for i, r := range rows {
		// (c) a state row has at most fitRowChars characters
		if n := len([]rune(r.name)) - len("row "); n > fitRowChars {
			t.Errorf("@%d %s has %d characters, the most is %d", at, r.name, n, fitRowChars)
		} else if n > m.longestRow {
			m.longestRow = n
			m.longestLabel = r.name[len("row "):]
		}
		// (b) a row is at least the Place gap clear of every card and icon
		for _, bs := range [][]fitBox{cards, icons} {
			for _, b := range bs {
				g := r.gap(b)
				m.minClear = math.Min(m.minClear, g)
				if g < fitClear {
					t.Errorf("@%d %s is %.1f from %s, the least is %.0f", at, r.name, g, b.name, scene.PlaceGap)
				}
			}
		}
		// (d) labels are pairwise disjoint
		for _, o := range rows[i+1:] {
			if r.gap(o) < 0 {
				t.Errorf("@%d %s overlaps %s", at, r.name, o.name)
			}
		}
	}
	return m
}

// fitMoments are the instants a scene is measured at: after its last key and
// at the middle key of all its key times.
func fitMoments(sc []fitShape) []int64 {
	set := map[int64]bool{}
	for _, s := range sc {
		for _, k := range s.Keys {
			set[k.At] = true
		}
	}
	var ats []int64
	for a := range set {
		ats = append(ats, a)
	}
	sort.Slice(ats, func(i, j int) bool { return ats[i] < ats[j] })
	if len(ats) == 0 {
		return nil
	}
	return []int64{ats[len(ats)/2], math.MaxInt64}
}

func checkSceneFile(t *testing.T, f string) {
	t.Helper()
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
	for k, at := range fitMoments(sc.Shapes) {
		m := fitScene(t, sc.Shapes, sc.Canvas.W, sc.Canvas.H, at)
		if k == 1 {
			t.Logf("geometry: bbox (%.0f,%.0f)-(%.0f,%.0f), %d cards, %d rows, least row clearance %.1f, longest row %d (%q)",
				m.bbox.x0, m.bbox.y0, m.bbox.x1, m.bbox.y1, m.cards, m.rows, m.minClear, m.longestRow, m.longestLabel)
		}
	}
}

// TestExamplesFitCanvas: in every example scene, at its middle key and at its
// last key, every node card, state row, icon and head lies inside the canvas, no
// two cards overlap, no state row has more than 24 characters, every row is
// 8 clear of every card and icon, and no two labels overlap (the guard for the
// canvas, ring and row constants of internal/scene, spec/09 §9.2 to §9.4).
func TestExamplesFitCanvas(t *testing.T) {
	files, err := filepath.Glob(filepath.Join("..", "..", "testdata", "run", "examples", "*", "scene.json"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no example scenes: %v", err)
	}
	for _, f := range files {
		t.Run(filepath.Base(filepath.Dir(f)), func(t *testing.T) { checkSceneFile(t, f) })
	}
}

// TestLooksFitCanvas runs the same checks on the scenes of other repositories:
// SYNOD_FIT_SCENES is a glob of scene.json files (for example the demo goldens
// of the looks module).
func TestLooksFitCanvas(t *testing.T) {
	pat := os.Getenv("SYNOD_FIT_SCENES")
	if pat == "" {
		t.Skip("set SYNOD_FIT_SCENES to a glob of scene.json files to run")
	}
	files, err := filepath.Glob(pat)
	if err != nil || len(files) == 0 {
		t.Fatalf("no scenes match %q: %v", pat, err)
	}
	for _, f := range files {
		t.Run(filepath.Base(filepath.Dir(f)), func(t *testing.T) { checkSceneFile(t, f) })
	}
}
