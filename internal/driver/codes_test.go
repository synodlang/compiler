package driver

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestCodesCovered requires a testdata/check/<code>/ folder for every
// diagnostic code of internal/source/codes.go, holding at least one case
// whose expect.txt reports that code.
func TestCodesCovered(t *testing.T) {
	src, err := os.ReadFile("../source/codes.go")
	if err != nil {
		t.Fatal(err)
	}
	// Only the compile-time codes are checked; run-time codes follow the
	// marker comment.
	text, _, _ := strings.Cut(string(src), "\n// Run-time error codes")
	re := regexp.MustCompile(`(?m)^\s+\w+\s+Code = "([a-z-]+)"`)
	ms := re.FindAllStringSubmatch(text, -1)
	if len(ms) != 32 {
		t.Fatalf("found %d codes in codes.go, want 32", len(ms))
	}
	for _, m := range ms {
		code := m[1]
		dir := filepath.Join("../../testdata/check", code)
		cases, err := os.ReadDir(dir)
		if err != nil {
			t.Errorf("%s: no folder testdata/check/%s", code, code)
			continue
		}
		n := 0
		for _, c := range cases {
			if !c.IsDir() {
				continue
			}
			n++
			exp, err := os.ReadFile(filepath.Join(dir, c.Name(), "expect.txt"))
			if err != nil {
				t.Errorf("%s/%s: %v", code, c.Name(), err)
				continue
			}
			if !strings.Contains(string(exp), "error["+code+"]") {
				t.Errorf("%s/%s: expect.txt has no error[%s]", code, c.Name(), code)
			}
		}
		if n == 0 {
			t.Errorf("%s: no case", code)
		}
	}
}
