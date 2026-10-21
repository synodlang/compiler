package format

import (
	"bytes"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/synodlang/compiler/internal/source"
)

func parseFixes(t *testing.T, path string) []Fix {
	t.Helper()
	src, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var out []Fix
	for _, line := range strings.Split(strings.TrimSpace(string(src)), "\n") {
		f := strings.Fields(line)
		lc := strings.SplitN(f[0], ":", 2)
		l, err1 := strconv.Atoi(lc[0])
		c, err2 := strconv.Atoi(lc[1])
		if err1 != nil || err2 != nil {
			t.Fatalf("%s: bad position in %q", path, line)
		}
		fx := Fix{Pos: source.Pos{Line: l, Col: c}}
		switch f[1] {
		case "rename":
			fx.Rename = f[2]
		case "drop":
			fx.DropPrefix = true
			if len(f) > 2 {
				fx.Part, _ = strconv.Atoi(f[2])
			}
		case "order":
			fx.Order = f[2:]
		default:
			t.Fatalf("%s: bad fix %q", path, line)
		}
		out = append(out, fx)
	}
	return out
}

// TestFixGoldens applies testdata/format/fix-<name>.fixes.txt to
// fix-<name>.fix.synod and compares with fix-<name>.want.synod.
func TestFixGoldens(t *testing.T) {
	dir := filepath.Join(testdata, "format")
	paths, err := filepath.Glob(filepath.Join(dir, "fix-*.fix.synod"))
	if err != nil || len(paths) < 4 {
		t.Fatalf("fix goldens: %v %v", paths, err)
	}
	for _, path := range paths {
		name := strings.TrimSuffix(filepath.Base(path), ".fix.synod")
		t.Run(name, func(t *testing.T) {
			src := mustRead(t, path)
			fixes := parseFixes(t, filepath.Join(dir, name+".fixes.txt"))
			out, d := FormatWith(source.NewFile(name, src), fixes)
			if d != nil {
				t.Fatal(d)
			}
			golden(t, filepath.Join(dir, name+".want.synod"), out)
			if name == "fix-ignored" {
				if plain := must(t, name, src); !bytes.Equal(plain, out) {
					t.Errorf("ignored fixes changed the file:\n%s", out)
				}
			}
			// The result is canonical and stable.
			if d := Check(source.NewFile(name, out)); d != nil {
				t.Errorf("output is not canonical: %s", d)
			}
			// Fixes are positions in the original file: applying the same
			// fixes to a different file must not corrupt it, and fixing the
			// output with no fixes changes nothing.
			if again := must(t, name, out); !bytes.Equal(again, out) {
				t.Errorf("not idempotent:\n%s", again)
			}
		})
	}
}

func fixed(t *testing.T, src string, fixes ...Fix) string {
	t.Helper()
	out, d := FormatWith(source.NewFile("x.synod", []byte(src)), fixes)
	if d != nil {
		t.Fatal(d)
	}
	return string(out)
}

func TestFixNilIsFormat(t *testing.T) {
	src := "# package x\nuse  std/net\ndefine A\n  x   1\n"
	a, _ := Format(source.NewFile("x.synod", []byte(src)))
	b, _ := FormatWith(source.NewFile("x.synod", []byte(src)), nil)
	if !bytes.Equal(a, b) {
		t.Errorf("Format and FormatWith(nil) differ")
	}
}

func TestFixRenameMemberAndWord(t *testing.T) {
	src := "Best.Length\nBest\n"
	got := fixed(t, src,
		Fix{Pos: source.Pos{Line: 1, Col: 1}, Rename: "best"},
		Fix{Pos: source.Pos{Line: 2, Col: 1}, Rename: "best"})
	if got != "best.Length\n\nbest\n" {
		t.Errorf("got %q", got)
	}
}

