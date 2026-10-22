package explain

import (
	"bytes"
	"fmt"
	"path"
	"sort"
	"strings"

	"github.com/synodlang/compiler/internal/driver"
	"github.com/synodlang/compiler/internal/load"
	"github.com/synodlang/compiler/internal/pipeline"
	"github.com/synodlang/compiler/internal/render"
	"github.com/synodlang/compiler/internal/scene"
	"github.com/synodlang/compiler/internal/source"
)

// root is the virtual folder the files live under in the in-memory FS.
const root = "/w"

// Doc is an entry together with what the compiler says about its example
// right now (decision 93: the diagnostic is made when asked, never stored).
type Doc struct {
	Entry *Entry
	// Diags are the user diagnostics of `synod check .` for a check
	// example; for a run example, the single failure of the run.
	Diags []source.Diagnostic
	// Failure is the "failed at ..." line of a run example.
	Failure string

	sources map[string]*source.File
	header  string
}

// Render checks (or runs) the example of e in memory.
func Render(e *Entry) (*Doc, error) {
	files := e.ExampleFiles()
	cfg, targets := inMemory(files)
	d := &Doc{Entry: e, sources: map[string]*source.File{}}
	for _, f := range files {
		d.sources[f.Path] = source.NewFile(f.Path, []byte(f.Content))
	}
	if e.Program == "" {
		res := driver.Check(driver.Options{Load: cfg}, targets)
		d.Diags = pipeline.UserDiags(res)
		for k, f := range res.Sources {
			d.sources[k] = f
		}
		if len(d.Diags) == 0 {
			return d, fmt.Errorf("the example of %s gives no diagnostic", e.Code)
		}
		return d, nil
	}
	out := runProgram(cfg, e.Program)
	if out.Err != nil {
		return d, out.Err
	}
	switch {
	case len(out.Diags) > 0:
		d.Diags = out.Diags
	case out.Failure != nil && out.Failure.Kind == "run":
		d.Diags = []source.Diagnostic{out.Failure.Diagnostic}
		d.Failure = out.Failure.Line
	default:
		return d, fmt.Errorf("the example of %s does not fail", e.Code)
	}
	return d, nil
}

// runProgram is `synod run <program>` on the in-memory tree.
func runProgram(cfg load.Config, program string) pipeline.RunOutput {
	return pipeline.Run(pipeline.RunInput{
		Load:   cfg,
		Folder: root + "/" + program, FolderShown: program,
		Pace: 1000, Mode: scene.Ordered,
		TraceDest: "out/trace.jsonl", SceneDest: "out/scene.json", TracePath: "out/trace.jsonl",
	})
}

// CheckFixed checks the fixed module of e and, for a run example, runs it. It
// returns the user diagnostics (as short lines) and the exit code of the run
// (0 for a check example).
func CheckFixed(e *Entry) (diags []string, runCode int) {
	cfg, targets := inMemory(e.FixedFiles())
	res := driver.Check(driver.Options{Load: cfg}, targets)
	for _, d := range pipeline.UserDiags(res) {
		diags = append(diags, d.String())
	}
	if len(diags) > 0 || e.Program == "" {
		return diags, 0
	}
	out := runProgram(cfg, e.Program)
	for _, d := range out.Diags {
		diags = append(diags, d.String())
	}
	if out.Err != nil {
		diags = append(diags, out.Err.Error())
	}
	return diags, out.Code
}

// Diagnostic returns the diagnostic text in the given style: the short
// lines, or the rich form. A run failure is headed by its "failed at" line.
func (d *Doc) Diagnostic(st pipeline.Style) string {
	var b bytes.Buffer
	lookup := func(f string) *source.File { return d.sources[f] }
	if d.Failure == "" {
		if err := pipeline.WriteDiags(&b, d.Diags, st, lookup); err != nil {
			return err.Error() + "\n"
		}
		return b.String()
	}
	if !st.Rich {
		return d.Failure + "\n"
	}
	if err := render.Write(&b, d.Diags[0], render.Options{Color: st.Color, Source: lookup, Header: d.Failure}); err != nil {
		return d.Failure + "\n"
	}
	return b.String()
}

// inMemory builds the loader configuration and the targets of `synod check .`
// for files held in memory.
func inMemory(files []File) (load.Config, []string) {
	m := make(load.MapFS, len(files))
	var keys []string
	for _, f := range files {
		m[root+"/"+f.Path] = f.Content
		keys = append(keys, f.Path)
	}
	sort.Slice(keys, func(i, j int) bool { return walkLess(keys[i], keys[j]) })
	var targets []string
	seen := map[string]bool{}
keys:
	for _, k := range keys {
		segs := strings.Split(k, "/")
		for _, s := range segs[:len(segs)-1] {
			if strings.HasPrefix(s, ".") || strings.HasPrefix(s, "_") || s == "testdata" || s == "vendor" {
				continue keys
			}
		}
		if !strings.HasSuffix(k, ".synod") {
			continue
		}
		dir := root
		if p := path.Dir(k); p != "." {
			dir = root + "/" + p
		}
		if !seen[dir] {
			seen[dir] = true
			targets = append(targets, dir)
		}
	}
	return load.Config{FS: m, Display: display}, targets
}

// display turns a loader path into the path of the entry's file.
func display(p string) string {
	switch {
	case p == root:
		return "."
	case strings.HasPrefix(p, root+"/"):
		return p[len(root)+1:]
	}
	return p
}

// walkLess orders paths as a directory walk visits them.
func walkLess(a, b string) bool {
	as, bs := strings.Split(a, "/"), strings.Split(b, "/")
	for i := 0; i < len(as) && i < len(bs); i++ {
		if as[i] != bs[i] {
			return as[i] < bs[i]
		}
	}
	return len(as) < len(bs)
}
