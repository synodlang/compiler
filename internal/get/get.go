// Package get implements `synod get` (decision 95): it downloads the modules
// of the build list with git into the shared store, verifies them against
// synod.sum, and copies exactly the build list into the project's vendor/.
// internal/load only reads vendor/; nothing else writes it.
package get

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/synodlang/compiler/internal/load"
	"github.com/synodlang/compiler/internal/source"
)

// Config says what Run does.
type Config struct {
	Dir     string // the folder to start from; the module is found at or above it
	Module  string // with Version: the require line to add or replace
	Version string
	Cache   string // the store; "" means ~/.synod/cache
}

// Status says what happened to one module of the build list.
type Status string

// The three outcomes of a module.
const (
	Downloaded Status = "downloaded"
	FromCache  Status = "from cache"
	UpToDate   Status = "up to date"
)

// Result is one module of the build list.
type Result struct {
	Module, Version string
	Status          Status
}

// String renders the output line of the module.
func (r Result) String() string { return r.Module + "@" + r.Version + " " + string(r.Status) }

// ErrNoModule is returned when no synod.mod is found.
var ErrNoModule = errors.New("no synod.mod here or above")

// ErrNoGit is returned when a download is needed and git is not on PATH.
var ErrNoGit = errors.New("downloading needs git on PATH")

// DefaultCache returns the store folder, ~/.synod/cache.
func DefaultCache() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".synod", "cache"), nil
}

// FindModule returns the folder of the nearest synod.mod at or above start.
func FindModule(start string) (string, error) {
	dir, err := filepath.Abs(start)
	if err != nil {
		return "", err
	}
	for {
		if st, err := os.Stat(filepath.Join(dir, "synod.mod")); err == nil && !st.IsDir() {
			return dir, nil
		}
		up := filepath.Dir(dir)
		if up == dir {
			return "", ErrNoModule
		}
		dir = up
	}
}

// entry is one module version in the store.
type entry struct {
	mod, ver, dir string
	commit, hash  string
	downloaded    bool
	requires      map[string]string
}

type getter struct {
	cache   string
	sum     map[string]sumLine
	entries map[string]*entry
	gitPath string
}

// Run makes vendor/ and synod.sum of the module found from cfg.Dir match its
// build list, and returns the build list sorted by module path. With
// cfg.Module it first sets that require line in synod.mod; the file is
// written once the build list has been resolved and verified, so a failed
// download leaves it as it was.
func Run(cfg Config) ([]Result, error) {
	root, err := FindModule(cfg.Dir)
	if err != nil {
		return nil, err
	}
	modFile := filepath.Join(root, "synod.mod")
	modSrc, err := os.ReadFile(modFile)
	if err != nil {
		return nil, err
	}
	newMod := modSrc
	if cfg.Module != "" {
		newMod = setRequire(modSrc, cfg.Module, cfg.Version)
	}
	main, diags := load.ParseMod("synod.mod", newMod)
	if err := modError("synod.mod", diags); err != nil {
		return nil, err
	}
	cache := cfg.Cache
	if cache == "" {
		if cache, err = DefaultCache(); err != nil {
			return nil, err
		}
	}
	g := &getter{cache: cache, sum: map[string]sumLine{}, entries: map[string]*entry{}}
	sumFile := filepath.Join(root, "synod.sum")
	oldSum, err := os.ReadFile(sumFile)
	if err == nil {
		if g.sum, err = parseSum(oldSum); err != nil {
			return nil, err
		}
	} else if !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}

	// Minimal version selection over the require lines and the synod.mod of
	// each selected dependency, read from the store.
	selected := map[string]string{}
	raise := func(reqs map[string]string) {
		for m, v := range reqs {
			if m == main.Path {
				continue
			}
			if cur, ok := selected[m]; !ok || load.VersionLess(cur, v) {
				selected[m] = v
			}
		}
	}
	raise(main.Requires)
	expanded := map[string]bool{}
	for progress := true; progress; {
		progress = false
		for _, m := range sortedKeys(selected) {
			v := selected[m]
			if expanded[m+"@"+v] {
				continue
			}
			expanded[m+"@"+v] = true
			progress = true
			e, err := g.ensure(m, v)
			if err != nil {
				return nil, err
			}
			raise(e.requires)
		}
	}

	if cfg.Module != "" && string(newMod) != string(modSrc) {
		if err := writeAtomic(modFile, newMod); err != nil {
			return nil, err
		}
	}

	// vendor/ holds copies of exactly the build list.
	vendor := filepath.Join(root, "vendor")
	var results []Result
	var lines []sumLine
	wanted, prefixes := map[string]bool{}, map[string]bool{}
	for _, m := range sortedKeys(selected) {
		e := g.entries[m+"@"+selected[m]]
		target := filepath.Join(vendor, filepath.FromSlash(m+"@"+e.ver))
		status := Downloaded
		if !e.downloaded {
			status = FromCache
			if h, err := treeHash(target, false); err == nil && h == e.hash {
				status = UpToDate
			}
		}
		if status != UpToDate {
			if err := copyEntry(e, target); err != nil {
				return nil, fmt.Errorf("%s@%s: copying to vendor: %v", m, e.ver, err)
			}
		}
		results = append(results, Result{m, e.ver, status})
		lines = append(lines, sumLine{m, e.ver, e.commit, e.hash})
		rel := m + "@" + e.ver
		wanted[rel] = true
		for d := path.Dir(rel); d != "."; d = path.Dir(d) {
			prefixes[d] = true
		}
	}
	if len(results) == 0 {
		if err := forceRemoveAll(vendor); err != nil {
			return nil, err
		}
	} else if err := prune(vendor, "", wanted, prefixes); err != nil {
		return nil, err
	}

	if len(lines) == 0 {
		if err := os.Remove(sumFile); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return nil, err
		}
	} else if text := formatSum(lines); string(text) != string(oldSum) {
		if err := writeAtomic(sumFile, text); err != nil {
			return nil, err
		}
	}
	return results, nil
}

