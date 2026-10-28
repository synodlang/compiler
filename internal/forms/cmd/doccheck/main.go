// Command doccheck checks the documentation of the dsl repository against the
// compiler (decisions 104 and 105, implementation/11-documentation.md): the
// coverage of the tutorial, each step and "why" program, the order in which
// items are introduced, the first and last steps against `synod new` and
// examples/raft, and the runnable examples of the std Library.
//
//	go run ./internal/forms/cmd/doccheck -synod <bin> -tutorial <dsl>/tutorial \
//	    -library <dsl>/std/examples -examples <examples checkout> [-update]
//
// Every problem is printed as "doccheck: <path>: <what>"; the exit code is 1
// when there is one, 2 for bad flags. -update rewrites the golden summaries
// and the generated Library index instead of reporting them as stale.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/synodlang/compiler/internal/doc"
	"github.com/synodlang/compiler/internal/forms"
)

// config says what to check. The zero chapter numbers switch a check off; main
// sets the numbers of the real tutorial.
type config struct {
	synod    string // the synod binary
	tutorial string
	library  string
	examples string
	spec     string // dsl/spec, with decisions.md
	update   bool

	chapters    int // number of chapters
	newChapter  int // step 1 equals `synod new`
	raftChapter int // last step equals examples/raft
	pkgChapter  int // require lines name EXAMPLES_VERSION from here on
	// catalog is what coverage.md must cover; tests restrict it.
	catalog []forms.Item

	// packages restricts the Library check to some std packages; tests only.
	packages []string

	docs *stdDocs
}

func (c *config) stdPackages() []string {
	if c.packages != nil {
		return c.packages
	}
	return doc.StdPackages()
}

type problems struct{ list []string }

func (p *problems) add(path, format string, args ...any) {
	p.list = append(p.list, fmt.Sprintf("%s: %s", path, fmt.Sprintf(format, args...)))
}

func main() {
	var cfg config
	fs := flag.NewFlagSet("doccheck", flag.ExitOnError)
	fs.StringVar(&cfg.synod, "synod", "", "the synod binary")
	fs.StringVar(&cfg.tutorial, "tutorial", "", "dsl/tutorial")
	fs.StringVar(&cfg.library, "library", "", "dsl/std/examples")
	fs.StringVar(&cfg.examples, "examples", "", "checkout of synodlang/examples at EXAMPLES_VERSION")
	fs.StringVar(&cfg.spec, "spec", "", "dsl/spec (default: ../spec next to -tutorial)")
	fs.BoolVar(&cfg.update, "update", false, "rewrite golden summaries and the Library index")
	fs.IntVar(&cfg.chapters, "chapters", 15, "number of chapters")
	fs.Parse(os.Args[1:])
	if cfg.synod == "" || cfg.tutorial == "" || cfg.library == "" || cfg.examples == "" {
		fmt.Fprintln(os.Stderr, "usage: doccheck -synod <bin> -tutorial <dir> -library <dir> -examples <dir> [-spec <dir>] [-chapters n] [-update]")
		os.Exit(2)
	}
	if cfg.spec == "" {
		cfg.spec = filepath.Join(cfg.tutorial, "..", "spec")
	}
	cfg.newChapter, cfg.raftChapter, cfg.pkgChapter = 4, 15, 14
	cfg.catalog = forms.Catalog()
	var p problems
	run(&cfg, &p, true)
	for _, l := range p.list {
		fmt.Fprintln(os.Stderr, "doccheck: "+l)
	}
	if len(p.list) > 0 {
		fmt.Fprintf(os.Stderr, "doccheck: %d problems\n", len(p.list))
		os.Exit(1)
	}
	fmt.Println("doccheck: ok")
}

// run does every check; library says whether to check the Library too.
func run(cfg *config, p *problems, library bool) {
	cfg.docs = &stdDocs{synod: cfg.synod}
	rows := checkCoverage(cfg, p)
	chapters := checkChapters(cfg, p)
	checkSteps(cfg, p, chapters, rows)
	checkWhys(cfg, p, chapters)
	checkNewAndExample(cfg, p, chapters)
	if library {
		checkLibrary(cfg, p)
	}
}
