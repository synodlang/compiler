package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// gossipDir copies the gossip example into a temp module, makes it the
// working directory until the test ends, and returns its path.
func gossipDir(t *testing.T) string {
	t.Helper()
	tmp := t.TempDir()
	for _, name := range []string{"gossip.synod", "demo.synod"} {
		write(t, filepath.Join(tmp, "gossip", name), read(t, filepath.Join(examples, "gossip", name)))
	}
	write(t, filepath.Join(tmp, "synod.mod"), read(t, filepath.Join(examples, "synod.mod")))
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(tmp); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(old) })
	return tmp
}

func overlayJSON(t *testing.T, _ string, replace map[string]string) string {
	t.Helper()
	b, err := json.Marshal(map[string]any{"format": "synod-overlay/1", "replace": replace})
	if err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(t.TempDir(), "overlay.json")
	write(t, p, string(b))
	return p
}

func TestOverlayDiagnosticsKeepPath(t *testing.T) {
	dir := gossipDir(t)
	orig := read(t, "gossip/gossip.synod")
	repl := filepath.Join(t.TempDir(), "1.synod")
	write(t, repl, strings.Replace(orig, "use std/time", "use std/timex", 1))
	ov := overlayJSON(t, t.TempDir(), map[string]string{"gossip/gossip.synod": repl})

	code, out, errs := exec("check", "--json", "--overlay", ov, "gossip")
	if code != 1 || errs != "" {
		t.Fatalf("code=%d stderr=%q out=%.300s", code, errs, out)
	}
	if !strings.Contains(out, `"file":"gossip/gossip.synod"`) || strings.Contains(out, "1.synod") {
		t.Errorf("diagnostic should carry the key's path: %.400s", out)
	}
	if read(t, filepath.Join(dir, "gossip/gossip.synod")) != orig {
		t.Error("disk file changed")
	}
	// Without the overlay the folder is clean.
	if code, out, _ := exec("check", "gossip"); code != 0 {
		t.Errorf("plain check: code=%d out=%s", code, out)
	}
}

func TestOverlayTokensFollowOverlay(t *testing.T) {
	gossipDir(t)
	orig := read(t, "gossip/gossip.synod")
	repl := filepath.Join(t.TempDir(), "1.synod")
	write(t, repl, "# extra comment line\n"+orig)
	// absolute key
	abs, _ := filepath.Abs("gossip/gossip.synod")
	ov := overlayJSON(t, t.TempDir(), map[string]string{abs: repl})

	_, plain, _ := exec("check", "--json", "--tokens", "gossip")
	code, out, errs := exec("check", "--json", "--tokens", "--overlay="+ov, "gossip")
	if errs != "" {
		t.Fatalf("stderr %q", errs)
	}
	_ = code
	var a, b struct {
		Files []struct {
			File   string `json:"file"`
			Tokens []struct{ Line, Col, Len int }
		}
	}
	if err := json.Unmarshal([]byte(plain), &a); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(out), &b); err != nil {
		t.Fatal(err)
	}
	find := func(d any) int {
		fs := a.Files
		if d == "b" {
			fs = b.Files
		}
		for _, f := range fs {
			if f.File == "gossip/gossip.synod" {
				return len(f.Tokens)
			}
		}
		t.Fatalf("file missing in %s", d)
		return 0
	}
	if find("b") != find("a")+1 {
		t.Errorf("overlay tokens %d, plain %d", find("b"), find("a"))
	}
}

func TestOverlaySynodMod(t *testing.T) {
	gossipDir(t)
	repl := filepath.Join(t.TempDir(), "mod")
	write(t, repl, "not a module file\n")
	ov := overlayJSON(t, t.TempDir(), map[string]string{"synod.mod": repl})
	code, out, _ := exec("check", "--json", "--overlay", ov, "gossip")
	if code != 1 || !strings.Contains(out, `"file":"synod.mod"`) {
		t.Errorf("code=%d out=%.300s", code, out)
	}
}

func TestOverlayErrors(t *testing.T) {
	gossipDir(t)
	repl := filepath.Join(t.TempDir(), "1.synod")
	write(t, repl, "x")
	tdir := t.TempDir()
	bad := filepath.Join(tdir, "bad.json")
	write(t, bad, "{")
	badFmt := filepath.Join(tdir, "fmt.json")
	write(t, badFmt, `{"format":"synod-overlay/9","replace":{}}`)
	write(t, "gossip/notes.txt", "x")
	cases := []struct {
		name, file, want string
	}{
		{"missing key", overlayJSON(t, tdir, map[string]string{"gossip/new.synod": repl}), "synod check: overlay: gossip/new.synod: no such file\n"},
		{"not synod", overlayJSON(t, tdir, map[string]string{"gossip/notes.txt": repl}), "synod check: overlay: gossip/notes.txt: no such file\n"},
		{"missing overlay", filepath.Join(tdir, "none.json"), "no such file\n"},
		{"bad json", bad, "not valid JSON"},
		{"bad format", badFmt, "synod-overlay/1"},
	}
	for _, c := range cases {
		code, out, errs := exec("check", "--overlay", c.file, "gossip")
		if code != 2 || out != "" || !strings.HasPrefix(errs, "synod check: overlay: ") || !strings.Contains(errs, strings.TrimSuffix(c.want, "\n")) {
			t.Errorf("%s: code=%d out=%q stderr=%q", c.name, code, out, errs)
		}
		if strings.HasSuffix(c.want, "\n") && c.name == "missing key" || c.name == "not synod" {
			if errs != c.want {
				t.Errorf("%s: stderr=%q want %q", c.name, errs, c.want)
			}
		}
	}
}

func TestOverlayOnlyCheck(t *testing.T) {
	gossipDir(t)
	ov := overlayJSON(t, t.TempDir(), map[string]string{})
	for _, cmd := range []string{"run", "fmt"} {
		code, _, errs := exec(cmd, "--overlay", ov, "gossip")
		if code != 2 || !strings.Contains(errs, "unknown flag") {
			t.Errorf("%s: code=%d stderr=%q", cmd, code, errs)
		}
	}
}
