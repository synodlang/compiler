package release

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNotes(t *testing.T) {
	in, err := os.ReadFile(filepath.Join("testdata", "notes", "CHANGELOG.md"))
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name, tag, golden, wantErr string
	}{
		{name: "heading suffix", tag: "v0.2.0", golden: "v0.2.0.golden"},
		{name: "middle section ends at next heading", tag: "v0.1.0", golden: "v0.1.0.golden"},
		{name: "last section", tag: "v0.0.2", golden: "v0.0.2.golden"},
		{name: "bare heading", tag: "v0.3.0", golden: "v0.3.0.golden"},
		{name: "missing", tag: "v9.9.9", wantErr: "no section"},
		{name: "empty", tag: "v0.0.1", wantErr: "is empty"},
		{name: "unreleased not matched by version", tag: "Unreleased-x", wantErr: "no section"},
		{name: "prefix of longer tag", tag: "v0.2", wantErr: "no section"},
		{name: "empty tag", tag: "", wantErr: "empty tag"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Notes(string(in), tt.tag)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v, want containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			golden(t, filepath.Join("notes", tt.golden), got)
		})
	}
}

func TestNotesUnreleasedByName(t *testing.T) {
	got, err := Notes("## Unreleased\n\nwip\n\n## v1.0.0\n\nfinal\n", "v1.0.0")
	if err != nil || got != "final\n" {
		t.Fatalf("got %q, %v", got, err)
	}
}
