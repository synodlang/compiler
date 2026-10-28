package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/synodlang/compiler/internal/doc"
)

// row is one row of coverage.md.
type row struct {
	key     string
	ref     string
	chapter int
	line    int
}

// stdDocs runs `synod doc --json std/<p>` once per package.
type stdDocs struct {
	synod string
	pkgs  map[string][]docPhrase
	err   error
}

type docPhrase struct {
	Pattern  string `json:"pattern"`
	Exported bool   `json:"exported"`
	Kind     string `json:"kind"`
}

// phrases returns the exported phrases of every std package, by import path.
func (s *stdDocs) phrases() (map[string][]docPhrase, error) {
	if s.pkgs != nil || s.err != nil {
		return s.pkgs, s.err
	}
	dir, err := os.MkdirTemp("", "doccheck-doc")
	if err != nil {
		s.err = err
		return nil, err
	}
	defer os.RemoveAll(dir)
	s.pkgs = map[string][]docPhrase{}
	for _, path := range doc.StdPackages() {
		out, errOut, code := runSynod(s.synod, dir, "doc", "--json", path)
		if code != 0 {
			s.err = fmt.Errorf("synod doc --json %s: exit %d: %s", path, code, strings.TrimSpace(errOut))
			return nil, s.err
		}
		var d struct {
			Packages []struct {
				Phrases []docPhrase `json:"phrases"`
			} `json:"packages"`
		}
		if err := json.Unmarshal([]byte(out), &d); err != nil || len(d.Packages) != 1 {
			s.err = fmt.Errorf("synod doc --json %s: unexpected output", path)
			return nil, s.err
		}
		for _, ph := range d.Packages[0].Phrases {
			if ph.Exported {
				s.pkgs[path] = append(s.pkgs[path], ph)
			}
		}
	}
	return s.pkgs, nil
}

// parseCoverage reads the rows of coverage.md: lines of the table
// `| Item | Shown as | Reference | Chapter |`. An Item is "<kind>:<id>" in
// backticks, or "phrase:<pattern>".
func parseCoverage(path string, p *problems) []row {
	f, err := os.Open(path)
	if err != nil {
		p.add(path, "%v", err)
		return nil
	}
	defer f.Close()
	var rows []row
	sc := bufio.NewScanner(f)
	n, header := 0, false
	for sc.Scan() {
		n++
		l := strings.TrimSpace(sc.Text())
		if !strings.HasPrefix(l, "|") {
			continue
		}
		cells := strings.Split(strings.Trim(l, "|"), "|")
		for i := range cells {
			cells[i] = strings.TrimSpace(cells[i])
		}
		if !header {
			if len(cells) != 4 || cells[0] != "Item" || cells[3] != "Chapter" {
				p.add(path, "line %d: the table header must be | Item | Shown as | Reference | Chapter |", n)
				return rows
			}
			header = true
			continue
		}
		if strings.HasPrefix(cells[0], "---") {
			continue
		}
		if len(cells) != 4 {
			p.add(path, "line %d: a row has 4 cells, found %d", n, len(cells))
			continue
		}
		ch, err := strconv.Atoi(cells[3])
		if err != nil {
			p.add(path, "line %d: chapter %q is not a number", n, cells[3])
			continue
		}
		rows = append(rows, row{key: strings.Trim(cells[0], "`"), ref: cells[2], chapter: ch, line: n})
	}
	if !header {
		p.add(path, "no table | Item | Shown as | Reference | Chapter |")
	}
	return rows
}