func sortedKeys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// modError turns the diagnostics of a synod.mod into an error. The
// non-canonical one is not an error here: get is what writes the file.
func modError(name string, ds []source.Diagnostic) error {
	for _, d := range ds {
		if d.Code != source.NonCanonical {
			return fmt.Errorf("%s:%d: %s", name, d.Line, d.Message)
		}
	}
	return nil
}

// copyEntry replaces target with a copy of the store entry.
func copyEntry(e *entry, target string) error {
	files, err := moduleFiles(e.dir, false)
	if err != nil {
		return err
	}
	parent := filepath.Dir(target)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return err
	}
	tmp, err := os.MkdirTemp(parent, ".tmp-")
	if err != nil {
		return err
	}
	defer forceRemoveAll(tmp)
	if err := copyFiles(files, tmp); err != nil {
		return err
	}
	if err := os.Chmod(tmp, 0o755); err != nil {
		return err
	}
	if err := forceRemoveAll(target); err != nil {
		return err
	}
	return os.Rename(tmp, target)
}

// prune removes everything under dir (relative name rel inside vendor/)
// that is neither a wanted module folder nor a folder on the way to one.
func prune(dir, rel string, wanted, prefixes map[string]bool) error {
	ents, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, d := range ents {
		r := path.Join(rel, d.Name())
		p := filepath.Join(dir, d.Name())
		switch {
		case wanted[r] && d.IsDir():
		case prefixes[r] && d.IsDir():
			if err := prune(p, r, wanted, prefixes); err != nil {
				return err
			}
		default:
			if err := forceRemoveAll(p); err != nil {
				return err
			}
		}
	}
	return nil
}

// entryDir is where the store keeps m@v.
func (g *getter) entryDir(m, v string) string {
	return filepath.Join(g.cache, filepath.FromSlash(m)+"@"+v)
}

// ensure returns the verified store entry of m@v, downloading it when the
// store lacks it.
func (g *getter) ensure(m, v string) (*entry, error) {
	key := m + "@" + v
	if e := g.entries[key]; e != nil {
		return e, nil
	}
	e := &entry{mod: m, ver: v, dir: g.entryDir(m, v)}
	line, have := g.sum[m]
	have = have && line.Version == v
	_, err := os.Lstat(e.dir)
	switch {
	case err == nil:
		err = g.useStored(e, line, have)
	case errors.Is(err, fs.ErrNotExist):
		err = g.fetch(e, line, have)
	}
	if err != nil {
		return nil, err
	}
	src, err := os.ReadFile(filepath.Join(e.dir, "synod.mod"))
	if err != nil {
		return nil, fmt.Errorf("%s: store entry has no synod.mod", key)
	}
	dep, diags := load.ParseMod(key+"/synod.mod", src)
	if err := modError(key+"/synod.mod", diags); err != nil {
		return nil, err
	}
	e.requires = dep.Requires
	g.entries[key] = e
	return e, nil
}

func commitFile(dir string) string { return dir + ".commit" }

// useStored verifies the existing store entry against synod.sum.
func (g *getter) useStored(e *entry, line sumLine, have bool) error {
	key := e.mod + "@" + e.ver
	h, err := TreeHash(e.dir)
	if err != nil {
		return fmt.Errorf("%s: the store entry is damaged (%v)\n\tdelete %s by hand and run \"synod get\" again", key, err, e.dir)
	}
	e.hash = h
	if b, err := os.ReadFile(commitFile(e.dir)); err == nil {
		e.commit = strings.TrimSpace(string(b))
	}
	if have {
		if line.Hash != h {
			return fmt.Errorf("%s: checksum mismatch: synod.sum has %s, the store has %s\n\tdelete %s by hand and run \"synod get\" again", key, line.Hash, h, e.dir)
		}
		if e.commit != "" && e.commit != line.Commit {
			return fmt.Errorf("%s: checksum mismatch: synod.sum has commit %s, the store has commit %s\n\tdelete %s by hand and run \"synod get\" again", key, line.Commit, e.commit, e.dir)
		}
		e.commit = line.Commit
	} else if !commitRe.MatchString(e.commit) {
		return fmt.Errorf("%s: the store entry has no commit record\n\tdelete %s by hand and run \"synod get\" again", key, e.dir)
	}
	return nil
}

