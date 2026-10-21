package value

import (
	"math"
	"strconv"
	"strings"

	"github.com/synodlang/compiler/internal/ir"
	"github.com/synodlang/compiler/internal/lex"
)

// FormatNumber is the text of a finite number, as ECMAScript Number toString
// and Go's encoding/json write it: "0" for zero, positional for
// 1e-6 <= |f| < 1e21, otherwise an exponent without leading zeros
// ("1e-7", "1e+21").
func FormatNumber(f float64) string {
	if f == 0 {
		return "0"
	}
	abs := math.Abs(f)
	if abs >= 1e-6 && abs < 1e21 {
		return strconv.FormatFloat(f, 'f', -1, 64)
	}
	s := strconv.FormatFloat(f, 'e', -1, 64)
	// Turn "e-07" into "e-7".
	if n := len(s); n >= 4 && s[n-4] == 'e' && s[n-3] == '-' && s[n-2] == '0' {
		s = s[:n-2] + s[n-1:]
	}
	return s
}

// FormatTime is the canonical duration literal of us microseconds:
// "2s", "1500ms", "0.5ms", "0.001ms", "0s", "-1500ms".
func FormatTime(us int64) string {
	neg := us < 0
	u := uint64(us)
	if neg {
		u = -u
	}
	s := strconv.FormatUint(u/1000, 10)
	if frac := u % 1000; frac != 0 {
		f := strconv.FormatUint(frac+1000, 10)[1:]
		s += "." + strings.TrimRight(f, "0")
	}
	if neg && s != "0" {
		s = "-" + s
	}
	return lex.CanonicalDuration(s + "ms")
}

// FormatOpts controls Format.
type FormatOpts struct {
	// Qual names a package in record and field names; nil uses Package.Name.
	Qual func(*ir.Package) string
	// ShowUnexported prints unexported record fields instead of "…".
	ShowUnexported bool
	// TopText prints a text at the top level raw, without quotes (text.From).
	TopText bool
}

// Format is the Synod text of v: text quoted with \" \\ \n escapes (raw at
// top level with TopText), numbers by FormatNumber, times by FormatTime,
// nodes as n<id> or nobody, lists {a, b}, maps {k: v, k: v}, records
// paxos.Prepare{number 101} with "…" for unexported fields.
func Format(v Value, o FormatOpts) string {
	var b strings.Builder
	if v.K == KText && o.TopText {
		return v.Str
	}
	format(&b, v, o)
	return b.String()
}

func qualName(p *ir.Package, o FormatOpts) string {
	if o.Qual != nil {
		return o.Qual(p)
	}
	return p.Name
}

func format(b *strings.Builder, v Value, o FormatOpts) {
	switch v.K {
	case KText:
		b.WriteByte('"')
		for i := 0; i < len(v.Str); i++ {
			switch c := v.Str[i]; c {
			case '"':
				b.WriteString(`\"`)
			case '\\':
				b.WriteString(`\\`)
			case '\n':
				b.WriteString(`\n`)
			default:
				b.WriteByte(c)
			}
		}
		b.WriteByte('"')
	case KNumber:
		b.WriteString(FormatNumber(v.Num))
	case KBool:
		b.WriteString(strconv.FormatBool(v.Bool()))
	case KTime:
		b.WriteString(FormatTime(v.Int))
	case KNode:
		b.WriteString(NodeName(v.NodeID()))
	case KList:
		b.WriteByte('{')
		for i, x := range v.List().Items {
			if i > 0 {
				b.WriteString(", ")
			}
			format(b, x, o)
		}
		b.WriteByte('}')
	case KMap:
		m := v.Map()
		b.WriteByte('{')
		for i := range m.Keys {
			if i > 0 {
				b.WriteString(", ")
			}
			format(b, m.Keys[i], o)
			b.WriteString(": ")
			format(b, m.Vals[i], o)
		}
		b.WriteByte('}')
	case KRecord:
		r := v.Record()
		b.WriteString(qualName(r.Type.Pkg, o) + "." + r.Type.Name + "{")
		for i, f := range r.Fields {
			if i > 0 {
				b.WriteString(", ")
			}
			b.WriteString(r.Type.Fields[i].Name)
			b.WriteByte(' ')
			if !r.Type.Fields[i].Exported && !o.ShowUnexported {
				b.WriteString("…")
			} else {
				format(b, f, o)
			}
		}
		b.WriteByte('}')
	case KPackage:
		b.WriteString(v.Ref.(*ir.Package).Path)
	case KField:
		f := v.Ref.(*ir.Field)
		b.WriteString(qualName(f.Pkg, o) + "." + f.Name)
	case KBlock:
		b.WriteString("<block>")
	default:
		b.WriteString("<invalid>")
	}
}
