package scene

import (
	"regexp"
	"testing"
)

func TestHashID(t *testing.T) {
	// FNV-1a 64 of the empty string is the offset basis
	if got := hashID(""); got != "s"+""+hex12(14695981039346656037&(1<<48-1)) {
		t.Errorf("empty: %s", got)
	}
	if !regexp.MustCompile(`^s[0-9a-f]{12}$`).MatchString(hashID("a.synod:3:5#0.0")) {
		t.Error("shape of id")
	}
	// stable across calls and different for different keys
	if hashID("x") != hashID("x") || hashID("x") == hashID("y") {
		t.Error("stability")
	}
	// pinned value: FNV-1a-64("a") = af63dc4c8601ec8c
	if got := hashID("a"); got != "s"+"dc4c8601ec8c" {
		t.Errorf("a: %s", got)
	}
}

func hex12(v uint64) string {
	const d = "0123456789abcdef"
	b := make([]byte, 12)
	for i := 11; i >= 0; i-- {
		b[i] = d[v&15]
		v >>= 4
	}
	return string(b)
}

func TestActivationKeys(t *testing.T) {
	var g idGen
	a := g.begin("main.synod", 4, 2)
	if a.next() != "main.synod:4:2#0.0" || a.next() != "main.synod:4:2#0.1" {
		t.Fatal("ordinals")
	}
	b := g.begin("main.synod", 4, 2)
	c := g.begin("main.synod", 9, 1)
	if k := b.next(); k != "main.synod:4:2#1.0" {
		t.Fatalf("run index: %s", k)
	}
	if k := c.next(); k != "main.synod:9:1#0.0" {
		t.Fatalf("other site: %s", k)
	}
}

func TestAssignIDsCollision(t *testing.T) {
	ids := assignIDs([]string{"a", "b", "a", "a"})
	if ids[0] != hashID("a") || ids[1] != hashID("b") {
		t.Fatal("first uses")
	}
	if ids[2] != hashID("a#2") || ids[3] != hashID("a#3") {
		t.Fatalf("collisions: %v", ids)
	}
	seen := map[string]bool{}
	for _, id := range ids {
		if seen[id] {
			t.Fatal("duplicate id")
		}
		seen[id] = true
	}
	// stability: the same keys give the same ids
	again := assignIDs([]string{"a", "b", "a", "a"})
	for i := range ids {
		if ids[i] != again[i] {
			t.Fatal("unstable")
		}
	}
}
