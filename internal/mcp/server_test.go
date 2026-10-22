package mcp

import (
	"bytes"
	"encoding/json"
	"os/exec"
	"strings"
	"testing"
)

func testServer() *Server {
	return &Server{Name: "t", Version: "1", Tools: []Tool{{
		Name:        "echo",
		InputSchema: json.RawMessage(`{"type":"object"}`),
		Call: func(args json.RawMessage) (*Result, *Error) {
			return &Result{Content: []Content{{Type: "text", Text: string(args)}}}, nil
		},
	}}}
}

func TestHandle(t *testing.T) {
	cases := []struct{ in, want string }{
		{`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18"}}`,
			`{"jsonrpc":"2.0","id":1,"result":{"capabilities":{"tools":{"listChanged":false}},"protocolVersion":"2025-06-18","serverInfo":{"name":"t","version":"1"}}}`},
		{`{"jsonrpc":"2.0","id":"a","method":"initialize","params":{"protocolVersion":"1999-01-01"}}`,
			`{"jsonrpc":"2.0","id":"a","result":{"capabilities":{"tools":{"listChanged":false}},"protocolVersion":"2025-11-25","serverInfo":{"name":"t","version":"1"}}}`},
		{`{"jsonrpc":"2.0","id":2,"method":"initialize"}`,
			`{"jsonrpc":"2.0","id":2,"error":{"code":-32602,"message":"initialize needs params.protocolVersion"}}`},
		{`{"jsonrpc":"2.0","id":3,"method":"ping"}`, `{"jsonrpc":"2.0","id":3,"result":{}}`},
		{`{"jsonrpc":"2.0","id":4,"method":"resources/list"}`,
			`{"jsonrpc":"2.0","id":4,"error":{"code":-32601,"message":"method not found: resources/list"}}`},
		{`{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"nope"}}`,
			`{"jsonrpc":"2.0","id":5,"error":{"code":-32602,"message":"unknown tool nope"}}`},
		{`{"jsonrpc":"2.0","id":6,"method":"tools/call","params":{"name":"echo","arguments":[1]}}`,
			`{"jsonrpc":"2.0","id":6,"error":{"code":-32602,"message":"arguments must be an object"}}`},
		{`{"jsonrpc":"2.0","id":7,"method":"tools/call","params":{"name":"echo"}}`,
			`{"jsonrpc":"2.0","id":7,"result":{"content":[{"type":"text","text":"{}"}],"isError":false}}`},
		{`not json`, `{"jsonrpc":"2.0","id":null,"error":{"code":-32700,"message":"parse error"}}`},
		{`[{"jsonrpc":"2.0","id":1,"method":"ping"}]`,
			`{"jsonrpc":"2.0","id":null,"error":{"code":-32600,"message":"batches are not supported"}}`},
		{`42`, `{"jsonrpc":"2.0","id":null,"error":{"code":-32600,"message":"invalid request"}}`},
		{`{"jsonrpc":"2.0","id":1}`, `{"jsonrpc":"2.0","id":null,"error":{"code":-32600,"message":"invalid request: no method"}}`},
		{`{"jsonrpc":"1.0","id":1,"method":"ping"}`,
			`{"jsonrpc":"2.0","id":null,"error":{"code":-32600,"message":"invalid request: jsonrpc must be \"2.0\""}}`},
		{`{"jsonrpc":"2.0","id":null,"method":"ping"}`,
			`{"jsonrpc":"2.0","id":null,"error":{"code":-32600,"message":"invalid request: id must be a string or a number"}}`},
		// Notifications and stray responses get nothing.
		{`{"jsonrpc":"2.0","method":"notifications/initialized"}`, ``},
		{`{"jsonrpc":"2.0","method":"notifications/cancelled","params":{"requestId":1}}`, ``},
		{`{"jsonrpc":"2.0","method":"tools/list"}`, ``},
		{`{"jsonrpc":"2.0","method":"nope"}`, ``},
		{`{"jsonrpc":"2.0","id":9,"result":{}}`, ``},
	}
	s := testServer()
	for _, c := range cases {
		if got := string(s.Handle([]byte(c.in))); got != c.want {
			t.Errorf("%s\n got  %s\n want %s", c.in, got, c.want)
		}
	}
}

func TestServeLines(t *testing.T) {
	in := "\n" + `{"jsonrpc":"2.0","id":1,"method":"ping"}` + "\r\n" +
		`{"jsonrpc":"2.0","method":"notifications/initialized"}` + "\n" +
		`{"jsonrpc":"2.0","id":2,"method":"ping"}` // no final newline
	var out bytes.Buffer
	if err := testServer().Serve(strings.NewReader(in), &out); err != nil {
		t.Fatal(err)
	}
	want := `{"jsonrpc":"2.0","id":1,"result":{}}` + "\n" + `{"jsonrpc":"2.0","id":2,"result":{}}` + "\n"
	if out.String() != want {
		t.Errorf("got %q", out.String())
	}
}

// The package must stay free of network code (decision 101).
func TestNoNetwork(t *testing.T) {
	out, err := exec.Command("go", "list", "-deps", ".").Output()
	if err != nil {
		t.Skipf("go list: %v", err)
	}
	for _, p := range strings.Fields(string(out)) {
		if p == "net" || p == "net/http" || strings.HasPrefix(p, "net/") {
			t.Errorf("internal/mcp depends on %s", p)
		}
	}
}
