package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/synodlang/compiler/internal/forms"
)

// runSynod runs the binary in dir and returns stdout, stderr and the exit code
// (-1 when it did not start).
func runSynod(bin, dir string, args ...string) (stdout, stderr string, code int) {
	cmd := exec.Command(bin, args...)
	cmd.Dir = dir
	var o, e bytes.Buffer
	cmd.Stdout, cmd.Stderr = &o, &e
	err := cmd.Run()
	code = 0
	if err != nil {
		code = -1
		if ee, ok := err.(*exec.ExitError); ok {
			code = ee.ExitCode()
		} else {
			e.WriteString(err.Error())
		}
	}
	return o.String(), e.String(), code
}

// copyTo copies the folder src to a new temporary folder, without the files
// named in skip (relative to src), and returns the new folder.
func copyTo(src string, skip ...string) (string, error) {
	tmp, err := os.MkdirTemp("", "doccheck")
	if err != nil {
		return "", err
	}
	err = filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, path)
		for _, s := range skip {
			if rel == s {
				return nil
			}
		}
		dst := filepath.Join(tmp, rel)
		if d.IsDir() {
			return os.MkdirAll(dst, 0o755)
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(dst, b, 0o644)
	})
	if err != nil {
		os.RemoveAll(tmp)
		return "", err
	}
	return tmp, nil
}

func firstLines(s string, n int) string {
	s = strings.TrimSpace(s)
	l := strings.Split(s, "\n")
	if len(l) > n {
		l = append(l[:n], "…")
	}
	return strings.Join(l, "\n    ")
}

// lintFolder runs fmt --check, get (when there is a synod.sum) and check on
// the copy in tmp, which is the module root; target is the folder to check.
func lintFolder(cfg *config, p *problems, where, tmp, target string) bool {
	ok := true
	if out, errOut, code := runSynod(cfg.synod, tmp, "fmt", "--check", target); code != 0 {
		p.add(where, "synod fmt --check %s: exit %d\n    %s", target, code, firstLines(out+errOut, 8))
		ok = false
	}
	if _, err := os.Stat(filepath.Join(tmp, "synod.sum")); err == nil {
		if out, errOut, code := runSynod(cfg.synod, tmp, "get"); code != 0 {
			p.add(where, "synod get: exit %d\n    %s", code, firstLines(out+errOut, 8))
			ok = false
		}
	}
	if out, errOut, code := runSynod(cfg.synod, tmp, "check", target); code != 0 {
		p.add(where, "synod check %s: exit %d\n    %s", target, code, firstLines(out+errOut, 8))
		ok = false
	}
	return ok
}

// checkSteps checks every step: fmt, get, check, the summary golden, and the
// order of introduction (decision 104).
func checkSteps(cfg *config, p *problems, chapters []chapter, rows map[string]int) {
	known := map[string]bool{}
	for _, it := range cfg.catalog {
		known[it.Key()] = true
	}
	for _, ch := range chapters {
		for _, s := range ch.steps {
			where := s.dir
			tmp, err := copyTo(s.dir)
			if err != nil {
				p.add(where, "%v", err)
				continue
			}
			if lintFolder(cfg, p, where, tmp, ".") {
				checkSummary(cfg, p, s, tmp)
				checkOrder(p, ch, s, tmp, known, rows)
			}
			os.RemoveAll(tmp)
		}
	}
}

func checkSummary(cfg *config, p *problems, s step, tmp string) {
	golden := s.dir + ".summary"
	out, errOut, code := runSynod(cfg.synod, tmp, "run", s.program, "--summary", "--style", "short")
	if code != 0 {
		p.add(s.dir, "synod run %s --summary --style short: exit %d\n    %s", s.program, code, firstLines(out+errOut, 8))
		return
	}
	want, err := os.ReadFile(golden)
	switch {
	case err == nil && string(want) == out:
	case cfg.update:
		if err := os.WriteFile(golden, []byte(out), 0o644); err != nil {
			p.add(golden, "%v", err)
		}
	case err != nil:
		p.add(golden, "missing (run doccheck -update)")
	default:
		p.add(golden, "stale: `synod run %s --summary --style short` now says\n    %s\n  (run doccheck -update)", s.program, firstLines(out, 6))
	}
}

