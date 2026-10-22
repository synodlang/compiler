// Package mcp is a small stdio Model Context Protocol server: JSON-RPC 2.0,
// one message per line, with only initialize, notifications/initialized, ping,
// tools/list, tools/call and notifications/cancelled. It uses the standard
// library only and has no network code.
package mcp

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"io"
)

// Versions lists the supported protocol versions, newest first. A client's
// version is echoed when it is in the list, otherwise Versions[0] is answered.
var Versions = []string{"2025-11-25", "2025-06-18", "2025-03-26"}

// JSON-RPC error codes.
const (
	CodeParse          = -32700
	CodeInvalidRequest = -32600
	CodeMethodNotFound = -32601
	CodeInvalidParams  = -32602
)

// Error is a JSON-RPC error. A tool's Call returns it for bad arguments.
type Error struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e *Error) Error() string { return e.Message }

// InvalidParams builds a -32602 error.
func InvalidParams(msg string) *Error { return &Error{Code: CodeInvalidParams, Message: msg} }

// Content is one text item of a tool result.
type Content struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// Result is a tool call result.
type Result struct {
	Content           []Content `json:"content"`
	StructuredContent any       `json:"structuredContent,omitempty"`
	IsError           bool      `json:"isError"`
}

// Tool is one registered tool. InputSchema and Annotations are raw JSON so the
// listing is byte-stable.
type Tool struct {
	Name        string
	Description string
	InputSchema json.RawMessage
	Annotations json.RawMessage
	// Call runs the tool. A non-nil *Error becomes a JSON-RPC error; tool
	// failures belong in Result.IsError.
	Call func(args json.RawMessage) (*Result, *Error)
}

// Server is a stdio MCP server. Requests are handled one at a time.
type Server struct {
	Name    string
	Version string
	Tools   []Tool
}

type request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Method  *string         `json:"method"`
	Params  json.RawMessage `json:"params"`
	Result  json.RawMessage `json:"result"`
	Error   json.RawMessage `json:"error"`
}

type errBody struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Error   *Error          `json:"error"`
}

type okBody struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result"`
}

// Serve reads lines from in until it is closed and writes responses to out.
func (s *Server) Serve(in io.Reader, out io.Writer) error {
	r := bufio.NewReader(in)
	for {
		line, err := r.ReadBytes('\n')
		line = bytes.TrimRight(line, "\r\n")
		if len(bytes.TrimSpace(line)) > 0 {
			if resp := s.Handle(line); resp != nil {
				if _, werr := out.Write(append(resp, '\n')); werr != nil {
					return werr
				}
			}
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
	}
}

// Handle answers one message line. It returns nil when no response is due.
func (s *Server) Handle(line []byte) []byte {
	null := json.RawMessage("null")
	trim := bytes.TrimSpace(line)
	if !json.Valid(trim) {
		return marshal(errBody{"2.0", null, &Error{CodeParse, "parse error"}})
	}
	if trim[0] == '[' {
		return marshal(errBody{"2.0", null, &Error{CodeInvalidRequest, "batches are not supported"}})
	}
	var req request
	if err := json.Unmarshal(trim, &req); err != nil || trim[0] != '{' {
		return marshal(errBody{"2.0", null, &Error{CodeInvalidRequest, "invalid request"}})
	}
	if req.Method == nil {
		if req.ID != nil && (req.Result != nil || req.Error != nil) {
			return nil // a response to a request we never send
		}
		return marshal(errBody{"2.0", null, &Error{CodeInvalidRequest, "invalid request: no method"}})
	}
	if req.JSONRPC != "2.0" {
		return marshal(errBody{"2.0", null, &Error{CodeInvalidRequest, `invalid request: jsonrpc must be "2.0"`}})
	}
	if req.ID == nil {
		return nil // notification: initialized, cancelled and the rest are ignored
	}
	if c := req.ID[0]; c != '"' && c != '-' && (c < '0' || c > '9') {
		return marshal(errBody{"2.0", null, &Error{CodeInvalidRequest, "invalid request: id must be a string or a number"}})
	}
	result, rerr := s.dispatch(*req.Method, req.Params)
	if rerr != nil {
		return marshal(errBody{"2.0", req.ID, rerr})
	}
	return marshal(okBody{"2.0", req.ID, result})
}

func (s *Server) dispatch(method string, params json.RawMessage) (any, *Error) {
	switch method {
	case "initialize":
		var p struct {
			ProtocolVersion string `json:"protocolVersion"`
		}
		if len(params) == 0 || json.Unmarshal(params, &p) != nil || p.ProtocolVersion == "" {
			return nil, InvalidParams("initialize needs params.protocolVersion")
		}
		ver := Versions[0]
		for _, v := range Versions {
			if v == p.ProtocolVersion {
				ver = v
			}
		}
		return map[string]any{
			"protocolVersion": ver,
			"capabilities":    map[string]any{"tools": map[string]any{"listChanged": false}},
			"serverInfo":      map[string]any{"name": s.Name, "version": s.Version},
		}, nil
	case "ping":
		return map[string]any{}, nil
	case "tools/list":
		type item struct {
			Name        string          `json:"name"`
			Description string          `json:"description"`
			InputSchema json.RawMessage `json:"inputSchema"`
			Annotations json.RawMessage `json:"annotations,omitempty"`
		}
		list := make([]item, 0, len(s.Tools))
		for _, t := range s.Tools {
			list = append(list, item{t.Name, t.Description, t.InputSchema, t.Annotations})
		}
		return map[string]any{"tools": list}, nil
	case "tools/call":
		var p struct {
			Name      string          `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
		}
		if len(params) == 0 || json.Unmarshal(params, &p) != nil || p.Name == "" {
			return nil, InvalidParams("tools/call needs params.name and an object params.arguments")
		}
		args := p.Arguments
		if len(args) == 0 || string(args) == "null" {
			args = json.RawMessage("{}")
		}
		if args[0] != '{' {
			return nil, InvalidParams("arguments must be an object")
		}
		for i := range s.Tools {
			if s.Tools[i].Name == p.Name {
				res, err := s.Tools[i].Call(args)
				if err != nil {
					return nil, err
				}
				return res, nil
			}
		}
		return nil, InvalidParams("unknown tool " + p.Name)
	}
	return nil, &Error{CodeMethodNotFound, "method not found: " + method}
}

func marshal(v any) []byte {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return []byte(`{"jsonrpc":"2.0","id":null,"error":{"code":-32603,"message":"internal error"}}`)
	}
	return bytes.TrimRight(b.Bytes(), "\n")
}
