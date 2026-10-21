package load

import (
	"strings"
	"testing"
)

func TestExamplesHas24Files(t *testing.T) {
	c := readCase(t, "examples")
	out := c.run(t, false)
	n := 0
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, "  file ") && !strings.HasPrefix(line, "  file std/") {
			n++
		}
	}
	if n != 24 {
		t.Errorf("%d example files, want 24", n)
	}
	if !strings.Contains(out, "diagnostics 0\n") {
		t.Errorf("examples have diagnostics:\n%s", out)
	}
	// webapp is a pure program: only its main package exists.
	if strings.Contains(out, "package github.com/synodlang/examples/webapp name=") {
		t.Error("webapp has a library package")
	}
}

func TestMapFS(t *testing.T) {
	m := MapFS{"/a/b/c.synod": "x", "/a/d.synod": "y", "/e": "z"}
	ents, err := m.ReadDir("/a")
	if err != nil || len(ents) != 2 || ents[0].Name() != "b" || !ents[0].IsDir() || ents[1].Name() != "d.synod" || ents[1].IsDir() {
		t.Fatalf("ReadDir(/a) = %v, %v", ents, err)
	}
	if _, err := m.ReadDir("/nope"); err == nil {
		t.Error("ReadDir of a missing directory succeeds")
	}
	if _, err := m.ReadFile("/a"); err == nil {
		t.Error("ReadFile of a directory succeeds")
	}
	if b, err := m.ReadFile("/a//b/c.synod"); err != nil || string(b) != "x" {
		t.Errorf("ReadFile = %q, %v", b, err)
	}
	root, err := m.ReadDir("/")
	if err != nil || len(root) != 2 {
		t.Errorf("ReadDir(/) = %v, %v", root, err)
	}
}

func TestParseMod(t *testing.T) {
	m, ds := parseMod("synod.mod", []byte("module a.com/x\nsynod 0.2\nrequire a.com/b v1.0.0\nrequire a.com/c v2.3.4\n"))
	if len(ds) != 0 || m.Path != "a.com/x" || m.Synod != "0.2" || m.Requires["a.com/c"] != "v2.3.4" {
		t.Errorf("%+v %v", m, ds)
	}
	_, ds = parseMod("synod.mod", nil)
	if len(ds) != 2 {
		t.Errorf("empty file: %v", ds)
	}
}

func TestDefaultCacheDir(t *testing.T) {
	t.Setenv("SYNOD_CACHE", "/x/cache")
	if got := DefaultCacheDir(); got != "/x/cache" {
		t.Errorf("DefaultCacheDir = %q", got)
	}
}

// A nil std rejects every std import without panicking.
func TestNoStd(t *testing.T) {
	fsys := MapFS{"/m/synod.mod": "module e.com/m\nsynod 0.2\n", "/m/a/a.synod": "# package a\nuse std/net\n"}
	_, ds := Load(Config{FS: fsys, CacheDir: "/c"}, []string{"/m/a"})
	if len(ds) != 1 || ds[0].Code != "unknown-import" {
		t.Errorf("%v", ds)
	}
}
