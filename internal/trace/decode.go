package trace

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/synodlang/compiler/internal/ir"
	"github.com/synodlang/compiler/internal/source"
	"github.com/synodlang/compiler/internal/value"
)

// decoder turns trace.jsonl back into facts.
type decoder struct {
	pkgs   map[string]*ir.Package // by qualifier
	byPath map[string]*ir.Package
	qual   map[*ir.Package]string
	line   int
}

type decodeError struct {
	line int
	msg  string
}

func (e *decodeError) Error() string { return fmt.Sprintf("trace: line %d: %s", e.line, e.msg) }

func (d *decoder) fail(format string, a ...any) error {
	return &decodeError{d.line, fmt.Sprintf(format, a...)}
}

// Decode reads trace.jsonl as written by Encode for prog. Qualified names
// are resolved against the packages of prog, then against extra (the
// packages that prog does not run, such as std/look/basic); a package of
// extra is named by its Name unless prog already uses it. Unexported record
// fields, which the trace hides, come back as the text "…". Fact.Step is 0,
// and the position of a sim.Failed fact is parsed from its trace path.
func Decode(r io.Reader, prog *ir.Program, extra ...*ir.Package) (Header, []Fact, error) {
	d := &decoder{pkgs: map[string]*ir.Package{}, byPath: map[string]*ir.Package{}, qual: map[*ir.Package]string{}}
	qual := Qualifiers(prog)
	for _, p := range prog.Packages {
		d.pkgs[qual(p)] = p
		d.qual[p] = qual(p)
		d.byPath[p.Path] = p
	}
	for _, p := range extra {
		if _, ok := d.byPath[p.Path]; ok {
			continue
		}
		d.byPath[p.Path] = p
		if _, ok := d.pkgs[p.Name]; !ok {
			d.pkgs[p.Name] = p
			d.qual[p] = p.Name
		}
	}
	sc := bufio.NewReaderSize(r, 1<<16)
	var h Header
	var facts []Fact
	for {
		line, err := sc.ReadBytes('\n')
		if len(line) > 0 {
			d.line++
			if line[len(line)-1] != '\n' {
				return h, nil, d.fail("line does not end in a newline")
			}
			line = line[:len(line)-1]
			if d.line == 1 {
				h, err = d.header(line)
				if err != nil {
					return h, nil, err
				}
			} else {
				f, ferr := d.fact(line, len(facts)+1)
				if ferr != nil {
					return h, nil, ferr
				}
				facts = append(facts, f)
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return h, nil, err
		}
	}
	if d.line == 0 {
		return h, nil, d.fail("empty trace")
	}
	return h, facts, nil
}

type rawObj map[string]json.RawMessage

func (d *decoder) obj(b []byte) (rawObj, error) {
	var o rawObj
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	if err := dec.Decode(&o); err != nil || o == nil {
		return nil, d.fail("not a JSON object")
	}
	return o, nil
}

func (d *decoder) header(b []byte) (Header, error) {
	var h Header
	o, err := d.obj(b)
	if err != nil {
		return h, err
	}
	var format string
	if json.Unmarshal(o["format"], &format) != nil || format != "synod-trace/1" {
		return h, d.fail("not a synod-trace/1 header")
	}
	if err := json.Unmarshal(o["seed"], &h.Seed); err != nil {
		return h, d.fail("bad seed")
	}
	if err := json.Unmarshal(o["program"], &h.Program); err != nil {
		return h, d.fail("bad program")
	}
	if err := json.Unmarshal(o["packages"], &h.Packages); err != nil {
		return h, d.fail("bad packages")
	}
	return h, nil
}

func (d *decoder) node(raw json.RawMessage) (int, error) {
	if string(raw) == "null" {
		return 0, nil
	}
	var s string
	if json.Unmarshal(raw, &s) != nil {
		return 0, d.fail("bad node %s", raw)
	}
	n, err := strconv.Atoi(strings.TrimPrefix(s, "n"))
	if err != nil || !strings.HasPrefix(s, "n") || n <= 0 {
		return 0, d.fail("bad node %q", s)
	}
	return n, nil
}

func (d *decoder) nodes(raw json.RawMessage) ([]int, error) {
	var items []json.RawMessage
	if json.Unmarshal(raw, &items) != nil {
		return nil, d.fail("bad node list")
	}
	out := make([]int, len(items))
	for i, it := range items {
		n, err := d.node(it)
		if err != nil {
			return nil, err
		}
		out[i] = n
	}
	return out, nil
}

// split cuts "q.Name" into its qualifier and name at the last dot.
func (d *decoder) qualified(s string) (*ir.Package, string, error) {
	i := strings.LastIndexByte(s, '.')
	if i < 0 {
		return nil, "", d.fail("bad name %q", s)
	}
	p := d.pkgs[s[:i]]
	if p == nil {
		return nil, "", d.fail("unknown package %q", s[:i])
	}
	return p, s[i+1:], nil
}

func (d *decoder) nodeField(s string) (*ir.Field, error) {
	p, name, err := d.qualified(s)
	if err != nil {
		return nil, err
	}
	for _, f := range p.NodeFields {
		if f.Name == name {
			return f, nil
		}
	}
	return nil, d.fail("unknown field %q", s)
}

func (d *decoder) fact(b []byte, seq int) (Fact, error) {
	f := Fact{Node: -1}
	o, err := d.obj(b)
	if err != nil {
		return f, err
	}
	var n int
	if json.Unmarshal(o["seq"], &n) != nil || n != seq {
		return f, d.fail("seq is not %d", seq)
	}
	if json.Unmarshal(o["t"], &f.T) != nil {
		return f, d.fail("bad t")
	}
	if json.Unmarshal(o["ev"], &f.Ev) != nil {
		return f, d.fail("bad ev")
	}
	lay, ok := layouts[f.Ev]
	if !ok {
		return f, d.fail("unknown event %q", f.Ev)
	}
	if raw, ok := o["node"]; ok && lay&kNode != 0 {
		if f.Node, err = d.node(raw); err != nil {
			return f, err
		}
	}
	if lay&kMsg != 0 {
		var m struct {
			ID     int             `json:"id"`
			Type   string          `json:"type"`
			Fields json.RawMessage `json:"fields"`
		}
		if json.Unmarshal(o["msg"], &m) != nil {
			return f, d.fail("bad msg")
		}
		f.MsgID = m.ID
		if f.Msg, err = d.recordOf(m.Type, m.Fields); err != nil {
			return f, err
		}
	}
	if lay&kFromTo != 0 {
		if f.From, err = d.node(o["from"]); err != nil {
			return f, err
		}
		if f.To, err = d.node(o["to"]); err != nil {
			return f, err
		}
	}
	if lay&kReason != 0 {
		if json.Unmarshal(o["reason"], &f.Reason) != nil {
			return f, d.fail("bad reason")
		}
	}
	if lay&kRecord != 0 {
		v, err := d.value(o["record"])
		if err != nil {
			return f, err
		}
		if v.K != value.KRecord {
			return f, d.fail("record is not a record")
		}
		f.Record = v.Record()
	}
	switch f.Ev {
	case "sim.Ended":
		if json.Unmarshal(o["reason"], &f.Reason) != nil {
			return f, d.fail("bad reason")
		}
	case "sim.Added", "sim.Restarted":
		err = d.runsState(&f, o)
	case "sim.Changed":
		var name string
		if json.Unmarshal(o["field"], &name) != nil {
			return f, d.fail("bad field")
		}
		if f.Field, err = d.nodeField(name); err != nil {
			return f, err
		}
		if f.Old, err = d.value(o["old"]); err != nil {
			return f, err
		}
		f.New, err = d.value(o["new"])
	case "sim.Broken":
		if json.Unmarshal(o["rule"], &f.Rule) != nil {
			return f, d.fail("bad rule")
		}
	case "sim.Failed":
		err = d.failed(&f, o)
	case "net.Partitioned":
		if f.A, err = d.nodes(o["a"]); err != nil {
			return f, err
		}
		f.B, err = d.nodes(o["b"])
	case "disk.Written":
		if json.Unmarshal(o["key"], &f.Key) != nil {
			return f, d.fail("bad key")
		}
	}
	return f, err
}

func (d *decoder) failed(f *Fact, o rawObj) error {
	var code, msg string
	if json.Unmarshal(o["code"], &code) != nil || json.Unmarshal(o["message"], &msg) != nil {
		return d.fail("bad sim.Failed")
	}
	fl := &value.Fault{Code: source.Code(code), Message: msg}
	if string(o["pos"]) != "null" {
		var pos string
		if json.Unmarshal(o["pos"], &pos) != nil {
			return d.fail("bad pos")
		}
		i := strings.LastIndexByte(pos, ':')
		j := strings.LastIndexByte(pos[:max(i, 0)], ':')
		if i < 0 || j < 0 {
			return d.fail("bad pos %q", pos)
		}
		line, e1 := strconv.Atoi(pos[j+1 : i])
		col, e2 := strconv.Atoi(pos[i+1:])
		if e1 != nil || e2 != nil {
			return d.fail("bad pos %q", pos)
		}
		fl.Pos = ir.Pos{File: pos[:j], Line: line, Col: col}
		fl.HasPos = true
	}
	f.Fault = fl
	return nil
}

func (d *decoder) runsState(f *Fact, o rawObj) error {
	var paths []string
	if json.Unmarshal(o["runs"], &paths) != nil {
		return d.fail("bad runs")
	}
	for _, p := range paths {
		pk := d.byPath[p]
		if pk == nil {
			return d.fail("unknown package %q", p)
		}
		f.Runs = append(f.Runs, pk)
	}
	var state rawObj
	if json.Unmarshal(o["state"], &state) != nil {
		return d.fail("bad state")
	}
	used := 0
	for _, p := range f.Runs {
		for _, fld := range p.NodeFields {
			if !fld.Exported {
				continue
			}
			key := d.qual[p] + "." + fld.Name
			raw, ok := state[key]
			if !ok {
				return d.fail("state has no value for %s.%s", p.Name, fld.Name)
			}
			v, err := d.value(raw)
			if err != nil {
				return err
			}
			f.State = append(f.State, v)
			used++
		}
	}
	if used != len(state) {
		return d.fail("state has %d values, want %d", len(state), used)
	}
	return nil
}

// recordOf builds the record of type "q.Name" from its fields object.
func (d *decoder) recordOf(typ string, fields json.RawMessage) (*value.Record, error) {
	p, name, err := d.qualified(typ)
	if err != nil {
		return nil, err
	}
	var rt *ir.Record
	for _, r := range p.Records {
		if r.Name == name {
			rt = r
			break
		}
	}
	if rt == nil {
		return nil, d.fail("unknown record %q", typ)
	}
	var fo rawObj
	if json.Unmarshal(fields, &fo) != nil {
		return nil, d.fail("bad fields of %s", typ)
	}
	vals := make([]value.Value, len(rt.Fields))
	for i, fld := range rt.Fields {
		raw, ok := fo[fld.Name]
		if !ok {
			return nil, d.fail("%s has no field %s", typ, fld.Name)
		}
		if !fld.Exported {
			vals[i] = value.Text("…")
			continue
		}
		if vals[i], err = d.value(raw); err != nil {
			return nil, err
		}
	}
	return &value.Record{Type: rt, Fields: vals}, nil
}

// value reads a value in the trace encoding.
func (d *decoder) value(raw json.RawMessage) (value.Value, error) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 {
		return value.Value{}, d.fail("missing value")
	}
	switch raw[0] {
	case '"':
		var s string
		if json.Unmarshal(raw, &s) != nil {
			return value.Value{}, d.fail("bad text")
		}
		return value.Text(s), nil
	case 't', 'f':
		return value.Bool(raw[0] == 't'), nil
	case '[':
		var items []json.RawMessage
		if json.Unmarshal(raw, &items) != nil {
			return value.Value{}, d.fail("bad list")
		}
		vs := make([]value.Value, len(items))
		for i, it := range items {
			v, err := d.value(it)
			if err != nil {
				return v, err
			}
			vs[i] = v
		}
		return value.ListOf(vs), nil
	case '{':
		var o rawObj
		if json.Unmarshal(raw, &o) != nil {
			return value.Value{}, d.fail("bad object")
		}
		switch {
		case o["time"] != nil:
			n, err := strconv.ParseInt(string(o["time"]), 10, 64)
			if err != nil {
				return value.Value{}, d.fail("bad time")
			}
			return value.Time(n), nil
		case o["node"] != nil:
			n, err := d.node(o["node"])
			return value.Node(n), err
		case o["map"] != nil:
			var pairs [][2]json.RawMessage
			if json.Unmarshal(o["map"], &pairs) != nil {
				return value.Value{}, d.fail("bad map")
			}
			m := &value.Map{}
			for _, kv := range pairs {
				k, err := d.value(kv[0])
				if err != nil {
					return k, err
				}
				v, err := d.value(kv[1])
				if err != nil {
					return v, err
				}
				m = m.With(k, v)
			}
			return value.MapValue(m), nil
		case o["type"] != nil:
			var typ string
			if json.Unmarshal(o["type"], &typ) != nil {
				return value.Value{}, d.fail("bad record type")
			}
			r, err := d.recordOf(typ, o["fields"])
			if err != nil {
				return value.Value{}, err
			}
			return value.Value{K: value.KRecord, Ref: r}, nil
		}
		return value.Value{}, d.fail("unknown object value")
	}
	f, err := strconv.ParseFloat(string(raw), 64)
	if err != nil {
		return value.Value{}, d.fail("bad number %s", raw)
	}
	return value.Number(f), nil
}
