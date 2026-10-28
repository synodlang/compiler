package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

type step struct {
	n       int
	program string
	dir     string // code/NN-slug/stepK
}

type chapter struct {
	num   int
	name  string // "01-hello"
	steps []step
	whys  []string // slugs
}

var (
	chapterFile = regexp.MustCompile(`^(\d\d)-([a-z0-9]+(?:-[a-z0-9]+)*)\.md$`)
	stepDir     = regexp.MustCompile(`<!--\s*step\s+(\d+)\s+program=(\S+?)\s*-->`)
	whyDir      = regexp.MustCompile(`<!--\s*why\s+([a-z0-9-]+)\s*-->`)
	details     = regexp.MustCompile(`<details>\s*<summary>Expected</summary>`)
)

// checkChapters checks the chapter files, their headings and directives and
// the code folders they name; it returns the chapters that exist.
func checkChapters(cfg *config, p *problems) []chapter {
	entries, err := os.ReadDir(cfg.tutorial)
	if err != nil {
		p.add(cfg.tutorial, "%v", err)
		return nil
	}
	var out []chapter
	for _, e := range entries {
		m := chapterFile.FindStringSubmatch(e.Name())
		if m == nil {
			continue
		}
		num, _ := strconv.Atoi(m[1])
		out = append(out, chapter{num: num, name: strings.TrimSuffix(e.Name(), ".md")})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].num < out[j].num })
	have := map[int]bool{}
	for _, c := range out {
		have[c.num] = true
	}
	for i := 1; i <= cfg.chapters; i++ {
		if !have[i] {
			p.add(cfg.tutorial, "chapter %02d is missing (chapters are NN-slug.md, 01..%02d)", i, cfg.chapters)
		}
	}
	for _, c := range out {
		if c.num > cfg.chapters {
			p.add(cfg.tutorial, "chapter %02d is beyond the expected %d", c.num, cfg.chapters)
		}
	}
	for i := range out {
		checkChapter(cfg, p, &out[i])
	}
	return out
}

func checkChapter(cfg *config, p *problems, ch *chapter) {
	path := filepath.Join(cfg.tutorial, ch.name+".md")
	b, err := os.ReadFile(path)
	if err != nil {
		p.add(path, "%v", err)
		return
	}
	text := string(b)
	lines := strings.Split(text, "\n")
	// Headings, outside code fences.
	var heads []string
	fence := false
	for _, l := range lines {
		if strings.HasPrefix(l, "```") {
			fence = !fence
		}
		if !fence && strings.HasPrefix(l, "#") {
			heads = append(heads, l)
		}
	}
	title := regexp.MustCompile(fmt.Sprintf(`^# %d\. \S`, ch.num))
	want := []string{"## Goal", "## New in this chapter", "## Steps", "## What to watch", "## Why it looks like this", "## Exercises"}
	if len(heads) == 0 || !title.MatchString(heads[0]) {
		p.add(path, "the first heading must be `# %d. Title`", ch.num)
	}
	pos := 1
	for _, w := range want {
		if pos < len(heads) && heads[pos] == w {
			pos++
			continue
		}
		if w == "## Why it looks like this" {
			continue
		}
		p.add(path, "missing heading %q here (headings: Goal, New in this chapter, Steps, What to watch, [Why it looks like this], Exercises)", w)
		return
	}
	if pos != len(heads) {
		p.add(path, "unexpected heading %q", heads[pos])
	}
	exercises := text[strings.Index(text, "## Exercises"):]
	if n := len(details.FindAllString(exercises, -1)); n < 2 || n > 4 {
		p.add(path, "%d exercises under ## Exercises, want 2 to 4 <details><summary>Expected</summary>", n)
	}

	code := filepath.Join(cfg.tutorial, "code", ch.name)
	for i, m := range stepDir.FindAllStringSubmatch(text, -1) {
		k, _ := strconv.Atoi(m[1])
		if k != i+1 {
			p.add(path, "step directives must count 1, 2, 3 …; found step %d at position %d", k, i+1)
			continue
		}
		s := step{n: k, program: m[2], dir: filepath.Join(code, fmt.Sprintf("step%d", k))}
		if st, err := os.Stat(s.dir); err != nil || !st.IsDir() {
			p.add(path, "step %d: folder %s does not exist", k, s.dir)
			continue
		}
		if st, err := os.Stat(filepath.Join(s.dir, filepath.FromSlash(s.program))); err != nil || !st.IsDir() {
			p.add(path, "step %d: program %q is no folder of %s", k, s.program, s.dir)
			continue
		}
		if _, err := os.Stat(s.dir + ".summary"); err != nil && !cfg.update {
			p.add(path, "step %d: %s.summary does not exist (run doccheck -update)", k, s.dir)
		}
		ch.steps = append(ch.steps, s)
	}
	if len(ch.steps) == 0 {
		p.add(path, "no `<!-- step 1 program=<folder> -->` directive")
	}
	listed := map[string]bool{}
	for _, m := range whyDir.FindAllStringSubmatch(text, -1) {
		slug := m[1]
		if listed[slug] {
			p.add(path, "why %s is listed twice", slug)
		}
		listed[slug] = true
		if st, err := os.Stat(filepath.Join(code, "why-"+slug)); err != nil || !st.IsDir() {
			p.add(path, "why %s: folder %s does not exist", slug, filepath.Join(code, "why-"+slug))
			continue
		}
		ch.whys = append(ch.whys, slug)
	}
	dirs, _ := os.ReadDir(code)
	for _, d := range dirs {
		if s, ok := strings.CutPrefix(d.Name(), "why-"); ok && d.IsDir() && !listed[s] {
			p.add(filepath.Join(code, d.Name()), "no `<!-- why %s -->` directive in %s", s, path)
		}
		if st, ok := strings.CutPrefix(d.Name(), "step"); ok && d.IsDir() {
			if k, err := strconv.Atoi(st); err == nil && k > len(ch.steps) {
				p.add(filepath.Join(code, d.Name()), "no `<!-- step %d … -->` directive in %s", k, path)
			}
		}
	}
}
