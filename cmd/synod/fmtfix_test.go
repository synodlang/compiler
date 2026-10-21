package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// copyPaxos copies the paxos library file to a temp folder and returns its
// path and the original text.
func copyPaxos(t *testing.T) (string, string) {
	t.Helper()
	orig, err := os.ReadFile(filepath.Join(examples, "paxos", "paxos.synod"))
	if err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(t.TempDir(), "paxos", "paxos.synod")
	write(t, p, string(orig))
	return p, string(orig)
}

func TestFmtReordersSettings(t *testing.T) {
	needReal(t)
	p, orig := copyPaxos(t)
	const canon = "(message Promise with number n.Promised, prior n.AcceptedBallot, priorValue n.AcceptedValue)"
	const swapped = "(message Promise with prior n.AcceptedBallot, number n.Promised, priorValue n.AcceptedValue)"
	if !strings.Contains(orig, canon) {
		t.Fatal("paxos example changed; update this test")
	}
	write(t, p, strings.Replace(orig, canon, swapped, 1))
	code, out, _ := exec("check", p)
	if code != 1 || !strings.Contains(out, "error[non-canonical]") || !strings.Contains(out, "hint: write ") {
		t.Fatalf("check: code=%d out=%q", code, out)
	}
	if code, _, errs := exec("fmt", p); code != 0 {
		t.Fatalf("fmt: code=%d err=%q", code, errs)
	}
	if got := read(t, p); got != orig {
		t.Errorf("not restored")
	}
}

func TestFmtLowercasesName(t *testing.T) {
	needReal(t)
	p, orig := copyPaxos(t)
	const canon = "  each p in n.Peers\n    net.Send (message Prepare with number n.ballot) from n to p\n"
	const upper = "  each P in n.Peers\n    net.Send (message Prepare with number n.ballot) from n to P\n"
	if !strings.Contains(orig, canon) {
		t.Fatal("paxos example changed; update this test")
	}
	write(t, p, strings.Replace(orig, canon, upper, 1))
	code, out, _ := exec("check", p)
	if code != 1 || !strings.Contains(out, "error[non-canonical]") {
		t.Fatalf("check: code=%d out=%q", code, out)
	}
	if code, _, errs := exec("fmt", p); code != 0 {
		t.Fatalf("fmt: code=%d err=%q", code, errs)
	}
	if got := read(t, p); got != orig {
		t.Errorf("not restored")
	}
}

// A file in a package that does not load keeps the M1 behaviour: it is
// formatted without fixes, and nothing else changes.
func TestFmtBrokenPackageKeepsM1(t *testing.T) {
	needReal(t)
	dir := t.TempDir()
	a := filepath.Join(dir, "pkg", "a.synod")
	b := filepath.Join(dir, "pkg", "b.synod")
	write(t, a, "# package pkg\nNode has\n  set Foo to 0   \n")
	write(t, b, "# package pkg\nfoo x-1\n") // does not lex: the package is broken
	if code, _, _ := exec("fmt", dir); code != 1 {
		t.Errorf("code=%d", code)
	}
	if got := read(t, a); got != "# package pkg\nNode has\n  set Foo to 0\n" {
		t.Errorf("a.synod: %q", got)
	}
	if read(t, b) != "# package pkg\nfoo x-1\n" {
		t.Error("broken file changed")
	}
}
