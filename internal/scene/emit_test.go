package scene

import (
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/synodlang/compiler/internal/stdlib"
	"github.com/synodlang/compiler/internal/trace"
	"github.com/synodlang/compiler/internal/types"
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

// The default theme tables must match std/theme/default.
func TestThemeTableMatchesStd(t *testing.T) {
	src, err := os.ReadFile(filepath.Join("..", "stdlib", "std", "theme", "default", "default.synod"))
	if err != nil {
		t.Skip("std theme/default not found")
	}
	re := regexp.MustCompile(`(?m)^define (\w+)\n  result (.+)$`)
	ms := re.FindAllStringSubmatch(string(src), -1)
	want := len(themeColorNames) + len(themeNumNames) + len(themeTextNames)
	if len(ms) != want {
		t.Fatalf("default.synod has %d names, tables %d", len(ms), want)
	}
	for _, m := range ms {
		name, lit := m[1], m[2]
		switch {
		case defaultColors[name] != "":
			if !strings.EqualFold(lit, defaultColors[name]) {
				t.Errorf("%s: %s vs %s", name, lit, defaultColors[name])
			}
		case defaultTexts[name] != "":
			if lit != strconv.Quote(defaultTexts[name]) {
				t.Errorf("%s: %s vs %q", name, lit, defaultTexts[name])
			}
		default:
			v, ok := defaultNums[name]
			if f, err := strconv.ParseFloat(lit, 64); !ok || err != nil || f != v {
				t.Errorf("%s: %s vs %v", name, lit, v)
			}
		}
	}
	if len(stdlib.ThemeNames) != want {
		t.Errorf("stdlib.ThemeNames has %d names, the tables %d", len(stdlib.ThemeNames), want)
	}
	for _, n := range stdlib.ThemeNames {
		_, c := defaultColors[n.Name]
		_, f := defaultNums[n.Name]
		_, s := defaultTexts[n.Name]
		if !c && !f && !s {
			t.Errorf("theme name %s has no default", n.Name)
		}
		if (c && n.Type != types.TColor) || (f && n.Type != types.TNumber) || (s && n.Type != types.TText) {
			t.Errorf("theme name %s has type %v", n.Name, n.Type)
		}
	}
	for i, c := range themeColors {
		if c.Name != themeColorNames[i] || c.Hex != defaultColors[c.Name] {
			t.Errorf("themeColors[%d] = %+v", i, c)
		}
	}
	if d := trace.DefaultStyle(); d != (theme{}).styleValue() {
		t.Errorf("DefaultStyle %+v differs from the default theme's style %+v", d, (theme{}).styleValue())
	}
}

func TestNodeAndMsgKeys(t *testing.T) {
	w := newTW(1000)
	a := w.mk(t, trace.KindCircle, 0)
	b := w.mk(t, trace.KindCircle, 0)
	c := w.mk(t, trace.KindCircle, 0)
	d := w.mk(t, trace.KindCircle, 0)
	must(t, w.attach("card", nodeKey(3), a, 0))
	must(t, w.attach("again", nodeKey(4), a, 0)) // the first node attachment wins
	must(t, w.attach("tag", textKey("x"), b, 0)) // a text key is not a node
	must(t, w.attach("n", numberKey(7), b, 0))   // nor a number key
	must(t, w.attach("card", nodeKey(5), c, 0))
	must(t, w.attach("card", nodeKey(5), d, 10)) // a replacement is attached too
	must(t, w.flightStart(b, a, d, 9, 0))
	must(t, w.flightStart(b, a, d, 11, 5)) // the first flight wins
	sc := w.emit(emitInput{Cursor: 100})
	got := func(i int) trace.SceneShape { return sc.Shapes[i] }
	if s := got(0); !s.HasNode || s.Node != "n3" || s.HasMsg {
		t.Errorf("a: %+v", s)
	}
	if s := got(1); s.HasNode || !s.HasMsg || s.Msg != 9 {
		t.Errorf("b: %+v", s)
	}
	if s := got(2); !s.HasNode || s.Node != "n5" {
		t.Errorf("c: %+v", s)
	}
	if s := got(3); !s.HasNode || s.Node != "n5" {
		t.Errorf("replacement: %+v", s)
	}
	var buf bytes.Buffer
	if err := trace.EncodeScene(&buf, sc); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"kind":"circle","node":"n3","props":`, `"kind":"circle","msg":9,"props":`} {
		if !strings.Contains(buf.String(), want) {
			t.Errorf("missing %s in\n%s", want, buf.String())
		}
	}
}

func TestPauseChoicesEncoding(t *testing.T) {
	sc := newTW(1000).emit(emitInput{Cursor: 10, Markers: []trace.Marker{
		{At: 1, Kind: "pause", Text: "q", Choices: []string{"a", "b"}},
		{At: 2, Kind: "pause", Text: "r"},
		{At: 3, Kind: "caption", Text: "c", Choices: []string{"ignored"}},
	}})
	var buf bytes.Buffer
	if err := trace.EncodeScene(&buf, sc); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`{"at":1,"kind":"pause","text":"q","choices":["a","b"]}`,
		`{"at":2,"kind":"pause","text":"r"}`,
		`{"at":3,"kind":"caption","text":"c"}`,
	} {
		if !strings.Contains(buf.String(), want) {
			t.Errorf("missing %s in\n%s", want, buf.String())
		}
	}
}