func TestFixDropPrefix(t *testing.T) {
	got := fixed(t, "set n.raft.Role to raft.X\nuse a\n",
		Fix{Pos: source.Pos{Line: 1, Col: 5}, DropPrefix: true, Part: 1},
		Fix{Pos: source.Pos{Line: 1, Col: 20}, DropPrefix: true})
	if got != "use a\n\nset n.Role to X\n" {
		t.Errorf("got %q", got)
	}
	// Out of range: ignored.
	if got = fixed(t, "a.b\n", Fix{Pos: source.Pos{Line: 1, Col: 1}, DropPrefix: true, Part: 2}); got != "a.b\n" {
		t.Errorf("got %q", got)
	}
	// DropPrefix wins over Rename.
	got = fixed(t, "a.b.c\n", Fix{Pos: source.Pos{Line: 1, Col: 1}, DropPrefix: true, Rename: "z"})
	if got != "b.c\n" {
		t.Errorf("got %q", got)
	}
}

func TestFixOrder(t *testing.T) {
	msg := source.Pos{Line: 1, Col: 6}
	cases := []struct {
		src, want string
		order     []string
	}{
		{"set x to (message T with b 1, a 2)\n", "set x to (message T with a 2, b 1)\n", []string{"a", "b"}},
		{"set x to (message T with a 2, b 1)\n", "set x to (message T with a 2, b 1)\n", []string{"a", "b"}},
		// Not exactly those words: left alone.
		{"set x to (message T with b 1, a 2)\n", "set x to (message T with b 1, a 2)\n", []string{"a", "c"}},
		{"set x to (message T with b 1, b 2)\n", "set x to (message T with b 1, b 2)\n", []string{"b", "b"}},
		{"set x to (message T with b 1, a 2)\n", "set x to (message T with b 1, a 2)\n", []string{"a"}},
		// A setting of several items is not touched.
		{"set x to (message T with b 1 2, a 2)\n", "set x to (message T with b 1 2, a 2)\n", []string{"a", "b"}},
	}
	for _, c := range cases {
		pos := msg
		pos.Col = strings.Index(c.src, "message") + 1
		if got := fixed(t, c.src, Fix{Pos: pos, Order: c.order}); got != c.want {
			t.Errorf("%q with %v: got %q, want %q", c.src, c.order, got, c.want)
		}
	}
}

func TestFixIgnoredWhenAbsent(t *testing.T) {
	src := "# package x\ndefine A [a: number]\n  show a 1\n"
	want, _ := Format(source.NewFile("x.synod", []byte(src)))
	got := fixed(t, src,
		Fix{Pos: source.Pos{Line: 40, Col: 1}, Rename: "q"},
		Fix{Pos: source.Pos{Line: 3, Col: 3}, Order: []string{"a"}},
		Fix{Pos: source.Pos{Line: 3, Col: 10}, DropPrefix: true},
		Fix{Pos: source.Pos{Line: 3, Col: 9}, Rename: ""})
	if got != string(want) {
		t.Errorf("got %q", got)
	}
}

func TestCheckWith(t *testing.T) {
	src := "# package x\nset m to (message T with b 1, a 2)\n\nset n to (message T with b 1, a 2)\n"
	fixes := []Fix{
		{Pos: source.Pos{Line: 2, Col: 11}, Order: []string{"a", "b"}},
		{Pos: source.Pos{Line: 4, Col: 11}, Order: []string{"a", "b"}},
	}
	f := source.NewFile("x.synod", []byte(src))
	d := CheckWith(f, fixes)
	if d == nil || d.Code != source.NonCanonical || d.Line != 2 || !strings.Contains(d.Hint, `write "set m to (message T with a 2, b 1)"`) {
		t.Fatalf("got %v", d)
	}
	if CheckWith(f, nil) != nil {
		t.Errorf("no fixes: want canonical")
	}
	out, _ := FormatWith(f, fixes)
	if d := CheckWith(source.NewFile("x.synod", out), fixes); d != nil {
		t.Errorf("fixed output still differs: %v", d)
	}
	// A file that does not parse yields its syntax error.
	bad := source.NewFile("x.synod", []byte("# package x\n  x\n"))
	if d := CheckWith(bad, fixes); d == nil || d.Code == source.NonCanonical {
		t.Errorf("got %v", d)
	}
}
