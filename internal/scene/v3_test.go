package scene

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/synodlang/compiler/internal/ir"
	"github.com/synodlang/compiler/internal/stdlib"
	"github.com/synodlang/compiler/internal/trace"
	"github.com/synodlang/compiler/internal/types"
)

// descV3 renders the properties of format 0.3 of keys compactly.
func descV3(ks []trace.Key) string {
	var parts []string
	for _, k := range ks {
		var f []string
		add := func(bit uint32, s string) {
			if k.Set&bit != 0 {
				f = append(f, s)
			}
		}
		add(trace.PCurve, "curve="+fmtN(k.Curve))
		add(trace.PTrail, "trail="+fmtN(k.Trail))
		add(trace.PDash, "dash="+fmtN(k.DashOn)+","+fmtN(k.DashOff))
		add(trace.PDraw, "draw="+fmtN(k.Draw))
		add(trace.PHalo, "halo="+fmtN(k.Halo))
		add(trace.PIcon, "icon="+k.Icon)
		add(trace.PCount, "count="+fmtN(k.Count))
		add(trace.PRole, "role="+k.Role)
		add(trace.PFace, "face="+k.Face)
		if len(f) > 0 {
			parts = append(parts, fmtI(k.At)+"{"+strings.Join(f, " ")+"}")
		}
	}
	return strings.Join(parts, " ")
}

func fmtN(f float64) string { return strconv.FormatFloat(f, 'f', -1, 64) }

func TestStepProps(t *testing.T) {
	w := newTW(1000)
	c := w.mk(t, trace.KindCircle, 0)
	must(t, w.setStep(c, trace.PCurve, pv{N: 0}, 0)) // default: nothing
	must(t, w.setStep(c, trace.PCurve, pv{N: 0.3}, 100))
	must(t, w.setStep(c, trace.PCurve, pv{N: 0.3}, 150)) // same value: nothing
	must(t, w.setStep(c, trace.PCurve, pv{N: 0}, 200))
	must(t, w.setStep(c, trace.PDash, pv{N: 4, M: 2}, 200))
	must(t, w.setStep(c, trace.PDash, pv{}, 200)) // back to default at once: key removed
	if got := descV3(w.shapes[c].keys); got != "100{curve=0.3} 200{curve=0}" {
		t.Errorf("steps %q", got)
	}
	l := w.mk(t, trace.KindLabel, 0)
	must(t, w.setStep(l, trace.PRole, pv{S: "title"}, 10))
	must(t, w.setStep(l, trace.PRole, pv{S: "label"}, 10))
	must(t, w.setStep(l, trace.PFace, pv{S: "mono"}, 20))
	if got := descV3(w.shapes[l].keys); got != "20{face=mono}" {
		t.Errorf("label %q", got)
	}
	if err := w.setStep(c, trace.PRole, pv{S: "title"}, 0); err == nil {
		t.Error("Role on a circle accepted")
	}
	g, err := w.group([]int{c, l}, "g", 0)
	must(t, err)
	if err := w.setStep(g, trace.PFace, pv{S: "mono"}, 0); err == nil {
		t.Error("Face on a group accepted")
	}
	// a dash on a group goes to its outlined members
	must(t, w.setStep(g, trace.PDash, pv{N: 3, M: 3}, 300))
	if got := descV3(w.shapes[c].keys); !strings.Contains(got, "300{dash=3,3}") {
		t.Errorf("group dash %q", got)
	}
	if got := descV3(w.shapes[l].keys); strings.Contains(got, "dash") {
		t.Errorf("a label got a dash: %q", got)
	}
}

