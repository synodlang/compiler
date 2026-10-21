package scene

import (
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/synodlang/compiler/internal/trace"
)

func TestEmitEncodes(t *testing.T) {
	w := newTW(1000)
	ball := w.mk(t, trace.KindCircle, 0)
	tag := w.mk(t, trace.KindLabel, 0)
	g, err := w.group([]int{ball, tag}, "g", 0)
	must(t, err)
	c, _ := w.center()
	ln, err := w.newShape(shapeSpec{Kind: trace.KindLine, From: g, To: c}, "ln", 0)
	must(t, err)
	must(t, w.set(ball, trace.PFill, pv{S: "raftlook.Hot"}, 0))
	must(t, w.set(tag, trace.PFill, pv{S: "Accent"}, 0))
	must(t, w.set(ln, trace.PStroke, pv{S: "Alpha"}, 0))
	gone := w.mk(t, trace.KindCircle, 700) // created and deleted at once: unwritten
	w.delete(gone, 700)
	must(t, w.tween(g, trace.PX, pv{N: 1000}, 100, 900))
	w.delete(ball, 1500)
	unref, _ := w.newShape(shapeSpec{Kind: trace.KindCircle}, "u", 0)
	_ = unref

	sc := w.emit(emitInput{
		Cursor:  1200,
		Slots:   []trace.SlotEntry{{Seq: 1, At: 0}, {Seq: 2, At: 1200}},
		Markers: []trace.Marker{{At: 50, Seq: 3, Kind: "caption", Text: "b"}, {At: 50, Seq: 2, Kind: "caption", Text: "a"}, {At: 10, Seq: 9, Kind: "pause", Text: "q"}},
		Hex: func(n string) (string, bool) {
			return map[string]string{"raftlook.Hot": "#ff0000", "Alpha": "#00ff00"}[n], true
		},
	})
	if sc.Duration != 1500 {
		t.Errorf("duration %d", sc.Duration)
	}
	// ball, tag, group, center (referenced by the line), line, unref; not `gone`
	if len(sc.Shapes) != 6 {
		t.Fatalf("shapes %d", len(sc.Shapes))
	}
	if sc.Markers[0].Kind != "pause" || sc.Markers[1].Text != "a" || sc.Markers[2].Text != "b" {
		t.Errorf("markers %+v", sc.Markers)
	}
	var names []string
	for _, c := range sc.Theme {
		names = append(names, c.Name)
	}
	if got := strings.Join(names, " "); got != "Paper Ink Line Quiet Accent Good Warning Critical Alpha raftlook.Hot" {
		t.Errorf("theme %s", got)
	}
	if sc.Theme[9].Hex != "#ff0000" || sc.Theme[8].Hex != "#00ff00" {
		t.Errorf("hex %+v", sc.Theme)
	}
	grp := sc.Shapes[2]
	if grp.Kind != trace.KindGroup || len(grp.Items) != 2 || grp.Items[0] != sc.Shapes[0].ID {
		t.Errorf("group %+v", grp)
	}
	if ls := sc.Shapes[4]; ls.From != grp.ID || ls.To != sc.Shapes[3].ID {
		t.Errorf("line %+v", ls)
	}
	if !sc.Shapes[0].HasEnd || sc.Shapes[0].End != 1500 || sc.Shapes[1].HasEnd {
		t.Error("end")
	}
	var buf bytes.Buffer
	if err := trace.EncodeScene(&buf, sc); err != nil {
		t.Fatalf("EncodeScene: %v", err)
	}
	out := buf.String()
	if !strings.HasPrefix(out, `{"format":"synod-scene/1","duration":1500,`) || strings.Contains(out, "-0,") {
		t.Errorf("output:\n%s", out)
	}
	// first keys: line has no position, others have x, y, opacity
	for _, s := range sc.Shapes {
		if len(s.Keys) == 0 {
			t.Errorf("%s has no keys", s.ID)
			continue
		}
		k := s.Keys[0]
		if s.Kind == trace.KindLine {
			if k.Set&(trace.PX|trace.PY) != 0 || k.Set&trace.POpacity == 0 {
				t.Errorf("line key bits %b", k.Set)
			}
		} else if k.Set&(trace.PX|trace.PY|trace.POpacity) != trace.PX|trace.PY|trace.POpacity {
			t.Errorf("first key bits %b", k.Set)
		}
		for i := 1; i < len(s.Keys); i++ {
			if s.Keys[i].At <= s.Keys[i-1].At {
				t.Errorf("keys not increasing: %v", s.Keys)
			}
		}
	}
}

func TestEmitEmptyAndNoCenter(t *testing.T) {
	w := newTW(1000)
	if _, err := w.center(); err != nil {
		t.Fatal(err)
	}
	sc := w.emit(emitInput{Cursor: 5})
	if len(sc.Shapes) != 0 || sc.Duration != 5 || len(sc.Theme) != 8 {
		t.Fatalf("%+v", sc)
	}
	if err := trace.EncodeScene(&bytes.Buffer{}, sc); err != nil {
		t.Fatal(err)
	}
}

func TestEmitDeterministic(t *testing.T) {
	build := func() string {
		w := newTW(2000)
		for i := 0; i < 20; i++ {
			s := w.mk(t, trace.KindCircle, int64(i))
			must(t, w.set(s, trace.PFill, pv{S: "Z"}, int64(i)))
			must(t, w.set(s, trace.PFill, pv{S: "A"}, int64(i)+10))
		}
		var b bytes.Buffer
		must(t, trace.EncodeScene(&b, w.emit(emitInput{})))
		return b.String()
	}
	if build() != build() {
		t.Fatal("not deterministic")
	}
}

// The fallback theme table must match std/theme/theme.synod.
func TestThemeTableMatchesStd(t *testing.T) {
	src, err := os.ReadFile(filepath.Join("..", "stdlib", "std", "theme", "theme.synod"))
	if err != nil {
		t.Skip("std theme not found")
	}
	re := regexp.MustCompile(`(?m)^define (\w+)\n  result (#[0-9A-Fa-f]{6})`)
	ms := re.FindAllStringSubmatch(string(src), -1)
	if len(ms) != len(themeColors) {
		t.Fatalf("theme.synod has %d colors, table %d", len(ms), len(themeColors))
	}
	for i, m := range ms {
		if m[1] != themeColors[i].Name || !strings.EqualFold(m[2], themeColors[i].Hex) {
			t.Errorf("color %d: %v vs %+v", i, m[1:], themeColors[i])
		}
	}
}
