package main

import (
	"bufio"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

const examplesModule = "github.com/synodlang/examples"

var versionKeys = []string{"SYNOD_VERSION", "SYNOD_LINUX_AMD64_SHA256", "COMPILER_COMMIT", "EXAMPLES_VERSION", "EXAMPLES_COMMIT"}

// readVersions reads versions.env: KEY=VALUE lines, # comments.
func readVersions(path string, p *problems) map[string]string {
	f, err := os.Open(path)
	if err != nil {
		p.add(path, "%v", err)
		return nil
	}
	defer f.Close()
	env := map[string]string{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		l := strings.TrimSpace(sc.Text())
		if l == "" || strings.HasPrefix(l, "#") {
			continue
		}
		k, v, ok := strings.Cut(l, "=")
		if !ok {
			p.add(path, "line %q is not KEY=VALUE", l)
			continue
		}
		env[strings.TrimSpace(k)] = strings.Trim(strings.TrimSpace(v), `"'`)
	}
	for _, k := range versionKeys {
		if env[k] == "" {
			p.add(path, "%s is missing", k)
		}
	}
	return env
}

// readTree returns the files below dir by slash path, with their contents.
func readTree(dir string) (map[string]string, error) {
	out := map[string]string{}
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(dir, path)
		out[filepath.ToSlash(rel)] = string(b)
		return nil
	})
	return out, err
}

// diffTrees reports files that are missing, extra or different.
func diffTrees(p *problems, where string, want, got map[string]string, wantName string) {
	for name, w := range want {
		g, ok := got[name]
		switch {
		case !ok:
			p.add(where, "%s is missing (%s has it)", name, wantName)
		case g != w:
			p.add(filepath.Join(where, name), "differs from %s", wantName)
		}
	}
	for name := range got {
		if _, ok := want[name]; !ok {
			p.add(filepath.Join(where, name), "is not in %s", wantName)
		}
	}
}

// checkNewAndExample checks the pins and the two steps that are copies:
// step 1 of the newChapter against `synod new`, the last step of the
// raftChapter against the examples, and the require lines from pkgChapter.
func checkNewAndExample(cfg *config, p *problems, chapters []chapter) {
	env := readVersions(filepath.Join(cfg.tutorial, "versions.env"), p)
	find := func(n int) *chapter {
		for i := range chapters {
			if chapters[i].num == n && len(chapters[i].steps) > 0 {
				return &chapters[i]
			}
		}
		return nil
	}
	if ch := find(cfg.newChapter); ch != nil && cfg.newChapter > 0 {
		tmp, err := os.MkdirTemp("", "doccheck-new")
		if err != nil {
			p.add(cfg.tutorial, "%v", err)
			return
		}
		defer os.RemoveAll(tmp)
		if out, errOut, code := runSynod(cfg.synod, tmp, "new", "tutorial", "--module", "example.com/tutorial"); code != 0 {
			p.add(cfg.synod, "synod new: exit %d: %s", code, firstLines(out+errOut, 4))
		} else {
			want, err1 := readTree(filepath.Join(tmp, "tutorial"))
			got, err2 := readTree(ch.steps[0].dir)
			if err1 != nil || err2 != nil {
				p.add(ch.steps[0].dir, "%v %v", err1, err2)
			} else {
				diffTrees(p, ch.steps[0].dir, want, got, "`synod new tutorial --module example.com/tutorial`")
			}
		}
	}
	if env == nil {
		return
	}
	if ch := find(cfg.raftChapter); ch != nil && cfg.raftChapter > 0 {
		checkExampleCopy(cfg, p, ch.steps[len(ch.steps)-1].dir, env)
	}
	if cfg.pkgChapter > 0 {
		req := regexp.MustCompile(`(?m)^require ` + regexp.QuoteMeta(examplesModule) + ` (\S+)\s*$`)
		for _, ch := range chapters {
			if ch.num < cfg.pkgChapter {
				continue
			}
			for _, s := range ch.steps {
				b, _ := os.ReadFile(filepath.Join(s.dir, "synod.mod"))
				for _, m := range req.FindAllStringSubmatch(string(b), -1) {
					if m[1] != env["EXAMPLES_VERSION"] {
						p.add(filepath.Join(s.dir, "synod.mod"), "requires %s %s, versions.env pins EXAMPLES_VERSION %s", examplesModule, m[1], env["EXAMPLES_VERSION"])
					}
				}
			}
		}
	}
}

// checkExampleCopy compares the last step of the raft chapter with
// examples/raft and examples/raftlook at the pinned version: the files are
// equal once the examples module path is replaced by example.com/tutorial/.
func checkExampleCopy(cfg *config, p *problems, last string, env map[string]string) {
	if _, err := os.Stat(filepath.Join(cfg.examples, ".git")); err == nil {
		out, err := exec.Command("git", "-C", cfg.examples, "rev-parse", "HEAD").Output()
		if err != nil || strings.TrimSpace(string(out)) != env["EXAMPLES_COMMIT"] {
			p.add(cfg.examples, "is not at EXAMPLES_COMMIT %s (HEAD %s)", env["EXAMPLES_COMMIT"], strings.TrimSpace(string(out)))
		}
	}
	for _, pkg := range []string{"raft", "raftlook"} {
		want, err := readTree(filepath.Join(cfg.examples, pkg))
		if err != nil {
			p.add(cfg.examples, "%v", err)
			continue
		}
		for name, text := range want {
			want[name] = strings.ReplaceAll(text, examplesModule+"/", "example.com/tutorial/")
		}
		got, _ := readTree(filepath.Join(last, pkg))
		diffTrees(p, filepath.Join(last, pkg), want, got, "examples/"+pkg+" at "+env["EXAMPLES_VERSION"])
	}
}
