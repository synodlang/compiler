package main

import (
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var update = flag.Bool("update", false, "rewrite testdata/gossip.tokens.json")

// inGossipCopy copies the gossip example to a temp folder, makes it the
// working directory, and returns the check output for "gossip".
func inGossipCopy(t *testing.T, args ...string) (int, string) {
	t.Helper()
	tmp := t.TempDir()
	ents, err := os.ReadDir(filepath.Join(examples, "gossip"))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range ents {
		data, err := os.ReadFile(filepath.Join(examples, "gossip", e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		write(t, filepath.Join(tmp, "gossip", e.Name()), string(data))
	}
	mod, err := os.ReadFile(filepath.Join(examples, "synod.mod"))
	if err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(tmp, "synod.mod"), string(mod))
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(tmp); err != nil {
		t.Fatal(err)
	}
	code, out, errs := exec(append(args, "gossip")...)
	if err := os.Chdir(old); err != nil {
		t.Fatal(err)
	}
	if errs != "" {
		t.Fatalf("stderr: %q", errs)
	}
	return code, out
}

func TestTokensGolden(t *testing.T) {
	code, out := inGossipCopy(t, "check", "--json", "--tokens")
	if code != 0 {
		t.Fatalf("code=%d out=%.300s", code, out)
	}
	golden := filepath.Join("testdata", "gossip.tokens.json")
	if *update {
		write(t, golden, out)
		return
	}
	if want := read(t, golden); out != want {
		t.Errorf("tokens differ from %s (run with -update to rewrite)", golden)
	}
}

type tokenOut struct {
	OK    bool `json:"ok"`
	Files []struct {
		File   string `json:"file"`
		Tokens []struct {
			Line, Col, Len int
			Role           string
			Def            *struct {
				File      string
				Line, Col int
			}
		}
	}
}

// The structural claims of the format: files and tokens are sorted, std
// declarations have no def, and members are split.
func TestTokensShape(t *testing.T) {
	code, out := inGossipCopy(t, "check", "--json", "--tokens")
	if code != 0 {
		t.Fatalf("code=%d", code)
	}
	var v tokenOut
	if err := json.Unmarshal([]byte(out), &v); err != nil {
		t.Fatal(err)
	}
	if !v.OK || len(v.Files) != 2 || v.Files[0].File != "gossip/demo.synod" || v.Files[1].File != "gossip/gossip.synod" {
		t.Fatalf("files: %+v", v.Files)
	}
	src, _ := os.ReadFile(filepath.Join(examples, "gossip", "gossip.synod"))
	lines := strings.Split(string(src), "\n")
	var known bool
	for _, f := range v.Files {
		pl, pc := 0, 0
		for _, tk := range f.Tokens {
			if tk.Line < pl || (tk.Line == pl && tk.Col <= pc) {
				t.Fatalf("%s: tokens not sorted at %d:%d", f.File, tk.Line, tk.Col)
			}
			pl, pc = tk.Line, tk.Col
			if tk.Def != nil && strings.HasPrefix(tk.Def.File, "std/") {
				t.Errorf("%s:%d:%d: def in std", f.File, tk.Line, tk.Col)
			}
			if f.File == "gossip/gossip.synod" && tk.Role == "field" && tk.Def != nil {
				text := []rune(lines[tk.Line-1])
				if tk.Col-1+tk.Len <= len(text) && string(text[tk.Col-1:tk.Col-1+tk.Len]) == "Known" &&
					strings.Contains(string(text[:tk.Col-1]), "n.") {
					known = true
					if !strings.HasPrefix(lines[tk.Def.Line-1], "Node has") && !strings.HasPrefix(lines[tk.Def.Line-1], "  set Known") {
						t.Errorf("Known def at %d:%d is %q", tk.Def.Line, tk.Def.Col, lines[tk.Def.Line-1])
					}
				}
			}
		}
	}
	if !known {
		t.Error("no field token with a def for n.Known in gossip.synod")
	}
}

func TestTokensOffByDefault(t *testing.T) {
	_, out := inGossipCopy(t, "check", "--json")
	if strings.Contains(out, "files") {
		t.Errorf("files key without --tokens: %.200s", out)
	}
}
