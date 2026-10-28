package forms

import (
	"os"
	"path/filepath"
	"testing"
)

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

func analyze(t *testing.T, files map[string]string) *Uses {
	t.Helper()
	dir := t.TempDir()
	write(t, dir, files)
	u, diags, err := AnalyzeDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range diags {
		t.Errorf("diagnostic: %s", d.String())
	}
	if u == nil {
		t.FailNow()
	}
	return u
}

func wantKeys(t *testing.T, u *Uses, have, not []string) {
	t.Helper()
	for _, k := range have {
		if !u.Items[k] {
			t.Errorf("missing %s; have %v", k, u.Keys())
		}
	}
	for _, k := range not {
		if u.Items[k] {
			t.Errorf("unexpected %s", k)
		}
	}
}

func TestUsesHello(t *testing.T) {
	u := analyze(t, map[string]string{"hello/hello.synod": `# package main
use std/lesson

# Say it
define main
  lesson.Note "Hello"
`})
	wantKeys(t, u,
		[]string{"form:header", "form:comment", "form:block", "form:use", "form:main", "form:value-text",
			"context:main", "package:std/lesson", "type:text"},
		[]string{"form:member", "form:paren", "form:list", "form:start", "context:node", "type:block", "form:use-as"})
	if !u.Phrases["lesson.Note [t: text]"] {
		t.Errorf("phrases %v", u.PhraseList())
	}
}

func TestUsesNodes(t *testing.T) {
	u := analyze(t, map[string]string{
		"synod.mod": "module example.com/t\nsynod 0.2\n",
		"counter/counter.synod": `# package counter
use std/time

Node has
  set Count to 0

define [n: Node] start
  time.Every 1s
    set n.Count to (n.Count plus 1)
`,
		"counter/demo.synod": `# package main
use example.com/t/counter
use std/sim

define main
  set first to (sim.Spawn counter)
  after 3s
    sim.Crash first
  sim.StopAt 6s
`})
	wantKeys(t, u,
		[]string{"form:has-node", "form:start", "form:set-field", "form:set-local", "form:member", "form:paren",
			"form:plus", "form:after", "form:value-duration", "form:value-number", "context:node", "context:main",
			"type:Node", "type:number", "type:time", "type:package", "package:std/time", "package:std/sim"},
		[]string{"type:block", "form:pattern-block", "form:when-node", "form:if", "form:list"})
	for _, p := range []string{"time.Every [d: time] [b: block]", "sim.Crash [n: node]"} {
		if !u.Phrases[p] {
			t.Errorf("phrase %q not found in %v", p, u.PhraseList())
		}
	}
}

func TestUsesBrokenProgram(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, map[string]string{"p/p.synod": "# package main\ndefine main\n  nope\n"})
	u, diags, err := AnalyzeDir(dir)
	if err != nil || u != nil || len(diags) == 0 {
		t.Fatalf("got %v %v %v", u, diags, err)
	}
}
