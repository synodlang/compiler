package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRun(t *testing.T) {
	dir := t.TempDir()
	formula := filepath.Join(dir, "tap", "Formula", "synod.rb")
	scoop := filepath.Join(dir, "bucket", "synod.json")
	sums := filepath.Join("..", "..", "testdata", "SHA256SUMS")
	if err := run([]string{"-tag", "v0.1.0", "-sums", sums, "-formula", formula, "-scoop", scoop}); err != nil {
		t.Fatal(err)
	}
	for path, want := range map[string]string{formula: "class Synod < Formula", scoop: `"bin": "synod.exe"`} {
		b, err := os.ReadFile(path)
		if err != nil || !strings.Contains(string(b), want) {
			t.Errorf("%s: %v\n%s", path, err, b)
		}
	}
}

func TestRunErrors(t *testing.T) {
	if err := run(nil); err == nil {
		t.Error("no args: want error")
	}
	if err := run([]string{"-tag", "v0.1.0", "-sums", "nope", "-scoop", "x"}); err == nil {
		t.Error("missing sums: want error")
	}
}
