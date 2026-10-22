package get

import (
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"testing"
)

// env is a sandbox: HOME, the store and bare repositories under a temp
// folder, and git redirected from https:// to them. Nothing touches the
// network or the real ~/.synod.
type env struct {
	t     *testing.T
	root  string
	repos string
	cache string
}

func newEnv(t *testing.T) *env {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	root, err := os.MkdirTemp("", "synod-get-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { forceRemoveAll(root) })
	e := &env{t: t, root: root, repos: filepath.Join(root, "repos"), cache: filepath.Join(root, "home", ".synod", "cache")}
	t.Setenv("HOME", filepath.Join(root, "home"))
	t.Setenv("USERPROFILE", filepath.Join(root, "home"))
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(root, "gitconfig"))
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	e.redirect(e.repos)
	return e
}

// redirect points https:// at the bare repositories under dir.
func (e *env) redirect(dir string) {
	u := filepath.ToSlash(dir)
	if !strings.HasPrefix(u, "/") {
		u = "/" + u
	}
	e.t.Setenv("GIT_CONFIG_COUNT", "1")
	e.t.Setenv("GIT_CONFIG_KEY_0", "url.file://"+u+"/.insteadOf")
	e.t.Setenv("GIT_CONFIG_VALUE_0", "https://")
}

