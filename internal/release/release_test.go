package release

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var update = flag.Bool("update", false, "rewrite golden files")

func loadSums(t *testing.T) map[string]string {
	t.Helper()
	f, err := os.Open(filepath.Join("testdata", "SHA256SUMS"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	sums, err := ParseSums(f)
	if err != nil {
		t.Fatal(err)
	}
	return sums
}

func golden(t *testing.T, name, got string) {
	t.Helper()
	path := filepath.Join("testdata", name)
	if *update {
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got != string(want) {
		t.Errorf("%s mismatch:\n--- got ---\n%s\n--- want ---\n%s", name, got, want)
	}
}

func TestFormulaGolden(t *testing.T) {
	got, err := Formula("v0.1.0", loadSums(t))
	if err != nil {
		t.Fatal(err)
	}
	golden(t, "synod.rb.golden", got)
}

func TestManifestGolden(t *testing.T) {
	got, err := Manifest("v0.1.0", loadSums(t))
	if err != nil {
		t.Fatal(err)
	}
	golden(t, "synod.json.golden", got)
}

func TestMissingChecksum(t *testing.T) {
	sums := loadSums(t)
	delete(sums, "synod-v0.1.0-windows-arm64.zip")
	if _, err := Manifest("v0.1.0", sums); err == nil || !strings.Contains(err.Error(), "windows-arm64") {
		t.Errorf("Manifest err = %v", err)
	}
	delete(sums, "synod-v0.1.0-linux-amd64.tar.gz")
	if _, err := Formula("v0.1.0", sums); err == nil {
		t.Error("Formula: want error")
	}
}

func TestBadTag(t *testing.T) {
	for _, tag := range []string{"", "0.1.0", "v1", "v1.0.0/x"} {
		if _, err := Formula(tag, nil); err == nil {
			t.Errorf("Formula(%q): want error", tag)
		}
		if _, err := Manifest(tag, nil); err == nil {
			t.Errorf("Manifest(%q): want error", tag)
		}
	}
}

func TestParseSums(t *testing.T) {
	h := strings.Repeat("ab", 32)
	got, err := ParseSums(strings.NewReader(strings.ToUpper(h) + " *a.zip\n\n" + h + "  b.tar.gz\n"))
	if err != nil || got["a.zip"] != h || got["b.tar.gz"] != h {
		t.Errorf("got %v, %v", got, err)
	}
	for _, bad := range []string{"nothex  a.zip\n", h + "\n", h + "  \n"} {
		if _, err := ParseSums(strings.NewReader(bad)); err == nil {
			t.Errorf("ParseSums(%q): want error", bad)
		}
	}
}

func TestArchiveName(t *testing.T) {
	if got := ArchiveName("v1.2.3", "windows", "arm64"); got != "synod-v1.2.3-windows-arm64.zip" {
		t.Error(got)
	}
	if got := ArchiveName("v1.2.3", "darwin", "universal"); got != "synod-v1.2.3-darwin-universal.tar.gz" {
		t.Error(got)
	}
}
