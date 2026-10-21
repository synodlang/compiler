package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/synodlang/compiler/internal/pipeline"
	"github.com/synodlang/compiler/internal/scene"
	"github.com/synodlang/compiler/internal/sim"
)

// simRun runs the simulator; tests replace it to provoke a panic.
var simRun = sim.Run

// maxSeed is the largest seed (2^53): above it a number is not exact.
const maxSeed = 1 << 53

// defaultTrace is where the trace goes without --trace.
const defaultTrace = "out/trace.jsonl"

// defaultScene is where the scene goes without --scene.
const defaultScene = "out/scene.json"

// runArgs are the parsed arguments of `synod run`.
type runArgs struct {
	folder  string
	seed    *uint64
	trace   string
	scene   string
	pace    int // thousandths
	mode    scene.Mode
	summary bool
	style   styleFlags
}

// notYet are the flags of later milestones: recognized and rejected.
var notYet = map[string]bool{"watch": true, "json-events": true}

// parseRunArgs parses `synod run` arguments: one folder, --seed (n or =n),
// --trace (file or =file) and --summary, in any order. "--" ends the flags.
func parseRunArgs(args []string) (runArgs, error) {
	ra := runArgs{trace: defaultTrace, scene: defaultScene, pace: 1000}
	var pos []string
	flags := true
	for i := 0; i < len(args); i++ {
		a := args[i]
		if !flags || !strings.HasPrefix(a, "-") || a == "-" {
			pos = append(pos, a)
			continue
		}
		if a == "--" {
			flags = false
			continue
		}
		name := strings.TrimLeft(a, "-")
		val, hasVal := "", false
		if k := strings.IndexByte(name, '='); k >= 0 {
			name, val, hasVal = name[:k], name[k+1:], true
		}
		value := func() (string, error) {
			if hasVal {
				return val, nil
			}
			if i+1 >= len(args) {
				return "", fmt.Errorf("--%s needs a value", name)
			}
			i++
			return args[i], nil
		}
		switch {
		case notYet[name]:
			return ra, fmt.Errorf("--%s is not implemented yet", name)
		case name == "summary":
			if hasVal {
				return ra, fmt.Errorf("--summary takes no value")
			}
			ra.summary = true
		case name == "seed":
			v, err := value()
			if err != nil {
				return ra, err
			}
			n, perr := strconv.ParseUint(v, 10, 64)
			if perr != nil || n > maxSeed {
				return ra, fmt.Errorf("--seed needs a whole number from 0 to %d, not %q", uint64(maxSeed), v)
			}
			ra.seed = &n
		case name == "style" || name == "color":
			v, err := value()
			if err != nil {
				return ra, err
			}
			if name == "style" {
				ra.style.style = v
			} else {
				ra.style.color = v
			}
			if err := ra.style.check(); err != nil {
				return ra, err
			}
		case name == "trace":
			v, err := value()
			if err != nil {
				return ra, err
			}
			if v == "" {
				return ra, fmt.Errorf("--trace needs a file name")
			}
			ra.trace = v
		case name == "scene":
			v, err := value()
			if err != nil {
				return ra, err
			}
			if v == "" {
				return ra, fmt.Errorf("--scene needs a file name")
			}
			ra.scene = v
		case name == "pace":
			v, err := value()
			if err != nil {
				return ra, err
			}
			n, ok := pipeline.ParsePace(v)
			if !ok {
				return ra, fmt.Errorf("--pace needs a number from 0.25 to 4, not %q", v)
			}
			ra.pace = n
		case name == "mode":
			v, err := value()
			if err != nil {
				return ra, err
			}
			switch v {
			case "ordered":
				ra.mode = scene.Ordered
			case "proportional":
				ra.mode = scene.Proportional
			default:
				return ra, fmt.Errorf("--mode needs ordered or proportional, not %q", v)
			}
		default:
			return ra, fmt.Errorf("unknown flag %q", a)
		}
	}
	switch len(pos) {
	case 0:
		return ra, fmt.Errorf("needs a folder")
	case 1:
		ra.folder = pos[0]
	default:
		return ra, fmt.Errorf("takes exactly one folder, got %d", len(pos))
	}
	if ra.trace == "-" && ra.scene == "-" {
		return ra, fmt.Errorf("--trace and --scene cannot both be \"-\"")
	}
	return ra, nil
}

// runRun implements `synod run` (implementation/05-cli.md).
func runRun(args []string, stdout, stderr io.Writer) (code int) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(stderr, "synod run: internal error: %v\n", r)
			code = exitUsage
		}
	}()
	ra, err := parseRunArgs(args)
	if err != nil {
		fmt.Fprintf(stderr, "synod run: %v\n", err)
		return exitUsage
	}
	info, err := os.Stat(ra.folder)
	if err != nil {
		fmt.Fprintf(stderr, "synod run: %v\n", err)
		return exitUsage
	}
	if !info.IsDir() {
		fmt.Fprintf(stderr, "synod run: %s is not a folder\n", ra.folder)
		return exitUsage
	}
	folder := filepath.ToSlash(filepath.Clean(ra.folder))
	status := stdout
	if ra.trace == "-" || ra.scene == "-" {
		status = stderr
	}
	diagRich, diagColor := ra.style.resolve(stdout)
	statusRich, statusColor := ra.style.resolve(status)
	out := pipeline.Run(pipeline.RunInput{
		Folder: folder, FolderShown: folder,
		Seed: ra.seed, Pace: ra.pace, Mode: ra.mode,
		TraceDest: ra.trace, SceneDest: ra.scene, TracePath: ra.trace,
		WantSummary: ra.summary,
		DiagStyle:   pipeline.Style{Rich: diagRich, Color: diagColor},
		StatusStyle: pipeline.Style{Rich: statusRich, Color: statusColor},
		Lookup:      (&sources{}).get,
		Sim:         simRun,
	})
	if out.Err != nil {
		fmt.Fprintf(stderr, "synod run: %v\n", out.Err)
		return exitUsage
	}
	if out.Trace == nil {
		if _, err := stdout.Write(out.Status); err != nil {
			fmt.Fprintf(stderr, "synod run: %v\n", err)
			return exitUsage
		}
		return out.Code
	}
	if err := writeOut(ra.trace, out.Trace, stdout); err != nil {
		fmt.Fprintf(stderr, "synod run: %v\n", err)
		return exitUsage
	}
	if err := writeOut(ra.scene, out.Scene, stdout); err != nil {
		fmt.Fprintf(stderr, "synod run: %v\n", err)
		return exitUsage
	}
	text := out.Status
	if ra.summary {
		text = out.Summary
	}
	if _, err := status.Write(text); err != nil {
		fmt.Fprintf(stderr, "synod run: %v\n", err)
		return exitUsage
	}
	return out.Code
}

// writeOut writes data to the file, creating its folders, or to stdout for "-".
func writeOut(path string, data []byte, stdout io.Writer) error {
	if path == "-" {
		_, err := stdout.Write(data)
		return err
	}
	if dir := filepath.Dir(path); dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	return os.WriteFile(path, data, 0o644)
}

// formatSceneLength prints ms as seconds, such as "147.98s".
func formatSceneLength(ms int64) string { return pipeline.FormatSceneLength(ms) }
