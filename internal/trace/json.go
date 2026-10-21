package trace

import (
	"math"
	"strconv"
	"unicode/utf8"

	"github.com/synodlang/compiler/internal/value"
)

// jw is the hand-written JSON writer: it appends to a byte slice and never
// reorders or reflects. Escaping follows the format spec: \" \\ \n \r \t,
// other bytes below 0x20 as \u00xx (lowercase hex), everything else raw
// UTF-8 (no HTML escaping). Invalid UTF-8 becomes U+FFFD.
type jw struct{ b []byte }

const hexDigits = "0123456789abcdef"

func (w *jw) raw(s string) { w.b = append(w.b, s...) }

func (w *jw) byte(c byte) { w.b = append(w.b, c) }

func (w *jw) str(s string) {
	w.b = append(w.b, '"')
	for i := 0; i < len(s); {
		c := s[i]
		if c >= utf8.RuneSelf {
			r, n := utf8.DecodeRuneInString(s[i:])
			if r == utf8.RuneError && n == 1 {
				w.b = append(w.b, "\xef\xbf\xbd"...)
			} else {
				w.b = append(w.b, s[i:i+n]...)
			}
			i += n
			continue
		}
		i++
		switch c {
		case '"':
			w.b = append(w.b, '\\', '"')
		case '\\':
			w.b = append(w.b, '\\', '\\')
		case '\n':
			w.b = append(w.b, '\\', 'n')
		case '\r':
			w.b = append(w.b, '\\', 'r')
		case '\t':
			w.b = append(w.b, '\\', 't')
		default:
			if c < 0x20 {
				w.b = append(w.b, '\\', 'u', '0', '0', hexDigits[c>>4], hexDigits[c&15])
			} else {
				w.b = append(w.b, c)
			}
		}
	}
	w.b = append(w.b, '"')
}

func (w *jw) int(n int64) { w.b = strconv.AppendInt(w.b, n, 10) }

func (w *jw) uint(n uint64) { w.b = strconv.AppendUint(w.b, n, 10) }

// key writes `"k":`; first says whether it is the first key of its object.
func (w *jw) key(k string, first bool) {
	if !first {
		w.b = append(w.b, ',')
	}
	w.str(k)
	w.b = append(w.b, ':')
}

// nodeName writes a node id as a JSON string "n3", or null for 0 (Nobody).
func (w *jw) nodeKey(id int) {
	if id == 0 {
		w.raw("null")
		return
	}
	w.str(value.NodeName(id))
}

// strs writes a list of strings.
func (w *jw) strs(ss []string) {
	w.byte('[')
	for i, s := range ss {
		if i > 0 {
			w.byte(',')
		}
		w.str(s)
	}
	w.byte(']')
}

// nodes writes a list of node names.
func (w *jw) nodes(ids []int) {
	w.byte('[')
	for i, id := range ids {
		if i > 0 {
			w.byte(',')
		}
		w.nodeKey(id)
	}
	w.byte(']')
}

type encodeError string

func (e encodeError) Error() string { return "trace: " + string(e) }

// value writes v in the trace encoding of the format spec.
func (e *encoder) value(v value.Value) error {
	w := &e.w
	switch v.K {
	case value.KText:
		w.str(v.Str)
	case value.KNumber:
		if math.IsNaN(v.Num) || math.IsInf(v.Num, 0) {
			return encodeError("non-finite number")
		}
		w.raw(value.FormatNumber(v.Num))
	case value.KBool:
		if v.Bool() {
			w.raw("true")
		} else {
			w.raw("false")
		}
	case value.KTime:
		w.raw(`{"time":`)
		w.int(v.Int)
		w.byte('}')
	case value.KNode:
		w.raw(`{"node":`)
		w.nodeKey(v.NodeID())
		w.byte('}')
	case value.KList:
		w.byte('[')
		for i, x := range v.List().Items {
			if i > 0 {
				w.byte(',')
			}
			if err := e.value(x); err != nil {
				return err
			}
		}
		w.byte(']')
	case value.KMap:
		m := v.Map()
		w.raw(`{"map":[`)
		for i := range m.Keys {
			if i > 0 {
				w.byte(',')
			}
			w.byte('[')
			if err := e.value(m.Keys[i]); err != nil {
				return err
			}
			w.byte(',')
			if err := e.value(m.Vals[i]); err != nil {
				return err
			}
			w.byte(']')
		}
		w.raw("]}")
	case value.KRecord:
		return e.record(v.Record())
	default:
		// Packages, fields and blocks cannot be exported state or message
		// content; write their Synod text so that Encode never fails on them.
		w.str(value.Format(v, value.FormatOpts{Qual: e.qual}))
	}
	return nil
}

// record writes {"type":"q.Name","fields":{…}}; unexported fields are "…".
func (e *encoder) record(r *value.Record) error {
	w := &e.w
	w.raw(`{"type":`)
	w.str(e.qual(r.Type.Pkg) + "." + r.Type.Name)
	w.raw(`,"fields":`)
	if err := e.fields(r); err != nil {
		return err
	}
	w.byte('}')
	return nil
}

// fields writes the object of the fields of r; unexported ones are "…".
func (e *encoder) fields(r *value.Record) error {
	w := &e.w
	w.byte('{')
	for i, f := range r.Fields {
		w.key(r.Type.Fields[i].Name, i == 0)
		if !r.Type.Fields[i].Exported {
			w.str("…")
			continue
		}
		if err := e.value(f); err != nil {
			return err
		}
	}
	w.byte('}')
	return nil
}