// checkOrder fails when step s of chapter ch uses an item that a later
// chapter introduces.
func checkOrder(p *problems, ch chapter, s step, tmp string, known map[string]bool, rows map[string]int) {
	u, diags, err := forms.AnalyzeDir(tmp)
	if err != nil || u == nil {
		p.add(s.dir, "usage analysis failed: %v %d diagnostics", err, len(diags))
		return
	}
	for _, k := range u.Keys() {
		if known[k] {
			if c, ok := rows[k]; ok && c > ch.num {
				p.add(s.dir, "chapter %d uses %s, which chapter %d introduces", ch.num, k, c)
			}
		}
	}
	for _, pat := range u.PhraseList() {
		if c, ok := rows["phrase:"+pat]; ok && c > ch.num {
			p.add(s.dir, "chapter %d uses the phrase %q, which chapter %d introduces", ch.num, pat, c)
		}
	}
}

// checkWhys checks that each why program fails with the code in expect.
func checkWhys(cfg *config, p *problems, chapters []chapter) {
	for _, ch := range chapters {
		for _, slug := range ch.whys {
			dir := filepath.Join(cfg.tutorial, "code", ch.name, "why-"+slug)
			code, err := readExpect(dir)
			if err != nil {
				p.add(dir, "%v", err)
				continue
			}
			tmp, err := copyTo(dir, "expect")
			if err != nil {
				p.add(dir, "%v", err)
				continue
			}
			if msg := failsWith(cfg, tmp, code); msg != "" {
				p.add(dir, "%s", msg)
			}
			os.RemoveAll(tmp)
		}
	}
}

// readExpect reads the file expect of dir: one line, a diagnostic code.
func readExpect(dir string) (string, error) {
	b, err := os.ReadFile(filepath.Join(dir, "expect"))
	if err != nil {
		return "", fmt.Errorf("expect: %v", err)
	}
	code := strings.TrimSpace(string(b))
	if !regexp.MustCompile(`^[a-z]+(-[a-z]+)*$`).MatchString(code) {
		return "", fmt.Errorf("expect must hold one diagnostic code, found %q", code)
	}
	return code, nil
}

var runCode = regexp.MustCompile(`error\[([a-z-]+)\]`)

// failsWith runs synod check on the folder tmp and, when that passes, synod
// run on each folder with a .synod file; it returns "" when the check reports
// code or a run fails with it, else what happened.
func failsWith(cfg *config, tmp, code string) string {
	out, errOut, exit := runSynod(cfg.synod, tmp, "check", "--json", ".")
	if exit == 0 {
		for _, d := range synodFolders(tmp) {
			o, e, x := runSynod(cfg.synod, tmp, "run", d, "--summary", "--style", "short")
			if x != 0 && strings.Contains(o+e, "error["+code+"]") {
				return ""
			}
		}
		return fmt.Sprintf("expected %s, but synod check passes and no run fails with it", code)
	}
	var res struct {
		Diagnostics []struct {
			Code string `json:"code"`
		} `json:"diagnostics"`
	}
	if err := json.Unmarshal([]byte(out), &res); err != nil {
		return fmt.Sprintf("synod check --json: exit %d: %s", exit, firstLines(out+errOut, 4))
	}
	var codes []string
	for _, d := range res.Diagnostics {
		if d.Code == code {
			return ""
		}
		codes = append(codes, d.Code)
	}
	return fmt.Sprintf("expected %s, synod check reports %v", code, codes)
}

// synodFolders lists the folders of dir (slash paths relative to it) that hold
// a .synod file, without vendor.
func synodFolders(dir string) []string {
	var out []string
	filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() && d.Name() == "vendor" {
			return filepath.SkipDir
		}
		if !d.IsDir() && strings.HasSuffix(d.Name(), ".synod") {
			rel, _ := filepath.Rel(dir, filepath.Dir(path))
			rel = filepath.ToSlash(rel)
			if len(out) == 0 || out[len(out)-1] != rel {
				out = append(out, rel)
			}
		}
		return nil
	})
	return out
}
