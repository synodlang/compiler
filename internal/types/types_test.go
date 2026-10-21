package types

import "testing"

var (
	nodeA = NodeOf{"ex/a"}
	nodeB = NodeOf{"ex/b"}
	nodeC = NodeOf{"ex/c"}
	nodeD = NodeOf{"ex/d"}
	recP  = Record{"ex/a", "Prepare"}
	recQ  = Record{"ex/b", "Prepare"}
)

// stubRuns: a uses b, b uses c (so a runs c transitively).
func stubRuns(p, q string) bool {
	switch p + ">" + q {
	case "ex/a>ex/b", "ex/b>ex/c", "ex/a>ex/c":
		return true
	}
	return false
}

func TestAccepts(t *testing.T) {
	list := func(e Type) Type { return List{e} }
	mp := func(k, v Type) Type { return Map{k, v} }
	cases := []struct {
		name string
		s, t Type
		want bool
	}{
		{"equal basic", TText, TText, true},
		{"different basic", TText, TNumber, false},
		{"number is not time", TNumber, TTime, false},
		{"equal node", nodeA, nodeA, true},
		{"equal record", recP, recP, true},
		{"records of two packages", recP, recQ, false},
		{"anything to value", TText, TValue, true},
		{"list to value", list(TText), TValue, true},
		{"node to value", nodeA, TValue, true},
		{"value to text", TValue, TText, false},
		{"value to value", TValue, TValue, true},
		{"invalid as source", TInvalid, TText, true},
		{"invalid as target", TText, TInvalid, true},
		{"invalid to invalid", TInvalid, TInvalid, true},
		{"node type to node", nodeA, TNode, true},
		{"node to node type", TNode, nodeA, false},
		{"text to node", TText, TNode, false},
		{"p.Node to q.Node when p runs q", nodeA, nodeB, true},
		{"transitive run", nodeA, nodeC, true},
		{"reverse run", nodeB, nodeA, false},
		{"unrelated node types", nodeD, nodeB, false},
		{"record to msg", recP, TMsg, true},
		{"msg to record", TMsg, recP, false},
		{"text to msg", TText, TMsg, false},
		{"list covariance", list(nodeA), list(nodeB), true},
		{"list contravariance fails", list(nodeB), list(nodeA), false},
		{"list of record to list of msg", list(recP), list(TMsg), true},
		{"list of node type to list of node", list(nodeA), list(TNode), true},
		{"list of anything to list of value", list(TText), list(TValue), true},
		{"list item mismatch", list(TText), list(TNumber), false},
		{"list to non-list", list(TText), TText, false},
		{"nested lists", list(list(nodeA)), list(list(TNode)), true},
		{"map covariance in key and value", mp(nodeA, recP), mp(nodeB, TMsg), true},
		{"map key mismatch", mp(TText, TNumber), mp(TNumber, TNumber), false},
		{"map value mismatch", mp(TText, TNumber), mp(TText, TText), false},
		{"map to list", mp(TText, TText), list(TText), false},
		{"list to map", list(TText), mp(TText, TText), false},
		{"map to map of value", mp(TText, TNumber), mp(TValue, TValue), true},
		{"empty to list", Empty{}, list(TText), true},
		{"empty to map", Empty{}, mp(TText, TText), true},
		{"empty to empty", Empty{}, Empty{}, true},
		{"empty to text", Empty{}, TText, false},
		{"empty to value", Empty{}, TValue, true},
		{"list to empty", list(TText), Empty{}, false},
	}
	for _, c := range cases {
		if got := Accepts(c.s, c.t, stubRuns); got != c.want {
			t.Errorf("%s: Accepts(%s, %s) = %v, want %v", c.name,
				Format(c.s, "", nil), Format(c.t, "", nil), got, c.want)
		}
	}
}

func TestAcceptsNilRuns(t *testing.T) {
	if !Accepts(nodeA, nodeA, nil) {
		t.Error("p.Node must be accepted as itself with a nil Runs")
	}
	if Accepts(nodeA, nodeB, nil) {
		t.Error("a nil Runs must not relate two packages")
	}
}