func TestDrawHaloIconCount(t *testing.T) {
	w := newTW(1000)
	r := w.mk(t, trace.KindRect, 0)
	must(t, w.tween(r, trace.PDraw, pv{N: 1}, 0, 100)) // default: nothing
	if got := descV3(w.shapes[r].keys); got != "" {
		t.Errorf("draw to default wrote %q", got)
	}
	must(t, w.tween(r, trace.PDraw, pv{N: 0}, 0, 0))
	must(t, w.tween(r, trace.PDraw, pv{N: 1}, 0, 600))
	if got := descV3(w.shapes[r].keys); got != "0{draw=0} 600{draw=1}" {
		t.Errorf("reveal %q", got)
	}
	if w.busyUntil(r) != 600 {
		t.Errorf("busy %d", w.busyUntil(r))
	}
	if err := w.tween(r, trace.PDraw, pv{N: 2}, 0, 0); err == nil {
		t.Error("draw 2 accepted")
	}
	if err := w.tween(r, trace.PHalo, pv{N: -1}, 0, 0); err == nil {
		t.Error("halo -1 accepted")
	}
	if err := w.tween(r, trace.PIcon, pv{S: "mail"}, 0, 0); err == nil {
		t.Error("icon on a rect accepted")
	}
	if err := w.tween(r, trace.PCount, pv{N: 1}, 0, 0); err == nil {
		t.Error("count on a rect accepted")
	}
	ic, err := w.newShape(shapeSpec{Kind: trace.KindIcon, Text: "mail"}, "ic", 0)
	must(t, err)
	must(t, w.tween(ic, trace.PIcon, pv{S: "cross"}, 100, 300))
	if got := descV3(w.shapes[ic].keys); got != "0{icon=mail} 100{icon=mail} 400{icon=cross}" {
		t.Errorf("icon %q", got)
	}
	if err := w.tween(ic, trace.PIcon, pv{S: "castle"}, 0, 0); err == nil {
		t.Error("unknown icon accepted")
	}
	if _, err := w.newShape(shapeSpec{Kind: trace.KindIcon, Text: "castle"}, "bad", 0); err == nil {
		t.Error("unknown icon created")
	}
	ct, err := w.newShape(shapeSpec{Kind: trace.KindCounter, Count: 7}, "ct", 0)
	must(t, err)
	if err := w.tween(ct, trace.PCount, pv{N: -1}, 0, 0); err == nil {
		t.Error("count -1 accepted")
	}
	if _, err := w.newShape(shapeSpec{Kind: trace.KindCounter, Count: -1}, "neg", 0); err == nil {
		t.Error("negative counter created")
	}
	if wd, h := w.sizeOf(w.shapes[ct]); wd != 8 || h != 18 {
		t.Errorf("counter size %v %v", wd, h)
	}
	if wd, h := w.sizeOf(w.shapes[ic]); wd != 32 || h != 32 {
		t.Errorf("icon size %v %v", wd, h)
	}
	// draw and halo of a group go to every member, labels included
	l := w.mk(t, trace.KindLabel, 0)
	g, err := w.group([]int{r, l}, "g", 0)
	must(t, err)
	must(t, w.tween(g, trace.PHalo, pv{N: 1}, 700, 100))
	if descV3(w.shapes[l].keys) != "700{halo=0} 800{halo=1}" || descV3(w.shapes[r].keys) != "0{draw=0} 600{draw=1} 700{halo=0} 800{halo=1}" {
		t.Errorf("group halo: %q / %q", descV3(w.shapes[l].keys), descV3(w.shapes[r].keys))
	}
}

func TestEffectSet(t *testing.T) {
	for name, ms := range map[string]int64{"glow": 600, "indicate": 600, "flash": 500, "circumscribe": 900, "ripple": 700, "sweep": 800} {
		w := newTW(2000)
		c := w.mk(t, trace.KindCircle, 0)
		must(t, w.effect(c, name, 100))
		want := "0{" + home + " o=1} 100{fx=" + name + "} " + fmtI(100+ms/2) + "{fx=none}"
		if got := w.keys(c); got != want {
			t.Errorf("%s: %q want %q", name, got, want)
		}
	}
	w := newTW(1000)
	c := w.mk(t, trace.KindCircle, 0)
	for _, bad := range []string{"blur", "trail", "bogus", ""} {
		if err := w.effect(c, bad, 0); err == nil {
			t.Errorf("effect %q accepted", bad)
		}
	}
}

