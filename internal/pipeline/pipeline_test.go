package pipeline

import (
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"runtime/debug"
	"strings"
	"testing"

	"github.com/synodlang/compiler/internal/source"
)

func abs(t *testing.T, p string) string {
	t.Helper()
	a, err := filepath.Abs(p)
	if err != nil {
		t.Fatal(err)
	}
	return filepath.ToSlash(a)
}

func readFile(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestRunExample(t *testing.T) {
	golden := abs(t, "../../testdata/run/examples/raft")
	in := RunInput{
		Folder: abs(t, "../../testdata/examples/raft"), FolderShown: "raft",
		Pace:      1000,
		TraceDest: "out/trace.jsonl", SceneDest: "out/scene.json", TracePath: "out/trace.jsonl",
		WantSummary: true,
		Lookup:      func(string) *source.File { return nil },
	}
	out := Run(in)
	if out.Err != nil || out.Code != 0 || out.Failure != nil || len(out.Diags) != 0 {
		t.Fatalf("code %d err %v failure %v diags %v", out.Code, out.Err, out.Failure, out.Diags)
	}
	if got, want := string(out.Summary), readFile(t, golden+"/summary.txt"); got != want {
		t.Errorf("summary differs from the golden:\n%s", got)
	}
	if got, want := string(out.Trace), readFile(t, golden+"/trace.jsonl"); got != want {
		t.Error("trace differs from the golden")
	}
	if got, want := string(out.Scene), readFile(t, golden+"/scene.json"); got != want {
		t.Error("scene differs from the golden")
	}
	re := regexp.MustCompile(`^ok seed \d+: .* -> out/trace\.jsonl\n(broken at .*\n)*scene [0-9.]+s, [1-9]\d* shapes? -> out/scene\.json\n$`)
	if !re.Match(out.Status) {
		t.Errorf("status %q", out.Status)
	}
	if bytes.Contains(out.Summary, []byte("scene ")) {
		t.Error("summary has a scene line")
	}
}

func TestRunDiagnostics(t *testing.T) {
	dir := t.TempDir()
	for name, src := range map[string]string{
		"synod.mod":    "module example.com/m\n\nsynod 0.2\n",
		"p/main.synod": "# package main\n\nnot valid\n",
	} {
		p := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	out := Run(RunInput{Folder: filepath.ToSlash(dir) + "/p", FolderShown: "p", Pace: 1000})
	if out.Code != 1 || len(out.Diags) == 0 || out.Trace != nil || out.Scene != nil || out.Summary != nil {
		t.Fatalf("code %d diags %v", out.Code, out.Diags)
	}
	if !strings.Contains(string(out.Status), "error[") {
		t.Errorf("status %q", out.Status)
	}
}

func TestParsePace(t *testing.T) {
	tests := []struct {
		in   string
		want int
		ok   bool
	}{
		{"1", 1000, true},
		{"4", 4000, true},
		{"0.25", 250, true},
		{"0.5", 500, true},
		{"2.125", 2125, true},
		{"0.1", 100, false},
		{"0.249", 249, false},
		{"4.001", 4001, false},
		{"5", 5000, false},
		{"1.", 0, false},
		{".5", 0, false},
		{"1.2345", 0, false},
		{"01", 0, false},
		{"", 0, false},
		{"-1", 0, false},
		{"1e0", 0, false},
		{"1,5", 0, false},
	}
	for _, tt := range tests {
		got, ok := ParsePace(tt.in)
		if ok != tt.ok || (ok && got != tt.want) {
			t.Errorf("ParsePace(%q) = %d, %v; want %d, %v", tt.in, got, ok, tt.want, tt.ok)
		}
	}
}

func TestFormatSceneLength(t *testing.T) {
	for ms, want := range map[int64]string{0: "0s", 147982: "147.98s", 1000: "1s", 1500: "1.5s", 5: "0.01s", 4: "0s", -1500: "-1.5s"} {
		if got := FormatSceneLength(ms); got != want {
			t.Errorf("FormatSceneLength(%d) = %q, want %q", ms, got, want)
		}
	}
}

func TestVersionText(t *testing.T) {
	want := "language 0.3\ncompiler v1\nformats synod-trace/1 synod-scene/1 synod-check/1 synod-doc/1 synod-explain/1\n"
	if got := VersionText("v1"); got != want {
		t.Errorf("got %q", got)
	}
	if got := CompilerVersion("dev", &debug.BuildInfo{Main: debug.Module{Version: "v0.1.0"}}); got != "v0.1.0" {
		t.Errorf("got %q", got)
	}
}

func TestWriteDiagsShort(t *testing.T) {
	d := source.Diagnostic{File: "a.synod", Severity: "error", Code: source.MissingMain, Message: "no main file in a"}
	var buf bytes.Buffer
	if err := WriteDiags(&buf, []source.Diagnostic{d}, Style{}, nil); err != nil {
		t.Fatal(err)
	}
	if got := buf.String(); got != d.String()+"\n" {
		t.Errorf("got %q", got)
	}
}
