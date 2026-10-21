package scene

import "strconv"

// Shape ids (plan 2.9). A shape's key says where it was drawn and which run
// of that site it belongs to: "<path>:<line>:<col>#<run>.<ordinal>". The
// site is an activation: an observer handler, a timer's after, or a reactive
// re-run's draw. run counts the earlier activations of the same site;
// ordinal counts the shapes created earlier in the same activation
// (including those created in called phrases). Ids are derived from keys.

// idGen hands out activations. Zero value is ready to use.
type idGen struct{ runs map[string]int }

// activation numbers the shapes of one activation of a site.
type activation struct {
	prefix string
	ord    int
}

// begin starts a new activation of the site path:line:col (path is the trace
// path of the position) and counts it.
func (g *idGen) begin(path string, line, col int) *activation {
	if g.runs == nil {
		g.runs = map[string]int{}
	}
	site := path + ":" + strconv.Itoa(line) + ":" + strconv.Itoa(col)
	run := g.runs[site]
	g.runs[site] = run + 1
	return &activation{prefix: site + "#" + strconv.Itoa(run) + "."}
}

// next returns the key of the next shape of the activation.
func (a *activation) next() string {
	k := a.prefix + strconv.Itoa(a.ord)
	a.ord++
	return k
}

const (
	fnvOffset = 14695981039346656037
	fnvPrime  = 1099511628211
)

// hashID is "s" and 12 lowercase hex digits of FNV-1a-64(key) & (2^48-1).
func hashID(key string) string {
	h := uint64(fnvOffset)
	for i := 0; i < len(key); i++ {
		h ^= uint64(key[i])
		h *= fnvPrime
	}
	h &= 1<<48 - 1
	const hexd = "0123456789abcdef"
	var b [13]byte
	b[0] = 's'
	for i := 12; i >= 1; i-- {
		b[i] = hexd[h&15]
		h >>= 4
	}
	return string(b[:])
}

// assignIDs gives each key its id, in order. A collision hashes key+"#2",
// key+"#3", … until the id is free.
func assignIDs(keys []string) []string {
	used := make(map[string]bool, len(keys))
	out := make([]string, len(keys))
	for i, k := range keys {
		id := hashID(k)
		for n := 2; used[id]; n++ {
			id = hashID(k + "#" + strconv.Itoa(n))
		}
		used[id] = true
		out[i] = id
	}
	return out
}