func TestFocusCamera(t *testing.T) {
	w := newTW(1000)
	must(t, w.focus(CenterX, CenterY, 1, 0, 100)) // already there: nothing
	if len(w.cam) != 0 {
		t.Fatalf("camera %v", w.cam)
	}
	must(t, w.focus(CenterX+100, CenterY, 2, 100, 1000))
	cam := func() string {
		var b strings.Builder
		for _, k := range w.cam {
			b.WriteString(fmtI(k.At) + ":" + fmtN(k.X) + "," + fmtN(k.Y) + "," + fmtN(k.Zoom) + " ")
		}
		return strings.TrimSpace(b.String())
	}
	if got := cam(); got != fmt.Sprintf("100:%v,%v,1 1100:%v,%v,2", CenterX, CenterY, CenterX+100, CenterY) {
		t.Errorf("camera %q", got)
	}
	// cut in the middle: hold the interpolated camera
	must(t, w.focus(CenterX, CenterY, 1, 600, 100))
	if got := cam(); got != fmt.Sprintf("100:%[1]v,%[2]v,1 600:%[3]v,%[2]v,1.5 700:%[1]v,%[2]v,1", CenterX, CenterY, CenterX+50) {
		t.Errorf("cut %q", got)
	}
	// an instant change pins the previous camera one ms before
	must(t, w.focus(700, 400, 3, 900, 0))
	if got := cam(); got != fmt.Sprintf("100:%[1]v,%[2]v,1 600:%[3]v,%[2]v,1.5 700:%[1]v,%[2]v,1 899:%[1]v,%[2]v,1 900:700,400,3", CenterX, CenterY, CenterX+50) {
		t.Errorf("instant %q", got)
	}
	if err := w.focus(0, 0, 1, 0, -1); err == nil {
		t.Error("negative duration accepted")
	}
	// spring easing overshoots
	sw := newWorld(newClock(1000, Ordered, 0), worldConfig{Theme: &worldTheme{Radius: 12, LinkOpacity: 1, Fill: 1, Spring: true}})
	must(t, sw.focus(CenterX+200, CenterY, 1, 0, 1000))
	if c := sw.camAt(500); c.X <= CenterX+100 {
		t.Errorf("spring camera at the middle: %v", c)
	}
}

func TestSpringEase(t *testing.T) {
	if ease(true, 0) != 0 || ease(true, 1) != 1 {
		t.Error("spring ends")
	}
	if ease(true, 0.7) <= 1 {
		t.Errorf("spring should overshoot at 0.7: %v", ease(true, 0.7))
	}
	if ease(false, 0.5) != 0.5 {
		t.Error("smootherstep middle")
	}
}

func TestBakedTheme(t *testing.T) {
	th := worldTheme{Radius: 4, LinkOpacity: 0.6, Fill: 1.5}
	w := newWorld(newClock(1000, Ordered, 0), worldConfig{Theme: &th})
	r, _ := w.newShape(shapeSpec{Kind: trace.KindRect, W: 10, H: 10}, "r", 0)
	l, _ := w.newShape(shapeSpec{Kind: trace.KindLine, From: r, To: r}, "l", 0)
	sc := w.emit(emitInput{})
	if sc.Shapes[0].Radius != 4 {
		t.Errorf("radius %v", sc.Shapes[0].Radius)
	}
	if got := w.keys(l); got != "0{o=0.6 stroke=Line}" {
		t.Errorf("line %s", got)
	}
	if x, y := slotPosF("row", 0, 2, 1.5); x != CenterX-120 || y != LayoutCY {
		t.Errorf("row %v %v", x, y)
	}
	if x, y := slotPosF("ring", 0, 3, 1.5); x != CenterX || y != LayoutCY-RingRY*1.5 {
		t.Errorf("ring %v %v", x, y)
	}
	if x, y := slotPosF("grid", 0, 4, 0.5); x != CenterX-50 || y != LayoutCY-40 {
		t.Errorf("grid %v %v", x, y)
	}
}