func TestJoin(t *testing.T) {
	list := func(e Type) Type { return List{e} }
	mp := func(k, v Type) Type { return Map{k, v} }
	cases := []struct {
		name string
		a, b Type
		want Type // nil: the join fails
	}{
		{"same", TText, TText, TText},
		{"unrelated basics", TText, TNumber, nil},
		{"node to node type: node", TNode, nodeA, TNode},
		{"node type to node", nodeA, TNode, TNode},
		{"p.Node and q.Node", nodeA, nodeB, nodeB},
		{"q.Node and p.Node", nodeB, nodeA, nodeB},
		{"two unrelated node types do not join to node", nodeD, nodeB, nil},
		{"record and msg", recP, TMsg, TMsg},
		{"two records", recP, recQ, nil},
		{"invalid left", TInvalid, TText, TText},
		{"invalid right", TText, TInvalid, TText},
		{"empty and list", Empty{}, list(TText), list(TText)},
		{"list and empty", list(TText), Empty{}, list(TText)},
		{"empty and map", Empty{}, mp(TText, TNumber), mp(TText, TNumber)},
		{"empty and empty", Empty{}, Empty{}, Empty{}},
		{"empty and text", Empty{}, TText, nil},
		{"lists join item-wise", list(nodeA), list(nodeB), list(nodeB)},
		{"lists of unrelated items", list(TText), list(TNumber), nil},
		{"nested lists", list(list(recP)), list(list(TMsg)), list(list(TMsg))},
		{"maps join key-wise and value-wise", mp(nodeA, recP), mp(nodeB, TMsg), mp(nodeB, TMsg)},
		{"maps with unrelated values", mp(TText, TText), mp(TText, TNumber), nil},
		{"list and map", list(TText), mp(TText, TText), nil},
		{"value absorbs", TText, TValue, TValue},
	}
	for _, c := range cases {
		got, ok := Join(c.a, c.b, stubRuns)
		if c.want == nil {
			if ok {
				t.Errorf("%s: Join succeeded with %s", c.name, Format(got, "", nil))
			}
			continue
		}
		if !ok || !Equal(got, c.want) {
			t.Errorf("%s: Join = %v, %v; want %s", c.name, got, ok, Format(c.want, "", nil))
		}
	}
}

func TestEqual(t *testing.T) {
	if !Equal(List{TText}, List{TText}) || Equal(List{TText}, List{TNumber}) {
		t.Error("list equality")
	}
	if !Equal(Map{TText, List{nodeA}}, Map{TText, List{nodeA}}) || Equal(Map{TText, TText}, Map{TNumber, TText}) {
		t.Error("map equality")
	}
	if Equal(recP, recQ) || !Equal(recP, recP) || Equal(nodeA, nodeB) {
		t.Error("named equality")
	}
	if !Equal(Empty{}, Empty{}) || Equal(Empty{}, List{TText}) {
		t.Error("empty equality")
	}
	if Equal(TText, nil) || !Equal(nil, nil) || Equal(nil, TText) {
		t.Error("nil equality")
	}
	if Equal(List{TText}, Map{TText, TText}) {
		t.Error("list is not a map")
	}
}

func TestFormat(t *testing.T) {
	qual := func(p string) string {
		if p == "ex/b" {
			return "bee"
		}
		return ""
	}
	cases := []struct {
		t    Type
		want string
	}{
		{TText, "text"},
		{TNumber, "number"},
		{TBool, "bool"},
		{TTime, "time"},
		{TNode, "node"},
		{TMsg, "msg"},
		{TShape, "shape"},
		{TColor, "color"},
		{TField, "field"},
		{TPackage, "package"},
		{TBlock, "block"},
		{TValue, "value"},
		{TInvalid, "invalid"},
		{Empty{}, "{}"},
		{nodeA, "Node"},
		{nodeB, "bee.Node"},
		{nodeC, "c.Node"},
		{recP, "Prepare"},
		{recQ, "bee.Prepare"},
		{List{TNode}, "list of node"},
		{List{nodeB}, "list of bee.Node"},
		{List{List{TNumber}}, "list of list of number"},
		{Map{TText, TNumber}, "map of text to number"},
		{List{Map{TText, TNumber}}, "list of map of text to number"},
		{Map{nodeA, List{recQ}}, "map of Node to list of bee.Prepare"},
		{Map{TText, Map{TText, TNumber}}, "map of text to map of text to number"},
	}
	for _, c := range cases {
		if got := Format(c.t, "ex/a", qual); got != c.want {
			t.Errorf("Format = %q, want %q", got, c.want)
		}
	}
	if got := Format(nodeB, "ex/a", nil); got != "b.Node" {
		t.Errorf("nil qual: %q", got)
	}
	if got := Format(NodeOf{"ex/a"}, "", nil); got != "a.Node" {
		t.Errorf("empty from: %q", got)
	}
	if got := Format(NodeOf{""}, "ex/a", nil); got != "Node" {
		t.Errorf("empty pkg: %q", got)
	}
}