// fetch downloads m@v with git into a temporary folder of the store, checks
// it against synod.sum, and renames it into place.
func (g *getter) fetch(e *entry, line sumLine, have bool) error {
	key := e.mod + "@" + e.ver
	segs := strings.Split(e.mod, "/")
	if len(segs) < 3 {
		return fmt.Errorf("%s: cannot download: a module path must start with host/owner/repo", key)
	}
	if g.gitPath == "" {
		p, err := exec.LookPath("git")
		if err != nil {
			return ErrNoGit
		}
		g.gitPath = p
	}
	if err := os.MkdirAll(g.cache, 0o755); err != nil {
		return err
	}
	tmp, err := os.MkdirTemp(g.cache, ".tmp-")
	if err != nil {
		return err
	}
	defer forceRemoveAll(tmp)

	url := "https://" + strings.Join(segs[:3], "/")
	sub := strings.Join(segs[3:], "/")
	tag := e.ver
	if sub != "" {
		tag = sub + "/" + e.ver
	}
	clone := filepath.Join(tmp, "clone")
	if out, err := g.git("", "clone", "--quiet", "--depth", "1", "--no-tags", "--branch", tag, "--", url, clone); err != nil {
		return fmt.Errorf("%s: git clone of %s at tag %s failed: %s", key, url, tag, out)
	}
	commit, err := g.git(clone, "rev-parse", "HEAD")
	if err != nil || !commitRe.MatchString(commit) {
		return fmt.Errorf("%s: cannot read the commit of tag %s: %s", key, tag, commit)
	}
	modRoot := filepath.Join(clone, filepath.FromSlash(sub))
	src, err := os.ReadFile(filepath.Join(modRoot, "synod.mod"))
	if err != nil {
		return fmt.Errorf("%s: tag %s has no synod.mod in %s", key, tag, orRoot(sub))
	}
	dep, diags := load.ParseMod(key+"/synod.mod", src)
	if err := modError(key+"/synod.mod", diags); err != nil {
		return err
	}
	if dep.Path != e.mod {
		return fmt.Errorf("%s: synod.mod declares module %s", key, dep.Path)
	}
	files, err := moduleFiles(modRoot, true)
	if err != nil {
		return fmt.Errorf("%s: %v", key, err)
	}
	tree := filepath.Join(tmp, "tree")
	if err := copyFiles(files, tree); err != nil {
		return err
	}
	hash, err := TreeHash(tree)
	if err != nil {
		return err
	}
	if have {
		switch {
		case line.Hash != hash:
			return fmt.Errorf("%s: checksum mismatch: synod.sum has %s, the download has %s\n\tnothing was stored", key, line.Hash, hash)
		case line.Commit != commit:
			return fmt.Errorf("%s: checksum mismatch: synod.sum has commit %s, the download has commit %s (the tag moved)\n\tnothing was stored", key, line.Commit, commit)
		}
	}
	if err := os.MkdirAll(filepath.Dir(e.dir), 0o755); err != nil {
		return err
	}
	// The commit is recorded beside the entry, before it appears: the
	// entry itself holds nothing but the module's tree.
	if err := writeAtomic(commitFile(e.dir), []byte(commit+"\n")); err != nil {
		return err
	}
	if err := os.Rename(tree, e.dir); err != nil {
		if _, serr := os.Lstat(e.dir); serr == nil {
			// Another process stored the entry first; use that one.
			return g.useStored(e, line, have)
		}
		return err
	}
	if err := makeReadOnly(e.dir); err != nil {
		return err
	}
	e.hash, e.commit, e.downloaded = hash, commit, true
	return nil
}

func orRoot(sub string) string {
	if sub == "" {
		return "the repository root"
	}
	return sub
}

// git runs git (in dir when it is not empty) without ever prompting, and
// returns its trimmed output; on failure that is the error text.
func (g *getter) git(dir string, args ...string) (string, error) {
	cmd := exec.Command(g.gitPath, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	out, err := cmd.CombinedOutput()
	text := strings.TrimSpace(string(out))
	if i := strings.LastIndex(text, "\n"); i >= 0 {
		text = strings.TrimSpace(text[i+1:])
	}
	if err != nil && text == "" {
		text = err.Error()
	}
	return text, err
}