func TestStyleOmitsDefaults(t *testing.T) {
	if (theme{}).style() != nil {
		t.Error("default theme has a style")
	}
	var b bytes.Buffer
	st := trace.DefaultStyle()
	st.Sketch, st.Sans, st.Glow = 1, "Shantell Sans", 12
	if err := trace.EncodeScene(&b, &trace.Scene{Style: &st}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(b.String(), `"theme":{},"style":{"sans":"Shantell Sans","glow":12,"sketch":1},"shapes":[]`) {
		t.Errorf("style %s", b.String())
	}
}

func TestFNVAndMessageColor(t *testing.T) {
	if fnv1a32("") != 2166136261 || fnv1a32("a") != 0xE40C292C || fnv1a32("foobar") != 0xBF9CF968 {
		t.Errorf("fnv1a32 %x %x", fnv1a32("a"), fnv1a32("foobar"))
	}
	seen := map[string]bool{}
	for _, n := range []string{"raft.Vote", "raft.Append", "paxos.Prepare", "paxos.Promise", "x.A", "x.B", "x.C", "x.D"} {
		seen[messageColorName(n)] = true
	}
	for k := range seen {
		if len(k) != 8 || k[:7] != "Message" || k[7] < '1' || k[7] > '4' {
			t.Errorf("bad color %q", k)
		}
	}
}

func fmtI(n int64) string { return strconv.FormatInt(n, 10) }

// TestBadArguments builds one look per bad call of the 0.3 phrases and
// expects the look fault bad-argument at the first fact (spec/09 section 9.3).
func TestBadArguments(t *testing.T) {
	cases := map[string]string{
		"icon name":         `draw (look.Icon "castle") as x`,
		"counter negative":  `draw (look.Counter (0 minus 1)) as x`,
		"tween icon label":  "draw (look.Label \"a\") as x\n  look.Tween \"icon\" of x to \"mail\" over 1s",
		"tween icon name":   "draw (look.Icon \"mail\") as x\n  look.Tween \"icon\" of x to \"castle\" over 1s",
		"tween count rect":  "draw (look.Rect 1 by 1) as x\n  look.Tween \"count\" of x to 3 over 1s",
		"tween count neg":   "draw (look.Counter 1) as x\n  look.Tween \"count\" of x to (0 minus 1) over 1s",
		"tween draw range":  "draw (look.Circle 1) as x\n  look.Tween \"draw\" of x to 2 over 1s",
		"tween halo range":  "draw (look.Circle 1) as x\n  look.Tween \"halo\" of x to (0 minus 1) over 1s",
		"role circle":       "draw (look.Circle 1) as x\n  look.Role x as \"title\"",
		"role name":         "draw (look.Label \"a\") as x\n  look.Role x as \"big\"",
		"face circle":       "draw (look.Circle 1) as x\n  look.Face x as \"mono\"",
		"face name":         "draw (look.Counter 1) as x\n  look.Face x as \"serif\"",
		"dash negative":     "draw (look.Circle 1) as x\n  look.Dash x on (0 minus 1) off 2",
		"dash gap negative": "draw (look.Circle 1) as x\n  look.Dash x on 1 off (0 minus 2)",
		"bend high":         "draw (look.Circle 1) as x\n  look.Bend x by 1.5",
		"bend low":          "draw (look.Circle 1) as x\n  look.Bend x by (0 minus 1.5)",
		"trail long":        "draw (look.Circle 1) as x\n  look.Trail x for 3s",
		"focus low":         "draw (look.Circle 1) as x\n  look.Focus x zoom 0.4 over 1s",
		"focus high":        "draw (look.Circle 1) as x\n  look.Focus x zoom 4.5 over 1s",
		"effect blur":       "draw (look.Circle 1) as x\n  look.Effect \"blur\" on x",
		"effect trail":      "draw (look.Circle 1) as x\n  look.Effect \"trail\" on x",
	}
	names := make([]string, 0, len(cases))
	for n := range cases {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			copyCase(t, "testdata/scene/travel", dir)
			write(t, filepath.Join(dir, "lk", "lk.synod"),
				"# package lk\nuse std/look\nuse std/sim\n\nwhen sim.Started\n  "+cases[name]+"\n")
			ps := checkDir(t, dir, func(s string) string { return s })
			for _, p := range ps {
				r := p.run()
				if r.Failed != nil {
					t.Fatal(r.Failed)
				}
				_, fl := Build(p.input(r.Facts), Options{})
				if fl == nil || fl.Fault.Code != "bad-argument" {
					t.Fatalf("want a bad-argument look fault, got %v", fl)
				}
			}
		})
	}
}

