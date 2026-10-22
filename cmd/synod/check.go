package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/synodlang/compiler/internal/driver"
	"github.com/synodlang/compiler/internal/load"
	"github.com/synodlang/compiler/internal/pipeline"
)

func runCheck(args []string, stdout, stderr io.Writer) int {
	paths, flags, values, err := parseFlags(args, []string{"json", "tokens"}, []string{"style", "color", "overlay"})
	if err != nil {
		fmt.Fprintf(stderr, "synod check: %v\n", err)
		return exitUsage
	}
	sf, err := styleOf(values)
	if err != nil {
		fmt.Fprintf(stderr, "synod check: %v\n", err)
		return exitUsage
	}
	if flags["tokens"] && !flags["json"] {
		fmt.Fprintln(stderr, "synod check: --tokens needs --json")
		return exitUsage
	}
	_, targets, err := collect(paths)
	if err != nil {
		fmt.Fprintf(stderr, "synod check: %v\n", err)
		return exitUsage
	}
	opts := driver.Options{Tokens: flags["tokens"]}
	if file, ok := values["overlay"]; ok {
		o, err := readOverlay(file)
		if err != nil {
			fmt.Fprintf(stderr, "synod check: overlay: %v\n", err)
			return exitUsage
		}
		opts.Load.FS = o
	}
	res := driver.Check(opts, targets)
	diags := pipeline.UserDiags(res)
	if flags["json"] {
		if werr := pipeline.WriteCheckJSON(stdout, diags, res, flags["tokens"]); werr != nil {
			fmt.Fprintf(stderr, "synod check: %v\n", werr)
			return exitUsage
		}
	} else {
		if werr := writeDiags(stdout, diags, sf, &sources{known: res.Sources}); werr != nil {
			fmt.Fprintf(stderr, "synod check: %v\n", werr)
			return exitUsage
		}
	}
	if len(diags) > 0 {
		return exitDiags
	}
	return exitOK
}

// readOverlay reads overlay.json (04-formats.md) and returns the file system
// that serves the replacement contents.
func readOverlay(file string) (load.FS, error) {
	data, err := os.ReadFile(file)
	if err != nil {
		return nil, fmt.Errorf("%s: %s", file, reason(err))
	}
	var doc struct {
		Format  string            `json:"format"`
		Replace map[string]string `json:"replace"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("%s: not valid JSON: %v", file, err)
	}
	if doc.Format != "synod-overlay/1" {
		return nil, fmt.Errorf("%s: format must be \"synod-overlay/1\"", file)
	}
	base := load.OSFS{}
	files := map[string][]byte{}
	keys := make([]string, 0, len(doc.Replace))
	for k := range doc.Replace {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		name := filepath.Base(k)
		info, err := os.Stat(k)
		if err != nil || !info.Mode().IsRegular() || (name != "synod.mod" && !strings.HasSuffix(name, ".synod")) {
			return nil, fmt.Errorf("%s: no such file", k)
		}
		content, err := os.ReadFile(doc.Replace[k])
		if err != nil {
			return nil, fmt.Errorf("%s: cannot read %s: %s", k, doc.Replace[k], reason(err))
		}
		abs, err := base.Abs(k)
		if err != nil {
			return nil, fmt.Errorf("%s: %v", k, err)
		}
		files[abs] = content
	}
	return load.OverlayFS{Base: base, Files: files}, nil
}

// reason is the text of err without the operation and path.
func reason(err error) string {
	var pe *fs.PathError
	if errors.As(err, &pe) {
		err = pe.Err
	}
	if errors.Is(err, fs.ErrNotExist) {
		return "no such file"
	}
	return err.Error()
}
