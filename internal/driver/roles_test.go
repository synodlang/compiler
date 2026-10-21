package driver

import (
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/synodlang/compiler/internal/load"
)

// rolesSrc holds one example of each kind of token; the tests below read
// back the role of each word, by line.
const rolesSrc = `# package p
use std/lists
use std/net
use std/time

Vote has
  set term to 0
  set granted to false

Node has
  set Known to {}
  set peers to {}
  set seenAt to {}
  set label to "x"

define Join [n: Node] to [seed: node]
  set n.peers to {seed}

define [n: Node] Version
  result 1

define [n: Node] Learn [k: node] at [v: number]
  set item k of n.Known to v
  set item k of n.seenAt to time.Now
  if ((k is not n) and (k is not in n.peers))
    set n.peers to (lists.Append k to n.peers)

define Majority of [total: number]
  result ((total divided by 2) plus 1)

define [n: Node] start
  n.Learn n at n.Version
  if ((lists.Size n.peers) at least 1)
    net.Send (message Vote with term 1, granted true) from n to n

when [n: Node] (net.Received Vote from sender)
  if (Vote.term more than 0)
    set n.label to "y"
  if (Vote.granted and (sender is not n))
    set n.label to "z"
`

// ln is the 1-based number of the first line of rolesSrc that starts with
// prefix (after indentation).
func ln(t *testing.T, prefix string) int {
	t.Helper()
	for i, l := range strings.Split(rolesSrc, "\n") {
		if strings.HasPrefix(strings.TrimLeft(l, " "), prefix) {
			return i + 1
		}
	}
	t.Fatalf("no line starts with %q", prefix)
	return 0
}

func lineRoles(t *testing.T, line int) string {
	t.Helper()
	m := load.MapFS{"/m/synod.mod": "module example.com/t\nsynod 0.2\n", "/m/p/p.synod": rolesSrc}
	res := Check(Options{Load: load.Config{FS: m}, Tokens: true}, []string{"/m/p"})
	if len(res.Diags) > 0 {
		t.Fatalf("diagnostics: %v", res.Diags)
	}
	src := strings.Split(rolesSrc, "\n")[line-1]
	runes := []rune(src)
	var parts []string
	for f, toks := range res.Tokens {
		if !strings.HasSuffix(f, "p.synod") {
			continue
		}
		for _, tk := range toks {
			if tk.Line != line {
				continue
			}
			s := string(runes[tk.Col-1 : tk.Col-1+tk.Len])
			if utf8.RuneCountInString(s) != tk.Len {
				t.Fatalf("bad token length at %d:%d", tk.Line, tk.Col)
			}
			d := ""
			if tk.Def != nil {
				d = fmt.Sprintf("@%d", tk.Def.Pos.Line)
			}
			parts = append(parts, s+"/"+string(tk.Role)+d)
		}
	}
	return strings.Join(parts, " ")
}

// expand replaces each {prefix} of want by the line number of the line
// starting with prefix.
func expand(t *testing.T, want string) string {
	t.Helper()
	for {
		i := strings.IndexByte(want, '{')
		if i < 0 {
			return want
		}
		j := strings.IndexByte(want[i:], '}') + i
		want = want[:i] + fmt.Sprint(ln(t, want[i+1:j])) + want[j+1:]
	}
}

func TestTokenRoles(t *testing.T) {
	cases := []struct {
		name string
		line string // prefix of the line
		want string // {prefix} stands for the number of a line
	}{
		{"comment", "# package", "# package p/comment"},
		{"import path is a name", "use std/lists", "use/language std/lists/name"},
		{"record type name", "Vote has", "Vote/type@{Vote has} has/language"},
		{"record field declaration", "set term", "set/language term/field@{set term} to/language 0/value"},
		{"bool literal", "set granted", "set/language granted/field@{set granted} to/language false/value"},
		{"Node is a type", "Node has", "Node/type@{Node has} has/language"},
		{"node field declaration", "set Known", "set/language Known/field@{set Known} to/language"},
		{"text literal", "set label", `set/language label/field@{set label} to/language "x"/value`},
		{"define head and its literal word", "define Join",
			"define/language Join/phrase@{define Join} n/name@{define Join} Node/type to/phrase seed/name@{define Join} node/type"},
		{"define receiver head", "define [n: Node] Version",
			"define/language n/name@{define [n: Node] Version} Node/type Version/phrase@{define [n: Node] Version}"},
		{"hole names and types, literal later word", "define [n: Node] Learn",
			"define/language n/name@{define [n: Node] Learn} Node/type Learn/phrase@{define [n: Node] Learn} k/name@{define [n: Node] Learn} node/type at/phrase v/name@{define [n: Node] Learn} number/type"},
		{"literal first word after head", "define Majority",
			"define/language Majority/phrase@{define Majority} of/phrase total/name@{define Majority} number/type"},
		{"start is a language word", "define [n: Node] start",
			"define/language n/name@{define [n: Node] start} Node/type start/language"},
		{"field member", "set n.peers to {seed}", "set/language n/name@{define Join} peers/field@{set peers} to/language seed/name@{define Join}"},
		{"package qualifier is a name, head a phrase", "set item k of n.seenAt",
			"set/language item/language k/name@{define [n: Node] Learn} of/language n/name@{define [n: Node] Learn} seenAt/field@{set seenAt} to/language time/name Now/phrase"},
		{"qualified call with literal word", "set n.peers to (lists",
			"set/language n/name@{define [n: Node] Learn} peers/field@{set peers} to/language lists/name Append/phrase k/name@{define [n: Node] Learn} to/phrase n/name@{define [n: Node] Learn} peers/field@{set peers}"},
		{"multi-word operators are language", "if ((k is not n)",
			"if/language k/name@{define [n: Node] Learn} is/language not/language n/name@{define [n: Node] Learn} and/language k/name@{define [n: Node] Learn} is/language not/language in/language n/name@{define [n: Node] Learn} peers/field@{set peers}"},
		{"divided by", "result ((total",
			"result/language total/name@{define Majority} divided/language by/language 2/value plus/language 1/value"},
		{"receiver phrase calls", "n.Learn n at",
			"n/name@{define [n: Node] start} Learn/phrase@{define [n: Node] Learn} n/name@{define [n: Node] start} at/phrase n/name@{define [n: Node] start} Version/phrase@{define [n: Node] Version}"},
		{"std call and at least", "if ((lists.Size",
			"if/language lists/name Size/phrase n/name@{define [n: Node] start} peers/field@{set peers} at/language least/language 1/value"},
		{"record literal", "net.Send",
			"net/name Send/phrase message/language Vote/type@{Vote has} with/language term/field@{set term} 1/value granted/field@{set granted} true/value from/phrase n/name@{define [n: Node] start} to/phrase n/name@{define [n: Node] start}"},
		{"when header", "when",
			"when/language n/name@{when} Node/type net/name Received/phrase Vote/type@{Vote has} from/phrase sender/name@{when}"},
		{"record binding field", "if (Vote.term",
			"if/language Vote/name@{when} term/field@{set term} more/language than/language 0/value"},
		{"node field through the receiver", "set n.label to \"y\"",
			`set/language n/name@{when} label/field@{set label} to/language "y"/value`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got, want := lineRoles(t, ln(t, c.line)), expand(t, c.want); got != want {
				t.Errorf("line %q\n got %s\nwant %s", c.line, got, want)
			}
		})
	}
}
