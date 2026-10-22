package main

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

type lexFile struct {
	File   string `json:"file"`
	Tokens []struct {
		Line, Col, Len int
		Role           string
		Def            any
	} `json:"tokens"`
}

func lexCheck(t *testing.T, edit func(src string) string) (string, []lexFile) {
	t.Helper()
	gossipDir(t)
	write(t, "gossip/gossip.synod", edit(read(t, "gossip/gossip.synod")))
	code, out, errs := exec("check", "--json", "--tokens", "gossip")
	if code != 1 || errs != "" {
		t.Fatalf("code=%d stderr=%q out=%.300s", code, errs, out)
	}
	var doc struct{ Files []lexFile }
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatal(err)
	}
	return out, doc.Files
}

func checkLexical(t *testing.T, files []lexFile) {
	t.Helper()
	got := map[string]bool{}
	for _, f := range files {
		got[f.File] = true
		if f.File == "gossip/demo.synod" {
			// main package: resolved normally, may use other roles.
			continue
		}
		if len(f.Tokens) == 0 {
			t.Errorf("%s has no tokens", f.File)
		}
		for _, tk := range f.Tokens {
			switch tk.Role {
			case "language", "value", "comment", "name":
			default:
				t.Fatalf("%s:%d:%d role %q", f.File, tk.Line, tk.Col, tk.Role)
			}
			if tk.Def != nil {
				t.Fatalf("%s:%d:%d has def", f.File, tk.Line, tk.Col)
			}
		}
	}
	if !got["gossip/gossip.synod"] {
		t.Errorf("broken file not listed: %v", got)
	}
}

var testdataAbs, _ = filepath.Abs("testdata")

func checkGolden(t *testing.T, name, out string) {
	t.Helper()
	golden := filepath.Join(testdataAbs, name)
	if *update {
		write(t, golden, out)
		return
	}
	if want := read(t, golden); want != out {
		t.Errorf("%s differs; run go test ./cmd/synod -run Lexical -update", name)
	}
}

func TestLexicalParseError(t *testing.T) {
	out, files := lexCheck(t, func(s string) string {
		return strings.Replace(s, "define Join [n: Node] to [seed: node]", "define Join (n: Node] to [seed: node]", 1)
	})
	if !strings.Contains(out, `"ok":false`) {
		t.Errorf("ok should be false")
	}
	checkLexical(t, files)
	// A parse error comes after a complete lex: the last line is tokenized.
	for _, f := range files {
		if f.File == "gossip/gossip.synod" {
			last := f.Tokens[len(f.Tokens)-1]
			if last.Line < 50 {
				t.Errorf("tokens stop early at line %d", last.Line)
			}
		}
	}
	checkGolden(t, "broken-parse.tokens.json", out)
}

func TestLexicalLexError(t *testing.T) {
	out, files := lexCheck(t, func(s string) string {
		return strings.Replace(s, "  set n.peers to {seed}", "\tset n.peers to {seed}", 1)
	})
	checkLexical(t, files)
	for _, f := range files {
		if f.File == "gossip/gossip.synod" {
			for _, tk := range f.Tokens {
				if tk.Line >= 20 {
					t.Fatalf("token at %d:%d is not before the lex error", tk.Line, tk.Col)
				}
			}
			if f.Tokens[len(f.Tokens)-1].Line != 19 {
				t.Errorf("last token on line %d, want 19", f.Tokens[len(f.Tokens)-1].Line)
			}
		}
	}
	checkGolden(t, "broken-lex.tokens.json", out)
}

func TestLexicalSiblingOfBrokenFile(t *testing.T) {
	gossipDir(t)
	write(t, "gossip/extra.synod", "# package gossip\nbroken (\n")
	code, out, _ := exec("check", "--json", "--tokens", "gossip")
	if code != 1 {
		t.Fatalf("code=%d", code)
	}
	var doc struct{ Files []lexFile }
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatal(err)
	}
	checkLexical(t, doc.Files)
	for _, f := range doc.Files {
		if f.File == "gossip/gossip.synod" {
			return
		}
	}
	t.Error("sibling gossip.synod not listed")
}

func tokensOf(t *testing.T, args ...string) (int, map[string]lexFile) {
	t.Helper()
	code, out, errs := exec(args...)
	if errs != "" {
		t.Fatalf("stderr %q", errs)
	}
	var doc struct{ Files []lexFile }
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatal(err)
	}
	m := map[string]lexFile{}
	for _, f := range doc.Files {
		m[f.File] = f
	}
	return code, m
}

func onlyLexical(t *testing.T, f lexFile) {
	t.Helper()
	if len(f.Tokens) == 0 {
		t.Errorf("%s has no tokens", f.File)
	}
	for _, tk := range f.Tokens {
		if tk.Role == "phrase" || tk.Role == "field" || tk.Role == "type" || tk.Def != nil {
			t.Fatalf("%s:%d:%d is not lexical: %s", f.File, tk.Line, tk.Col, tk.Role)
		}
	}
}

func TestLexicalBrokenMainFile(t *testing.T) {
	for name, edit := range map[string]string{"parse": "x (\n", "lex": "\tx\n"} {
		t.Run(name, func(t *testing.T) { brokenMain(t, name, edit) })
	}
}

func brokenMain(t *testing.T, name, edit string) {
	{
		gossipDir(t)
		write(t, "gossip/demo.synod", read(t, "gossip/demo.synod")+edit)
		code, files := tokensOf(t, "check", "--json", "--tokens", "gossip")
		if code != 1 {
			t.Fatalf("%s: code=%d", name, code)
		}
		for _, f := range []string{"gossip/demo.synod", "gossip/gossip.synod"} {
			got, ok := files[f]
			if !ok {
				t.Fatalf("%s: %s not listed", name, f)
			}
			onlyLexical(t, got)
		}
	}
}

func TestLexicalExtraMainFile(t *testing.T) {
	gossipDir(t)
	write(t, "gossip/second.synod", "# package main\ndefine main\n  sim.Seed 1\n")
	_, files := tokensOf(t, "check", "--json", "--tokens", "gossip")
	got, ok := files["gossip/second.synod"]
	if !ok {
		t.Fatal("extra main file not listed")
	}
	onlyLexical(t, got)
}