// checkCoverage checks coverage.md and returns the chapter of each key.
func checkCoverage(cfg *config, p *problems) map[string]int {
	path := filepath.Join(cfg.tutorial, "coverage.md")
	rows := parseCoverage(path, p)
	known := map[string]bool{}
	for _, it := range cfg.catalog {
		known[it.Key()] = true
	}
	chapters := map[string]int{}
	seen := map[string]int{}
	var patterns map[string]bool
	for _, r := range rows {
		where := fmt.Sprintf("%s:%d", path, r.line)
		if first, dup := seen[r.key]; dup {
			p.add(where, "duplicate row for %s (first at line %d)", r.key, first)
			continue
		}
		seen[r.key] = r.line
		chapters[r.key] = r.chapter
		if r.chapter < 1 || r.chapter > cfg.chapters {
			p.add(where, "%s: chapter %d is outside 1..%d", r.key, r.chapter, cfg.chapters)
		}
		if pat, ok := strings.CutPrefix(r.key, "phrase:"); ok {
			if patterns == nil {
				patterns = map[string]bool{}
				all, err := cfg.docs.phrases()
				if err != nil {
					p.add(where, "%v", err)
				}
				for _, phs := range all {
					for _, ph := range phs {
						patterns[ph.Pattern] = true
					}
				}
			}
			if cfg.docs.err == nil && !patterns[pat] {
				p.add(where, "%q is no exported std pattern (as `synod doc` prints it)", pat)
			}
		} else if !known[r.key] {
			p.add(where, "%s is not an item of the catalog", r.key)
		}
		for _, bad := range badRefs(cfg, r.ref, strings.HasPrefix(r.key, "phrase:")) {
			p.add(where, "%s: Reference %q: %s", r.key, r.ref, bad)
		}
	}
	for _, it := range cfg.catalog {
		if _, ok := seen[it.Key()]; !ok {
			p.add(path, "no row for %s (spec: %s)", it.Key(), it.Spec)
		}
	}
	return chapters
}

var refToken = regexp.MustCompile(`spec/\d+|implementation/\d+|decisions?|\d+(?:\.\d+)?`)

// badRefs returns what is wrong with a Reference cell: "spec/03 §3.3",
// "spec/04 §4.1, 4.4; decision 82", "decisions 25,50". Every section must be
// a heading of the named spec file and every decision a row of decisions.md.
func badRefs(cfg *config, ref string, optional bool) []string {
	if ref == "" || ref == "—" || ref == "-" {
		if optional {
			return nil
		}
		return []string{"empty"}
	}
	var bad []string
	file, inDecisions, found := "", false, false
	for _, tok := range refToken.FindAllString(ref, -1) {
		switch {
		case strings.HasPrefix(tok, "decision"):
			inDecisions = true
		case strings.HasPrefix(tok, "spec/"), strings.HasPrefix(tok, "implementation/"):
			inDecisions = false
			text, ok := readDsl(cfg, tok)
			if !ok {
				bad = append(bad, tok+" does not exist")
			}
			file, found = text, true
			if !ok {
				file = ""
			}
		case inDecisions:
			found = true
			text, _ := os.ReadFile(filepath.Join(cfg.spec, "decisions.md"))
			if !regexp.MustCompile(`(?m)^\| ` + tok + ` \|`).Match(text) {
				bad = append(bad, "decision "+tok+" does not exist")
			}
		case strings.Contains(tok, "."):
			found = true
			if file == "" {
				bad = append(bad, "section "+tok+" has no spec file before it")
			} else if !regexp.MustCompile(`(?m)^#{1,4} ` + regexp.QuoteMeta(tok) + `\b`).MatchString(file) {
				bad = append(bad, "no heading "+tok)
			}
		}
	}
	if !found {
		bad = append(bad, "names no spec section or decision")
	}
	return bad
}

// readDsl reads the file spec/NN-*.md or implementation/NN-*.md of the dsl
// checkout that is the parent of the spec directory.
func readDsl(cfg *config, tok string) (string, bool) {
	dir, num, _ := strings.Cut(tok, "/")
	if dir == "spec" {
		dir = cfg.spec
	} else {
		dir = filepath.Join(cfg.spec, "..", dir)
	}
	m, _ := filepath.Glob(filepath.Join(dir, fmt.Sprintf("%02s-*.md", num)))
	if len(m) != 1 {
		return "", false
	}
	b, err := os.ReadFile(m[0])
	return string(b), err == nil
}
