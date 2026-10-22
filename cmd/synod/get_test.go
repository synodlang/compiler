package main

import (
	"os"
	osexec "os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// gitSandbox builds a bare repository github.com/o/greet (module
// github.com/o/greet, package greet) tagged v1.0.0 under a temp folder,
// redirects https:// to it, and points HOME at the temp folder.
func gitSandbox(t *testing.T) (root string) {
	t.Helper()
	if _, err := osexec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	root, err := os.MkdirTemp("", "synod-getcmd-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		filepath.WalkDir(root, func(p string, d os.DirEntry, _ error) error {
			if d != nil && d.IsDir() {
				os.Chmod(p, 0o755)
			}
			return nil
		})
		os.RemoveAll(root)
	})
	t.Setenv("HOME", filepath.Join(root, "home"))
	t.Setenv("USERPROFILE", filepath.Join(root, "home"))
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(root, "gitconfig"))
	t.Setenv("GIT_CONFIG_COUNT", "1")
	t.Setenv("GIT_CONFIG_KEY_0", "url.file://"+filepath.ToSlash(filepath.Join(root, "repos"))+"/.insteadOf")
	t.Setenv("GIT_CONFIG_VALUE_0", "https://")
	work := filepath.Join(root, "work")
	os.MkdirAll(filepath.Join(work, "greet"), 0o755)
	os.WriteFile(filepath.Join(work, "synod.mod"), []byte("module github.com/o/greet\nsynod 0.2\n"), 0o644)
	os.WriteFile(filepath.Join(work, "greet", "greet.synod"), []byte("# package greet\n# Double a number\ndefine Twice [x: number]\n  result (x times 2)\n"), 0o644)
	g := func(dir string, args ...string) {
		t.Helper()
		c := osexec.Command("git", append([]string{"-c", "user.name=t", "-c", "user.email=t@e.com", "-c", "commit.gpgsign=false", "-c", "tag.gpgsign=false"}, args...)...)
		c.Dir = dir
		if out, err := c.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	g(work, "init", "-q", "-b", "main")
	g(work, "add", "-A")
	g(work, "commit", "-q", "-m", "v1")
	g(work, "tag", "v1.0.0")
	bare := filepath.Join(root, "repos", "github.com", "o", "greet")
	os.MkdirAll(filepath.Dir(bare), 0o755)
	g(root, "clone", "-q", "--bare", work, bare)
	return root
}

func inDir(t *testing.T, dir string) {
	t.Helper()
	old, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(old) })
}

func TestGetCommand(t *testing.T) {
	needReal(t)
	root := gitSandbox(t)
	proj := filepath.Join(root, "proj")
	if c, _, e := call(runNew, proj); c != 0 {
		t.Fatal(e)
	}
	inDir(t, proj)

	// The import is unknown until synod get.
	os.WriteFile(filepath.Join(proj, "proj", "uses.synod"), []byte("# package proj\nuse github.com/o/greet/greet\n\ndefine Four\n  result (greet.Twice 2)\n"), 0o644)
	if c, o, _ := call(runCheck, "."); c != 1 || !strings.Contains(o, "unknown-import") {
		t.Fatalf("check before get: %d %s", c, o)
	}
	code, out, errs := call(runGet, "github.com/o/greet@v1.0.0")
	if code != 0 || errs != "" || out != "github.com/o/greet@v1.0.0 downloaded\n" {
		t.Fatalf("get: %d %q %q", code, out, errs)
	}
	sum, _ := os.ReadFile("synod.sum")
	f := strings.Fields(string(sum))
	if len(f) != 4 || f[0] != "github.com/o/greet" || f[1] != "v1.0.0" || len(f[2]) != 40 || !strings.HasPrefix(f[3], "h1:") {
		t.Errorf("synod.sum %q", sum)
	}
	mod, _ := os.ReadFile("synod.mod")
	if !strings.HasSuffix(string(mod), "require github.com/o/greet v1.0.0\n") {
		t.Errorf("synod.mod %q", mod)
	}
	if c, o, e := call(runCheck, "."); c != 0 {
		t.Errorf("check after get: %d %s%s", c, o, e)
	}
	// From a subfolder the module is found above; a second run is cached.
	inDir(t, filepath.Join(proj, "proj"))
	code, out, _ = call(runGet)
	if code != 0 || out != "github.com/o/greet@v1.0.0 up to date\n" {
		t.Errorf("second get: %d %q", code, out)
	}
}

func TestGetCommandRefusals(t *testing.T) {
	inDir(t, t.TempDir())
	for _, c := range []struct {
		args []string
		want string
	}{
		{nil, "no synod.mod here or above"},
		{[]string{"github.com/o/greet"}, "synod get: write <module>@vX.Y.Z"},
		{[]string{"github.com/o/greet@1.0"}, "synod get: write <module>@vX.Y.Z"},
		{[]string{"github.com/o/greet@v1"}, "synod get: write <module>@vX.Y.Z"},
		{[]string{"@v1.0.0"}, "synod get: write <module>@vX.Y.Z"},
		{[]string{"a@v1.0.0", "b@v1.0.0"}, "at most one argument"},
		{[]string{"--bogus"}, "unknown flag"},
	} {
		code, _, errs := call(runGet, c.args...)
		if code != 2 || !strings.Contains(errs, c.want) {
			t.Errorf("%v: %d %q", c.args, code, errs)
		}
	}
}