func copyCase(t *testing.T, from, to string) {
	t.Helper()
	for _, f := range []string{"synod.mod", "main/main.synod", "p/p.synod"} {
		data, err := os.ReadFile(filepath.Join(from, f))
		if err != nil {
			t.Fatal(err)
		}
		write(t, filepath.Join(to, f), string(data))
	}
}

func write(t *testing.T, path, s string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(s), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestExamplesUnderTheme builds every example under a theme that changes
// everything: no look fault, a valid scene, the same bytes twice, and a
// different scene from the default theme's (a theme reskins and never changes
// a fact, so the facts are the same under both).
func TestExamplesUnderTheme(t *testing.T) {
	ex := examples(t)
	th := &ir.Theme{Path: "test"}
	over := map[string]any{"Paper": "#FFFFFF", "Sketch": 1.0, "Glow": 0.0, "CornerRadius": 4.0, "LinkOpacity": 0.6,
		"NodeShape": "circle", "NodeFill": "outline", "Easing": "spring", "Sans": "Shantell Sans", "Fill": 1.5,
		"Vignette": 0.35, "Shadow": 0.15, "Drift": 6.0, "StrokeWidth": 3.0}
	for _, n := range stdlib.ThemeNames {
		v := ir.ThemeValue{Name: n.Name, Type: n.Type}
		switch {
		case n.Type == types.TColor:
			v.V = defaultColors[n.Name]
		case n.Type == types.TNumber:
			v.V = defaultNums[n.Name]
		default:
			v.V = defaultTexts[n.Name]
		}
		if o, ok := over[n.Name]; ok {
			v.V = o
		}
		th.Values = append(th.Values, v)
	}
	for _, n := range names(ex) {
		p := ex[n]
		t.Run(n, func(t *testing.T) {
			r := p.run()
			if r.Failed != nil {
				t.Fatal(r.Failed)
			}
			plain, fl := Build(p.input(r.Facts), Options{})
			if fl != nil {
				t.Fatal(fl.Fault)
			}
			saved := p.Prog.Theme
			p.Prog.Theme = th
			defer func() { p.Prog.Theme = saved }()
			sc, fl := Build(p.input(r.Facts), Options{})
			if fl != nil {
				t.Fatalf("look failed at fact %d: %v", fl.Seq, fl.Fault.Error())
			}
			checkScene(t, sc)
			a := encodeScene(t, sc)
			sc2, _ := Build(p.input(r.Facts), Options{})
			if !bytes.Equal(a, encodeScene(t, sc2)) {
				t.Error("two builds differ")
			}
			if bytes.Equal(a, encodeScene(t, plain)) || sc.Style == nil || sc.Style.Sketch != 1 {
				t.Error("the theme changed nothing")
			}
		})
	}
}
