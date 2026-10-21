package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRun(t *testing.T) {
	dir := t.TempDir()
	cl := filepath.Join(dir, "CHANGELOG.md")
	if err := os.WriteFile(cl, []byte("# Changelog\n\n## v1.0.0 — x\n\nhello\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	outPath := filepath.Join(dir, "out")
	out, err := os.Create(outPath)
	if err != nil {
		t.Fatal(err)
	}
	defer out.Close()
	if err := run([]string{"-tag", "v1.0.0", "-changelog", cl}, out); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(outPath)
	if string(got) != "hello\n" {
		t.Errorf("got %q", got)
	}
	if err := run([]string{"-tag", "v2.0.0", "-changelog", cl}, out); err == nil || !strings.Contains(err.Error(), "no section") {
		t.Errorf("want missing-section error, got %v", err)
	}
	if err := run([]string{"-changelog", cl}, out); err == nil {
		t.Error("want error without -tag")
	}
}
