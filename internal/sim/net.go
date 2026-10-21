package sim

// link is an unordered pair of nodes.
type link struct{ a, b int }

func mkLink(a, b int) link {
	if a > b {
		a, b = b, a
	}
	return link{a, b}
}

// dropKey names a directed link and a record type for DropNext.
type dropKey struct {
	from, to int
	typ      string
}

// network is the simulated network's configuration.
type network struct {
	latA, latB int64
	loss       float64
	cut        map[link]bool
	dropNext   map[dropKey]int
}

func newNetwork() network {
	return network{
		latA: 10_000, latB: 30_000,
		cut:      map[link]bool{},
		dropNext: map[dropKey]int{},
	}
}