func (e *env) git(dir string, args ...string) string {
	e.t.Helper()
	cmd := exec.Command("git", append([]string{"-c", "user.name=t", "-c", "user.email=t@example.com", "-c", "commit.gpgsign=false", "-c", "tag.gpgsign=false"}, args...)...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		e.t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func write(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for name, text := range files {
		p := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// publish makes (or extends) the repository host/owner/repo with files at
// tag, and returns the commit of the tag.
func (e *env) publish(repo string, files map[string]string, tag string) string {
	e.t.Helper()
	work := filepath.Join(e.root, "work", filepath.FromSlash(repo))
	bare := filepath.Join(e.repos, filepath.FromSlash(repo))
	if _, err := os.Stat(work); err != nil {
		os.MkdirAll(work, 0o755)
		e.git(work, "init", "-q", "-b", "main")
	}
	write(e.t, work, files)
	e.git(work, "add", "-A")
	e.git(work, "commit", "-q", "--allow-empty", "-m", tag)
	e.git(work, "tag", "-f", tag)
	os.RemoveAll(bare)
	os.MkdirAll(filepath.Dir(bare), 0o755)
	e.git(e.root, "clone", "-q", "--bare", work, bare)
	return e.git(work, "rev-parse", tag+"^{commit}")
}

// project makes a main module with the given require lines.
func (e *env) project(requires ...string) string {
	dir := filepath.Join(e.root, "proj")
	os.RemoveAll(dir)
	text := "module example.com/proj\nsynod 0.2\n"
	for _, r := range requires {
		text += "require " + r + "\n"
	}
	write(e.t, dir, map[string]string{"synod.mod": text})
	return dir
}

func modFiles(module string, requires ...string) map[string]string {
	text := "module " + module + "\nsynod 0.2\n"
	for _, r := range requires {
		text += "require " + r + "\n"
	}
	return map[string]string{"synod.mod": text, "pkg/pkg.synod": "# package pkg\n"}
}

// refHash is an independent implementation of h1 for the tests.
func refHash(t *testing.T, root string, skip func(rel string) bool) string {
	t.Helper()
	var rels []string
	filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
		rel, _ := filepath.Rel(root, p)
		rel = filepath.ToSlash(rel)
		if info.IsDir() || skip(rel) {
			return nil
		}
		rels = append(rels, rel)
		return nil
	})
	sort.Strings(rels)
	var b strings.Builder
	for _, r := range rels {
		data, _ := os.ReadFile(filepath.Join(root, filepath.FromSlash(r)))
		fmt.Fprintf(&b, "%x  %s\n", sha256.Sum256(data), r)
	}
	s := sha256.Sum256([]byte(b.String()))
	return "h1:" + base64.StdEncoding.EncodeToString(s[:])
}

func noGit(rel string) bool { return strings.HasPrefix(rel, ".git/") || rel == ".git" }

func run(t *testing.T, dir, mod, ver string) ([]Result, error) {
	t.Helper()
	return Run(Config{Dir: dir, Module: mod, Version: ver})
}

func statuses(rs []Result) string {
	var s []string
	for _, r := range rs {
		s = append(s, r.String())
	}
	return strings.Join(s, "\n")
}

func TestGetBasic(t *testing.T) {
	e := newEnv(t)
	const m = "github.com/o/raft"
	commit := e.publish("github.com/o/raft", modFiles(m), "v1.0.0")
	dir := e.project("github.com/a/zzz v1.0.0")
	e.publish("github.com/a/zzz", modFiles("github.com/a/zzz"), "v1.0.0")

	rs, err := run(t, dir, m, "v1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	if got := statuses(rs); got != "github.com/a/zzz@v1.0.0 downloaded\ngithub.com/o/raft@v1.0.0 downloaded" {
		t.Errorf("results:\n%s", got)
	}
	mod, _ := os.ReadFile(filepath.Join(dir, "synod.mod"))
	if want := "module example.com/proj\nsynod 0.2\nrequire github.com/a/zzz v1.0.0\nrequire github.com/o/raft v1.0.0\n"; string(mod) != want {
		t.Errorf("synod.mod:\n%s", mod)
	}

	entry := filepath.Join(e.cache, "github.com", "o", "raft@v1.0.0")
	if runtime.GOOS != "windows" {
		for p, want := range map[string]os.FileMode{entry: 0o555, filepath.Join(entry, "pkg"): 0o555, filepath.Join(entry, "synod.mod"): 0o444, filepath.Join(entry, "pkg", "pkg.synod"): 0o444} {
			if st, err := os.Stat(p); err != nil || st.Mode().Perm() != want {
				t.Errorf("%s: %v %v", p, st, err)
			}
		}
	}
	vend := filepath.Join(dir, "vendor", "github.com", "o", "raft@v1.0.0")
	if a, b := refHash(t, vend, noGit), refHash(t, entry, noGit); a != b {
		t.Errorf("vendor differs from store")
	}
	sum, _ := os.ReadFile(filepath.Join(dir, "synod.sum"))
	h := refHash(t, entry, noGit)
	wantSum := "github.com/a/zzz v1.0.0 " + e.git(filepath.Join(e.root, "work", "github.com", "a", "zzz"), "rev-parse", "HEAD") + " " + refHash(t, filepath.Join(e.root, "work", "github.com", "a", "zzz"), noGit) + "\n" +
		"github.com/o/raft v1.0.0 " + commit + " " + h + "\n"
	if string(sum) != wantSum {
		t.Errorf("synod.sum:\n%s\nwant:\n%s", sum, wantSum)
	}
	// The vendor copy is a copy: editing it never reaches the store.
	vf := filepath.Join(vend, "pkg", "pkg.synod")
	if err := os.WriteFile(vf, []byte("# edited\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(filepath.Join(entry, "pkg", "pkg.synod")); string(b) != "# package pkg\n" {
		t.Errorf("store changed: %q", b)
	}

	// Offline: git redirected to nothing, and not even on PATH.
	e.redirect(filepath.Join(e.root, "missing"))
	t.Setenv("PATH", "")
	os.MkdirAll(filepath.Join(dir, "vendor", "junk", "x"), 0o755)
	os.WriteFile(filepath.Join(dir, "vendor", "stray.txt"), nil, 0o644)
	rs, err = run(t, dir, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if got := statuses(rs); got != "github.com/a/zzz@v1.0.0 up to date\ngithub.com/o/raft@v1.0.0 from cache" {
		t.Errorf("offline results:\n%s", got)
	}
	if b, _ := os.ReadFile(vf); string(b) != "# package pkg\n" {
		t.Errorf("edited vendor file not restored: %q", b)
	}
	for _, p := range []string{"junk", "stray.txt"} {
		if _, err := os.Lstat(filepath.Join(dir, "vendor", p)); err == nil {
			t.Errorf("vendor/%s not pruned", p)
		}
	}
	rs, err = run(t, dir, "", "")
	if err != nil || !strings.Contains(statuses(rs), "raft@v1.0.0 up to date") {
		t.Errorf("third run: %v %v", rs, err)
	}
	if s2, _ := os.ReadFile(filepath.Join(dir, "synod.sum")); string(s2) != wantSum {
		t.Errorf("sum changed")
	}
}

func TestGetTransitiveMVS(t *testing.T) {
	e := newEnv(t)
	e.publish("github.com/o/b", modFiles("github.com/o/b"), "v1.0.0")
	e.publish("github.com/o/b", map[string]string{"extra": "x"}, "v1.1.0")
	e.publish("github.com/o/a", modFiles("github.com/o/a", "github.com/o/b v1.1.0"), "v1.0.0")
	dir := e.project("github.com/o/a v1.0.0", "github.com/o/b v1.0.0")
	rs, err := run(t, dir, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if got := statuses(rs); got != "github.com/o/a@v1.0.0 downloaded\ngithub.com/o/b@v1.1.0 downloaded" {
		t.Errorf("results:\n%s", got)
	}
	if _, err := os.Stat(filepath.Join(dir, "vendor", "github.com", "o", "b@v1.0.0")); err == nil {
		t.Error("superseded version vendored")
	}
	sum, _ := os.ReadFile(filepath.Join(dir, "synod.sum"))
	if strings.Count(string(sum), "\n") != 2 || strings.Contains(string(sum), "v1.0.0 ") && strings.Contains(string(sum), "o/b v1.0.0") {
		t.Errorf("sum:\n%s", sum)
	}
}

func TestGetSubfolderModule(t *testing.T) {
	e := newEnv(t)
	const m = "github.com/o/mono/sub/deep"
	files := map[string]string{"README": "r", "other/x": "y"}
	for k, v := range modFiles(m) {
		files["sub/deep/"+k] = v
	}
	// A nested module inside the module is left out; so is .git.
	files["sub/deep/nested/synod.mod"] = "module " + m + "/nested\nsynod 0.2\n"
	files["sub/deep/nested/n.synod"] = "# package n\n"
	e.publish("github.com/o/mono", files, "sub/deep/v1.0.0")
	dir := e.project()
	if _, err := run(t, dir, m, "v1.0.0"); err != nil {
		t.Fatal(err)
	}
	vend := filepath.Join(dir, "vendor", filepath.FromSlash(m)+"@v1.0.0")
	if _, err := os.Stat(filepath.Join(vend, "pkg", "pkg.synod")); err != nil {
		t.Error(err)
	}
	for _, p := range []string{"nested", ".git", "README", "other"} {
		if _, err := os.Lstat(filepath.Join(vend, p)); err == nil {
			t.Errorf("%s was copied", p)
		}
	}
}

func TestGetTamperedStore(t *testing.T) {
	e := newEnv(t)
	const m = "github.com/o/raft"
	e.publish("github.com/o/raft", modFiles(m), "v1.0.0")
	dir := e.project("github.com/o/raft v1.0.0")
	if _, err := run(t, dir, "", ""); err != nil {
		t.Fatal(err)
	}
	f := filepath.Join(e.cache, "github.com", "o", "raft@v1.0.0", "pkg", "pkg.synod")
	if err := os.Chmod(f, 0o644); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(f, []byte("# package pkg\n# evil\n"), 0o644)
	_, err := run(t, dir, "", "")
	if err == nil || !strings.Contains(err.Error(), "github.com/o/raft@v1.0.0: checksum mismatch: synod.sum has h1:") || !strings.Contains(err.Error(), "the store has h1:") {
		t.Fatalf("err %v", err)
	}
	if b, _ := os.ReadFile(f); string(b) != "# package pkg\n# evil\n" {
		t.Errorf("store entry was changed: %q", b)
	}
}

func TestGetMovedTag(t *testing.T) {
	e := newEnv(t)
	const m = "github.com/o/raft"
	e.publish("github.com/o/raft", modFiles(m), "v1.0.0")
	dir := e.project("github.com/o/raft v1.0.0")
	if _, err := run(t, dir, "", ""); err != nil {
		t.Fatal(err)
	}
	// A new machine: no store entry, no vendor. The tag has moved.
	e.publish("github.com/o/raft", map[string]string{"pkg/pkg.synod": "# package pkg\n# new\n"}, "v1.0.0")
	if err := forceRemoveAll(e.cache); err != nil {
		t.Fatal(err)
	}
	os.RemoveAll(filepath.Join(dir, "vendor"))
	_, err := run(t, dir, "", "")
	if err == nil || !strings.Contains(err.Error(), "checksum mismatch: synod.sum has") {
		t.Fatalf("err %v", err)
	}
	if _, err := os.Stat(filepath.Join(e.cache, "github.com", "o", "raft@v1.0.0")); err == nil {
		t.Error("mismatching download was stored")
	}
	// A moved tag with the same tree is caught by its commit.
	e.publish("github.com/o/raft", map[string]string{"pkg/pkg.synod": "# package pkg\n"}, "v1.0.0")
	if _, err := run(t, dir, "", ""); err == nil || !strings.Contains(err.Error(), "synod.sum has commit") {
		t.Fatalf("commit mismatch: %v", err)
	}
}

func TestGetErrors(t *testing.T) {
	e := newEnv(t)
	e.publish("github.com/o/wrong", modFiles("github.com/o/other"), "v1.0.0")
	linkFiles := modFiles("github.com/o/link")
	e.publish("github.com/o/link", linkFiles, "v1.0.0")
	work := filepath.Join(e.root, "work", "github.com", "o", "link")
	if err := os.Symlink("pkg", filepath.Join(work, "ln")); err != nil {
		t.Skip("no symlinks")
	}
	e.publish("github.com/o/link", map[string]string{}, "v1.0.0")
	e.publish("github.com/o/ok", modFiles("github.com/o/ok"), "v1.0.0")

	dir := e.project("github.com/o/wrong v1.0.0")
	if _, err := run(t, dir, "", ""); err == nil || !strings.Contains(err.Error(), "declares module github.com/o/other") {
		t.Errorf("wrong module: %v", err)
	}
	dir = e.project("github.com/o/link v1.0.0")
	if _, err := run(t, dir, "", ""); err == nil || !strings.Contains(err.Error(), "symbolic link") {
		t.Errorf("symlink: %v", err)
	}
	dir = e.project("github.com/o/ok v9.9.9")
	if _, err := run(t, dir, "", ""); err == nil || !strings.Contains(err.Error(), "git clone") {
		t.Errorf("missing tag: %v", err)
	}
	// A failed add leaves synod.mod alone.
	if _, err := run(t, dir, "github.com/o/ok", "v8.8.8"); err == nil {
		t.Error("expected error")
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "synod.mod")); !strings.Contains(string(b), "v9.9.9") || strings.Contains(string(b), "v8.8.8") {
		t.Errorf("synod.mod changed: %s", b)
	}
	dir = e.project("github.com/o/ok v1.0.0")
	t.Setenv("PATH", "")
	if _, err := run(t, dir, "", ""); !errors.Is(err, ErrNoGit) {
		t.Errorf("no git: %v", err)
	}
	if _, err := Run(Config{Dir: t.TempDir()}); !errors.Is(err, ErrNoModule) {
		t.Errorf("no module: %v", err)
	}
	dir = e.project("short v1.0.0")
	if _, err := run(t, dir, "", ""); err == nil || !strings.Contains(err.Error(), "host/owner/repo") {
		t.Errorf("short path: %v", err)
	}
}

func TestGetEmptyBuildList(t *testing.T) {
	e := newEnv(t)
	e.publish("github.com/o/ok", modFiles("github.com/o/ok"), "v1.0.0")
	dir := e.project("github.com/o/ok v1.0.0")
	if _, err := run(t, dir, "", ""); err != nil {
		t.Fatal(err)
	}
	write(t, dir, map[string]string{"synod.mod": "module example.com/proj\nsynod 0.2\n"})
	rs, err := run(t, dir, "", "")
	if err != nil || len(rs) != 0 {
		t.Fatal(rs, err)
	}
	for _, p := range []string{"synod.sum", "vendor"} {
		if _, err := os.Stat(filepath.Join(dir, p)); err == nil {
			t.Errorf("%s not removed", p)
		}
	}
}

func TestGetConcurrent(t *testing.T) {
	e := newEnv(t)
	e.publish("github.com/o/ok", modFiles("github.com/o/ok"), "v1.0.0")
	var dirs []string
	for i := 0; i < 4; i++ {
		d := filepath.Join(e.root, fmt.Sprint("p", i))
		write(t, d, map[string]string{"synod.mod": "module example.com/p\nsynod 0.2\nrequire github.com/o/ok v1.0.0\n"})
		dirs = append(dirs, d)
	}
	var wg sync.WaitGroup
	errs := make([]error, len(dirs))
	for i, d := range dirs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, errs[i] = Run(Config{Dir: d})
		}()
	}
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			t.Error(err)
		}
	}
	ents, _ := os.ReadDir(filepath.Join(e.cache, "github.com", "o"))
	var names []string
	for _, x := range ents {
		names = append(names, x.Name())
	}
	if got := strings.Join(names, " "); got != "ok@v1.0.0 ok@v1.0.0.commit" {
		t.Errorf("store: %s", got)
	}
	left, _ := filepath.Glob(filepath.Join(e.cache, ".tmp-*"))
	if len(left) != 0 {
		t.Errorf("temp folders left: %v", left)
	}
}

func TestSetRequire(t *testing.T) {
	in := "# my module\nmodule m\nsynod 0.2\nrequire b/b/b v1.0.0\nrequire a/a/a v1.0.0\n\n"
	got := string(setRequire([]byte(in), "b/b/b", "v1.2.0"))
	want := "# my module\nmodule m\nsynod 0.2\nrequire a/a/a v1.0.0\nrequire b/b/b v1.2.0\n"
	if got != want {
		t.Errorf("got %q", got)
	}
}
