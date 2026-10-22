package resolve

import (
	"strings"

	"github.com/synodlang/compiler/internal/types"
)

// HoleType prints the type of a hole as a reader in the package with import
// path from writes it: its own types bare, the types of other packages with
// the qualifier that qual gives (the last segment of the path when qual is
// nil or returns "").
func (p *Param) HoleType(from string, qual func(pkgPath string) string) string {
	if p.Kind == ParamBlock {
		return "block"
	}
	if p.Type == nil {
		return p.Source
	}
	return types.Format(p.Type, from, qual)
}

// DescribeAs prints the pattern like Describe, with the head qualified by
// headQual ("" for none) and the types of the holes spelled for a reader in
// the package from (see HoleType). With from equal to the phrase's own
// package it is Describe.
func (p *Phrase) DescribeAs(headQual, from string, qual func(pkgPath string) string) string {
	var b strings.Builder
	if p.Receiver != nil {
		b.WriteString("[" + p.Receiver.Name + ": " + p.Receiver.HoleType(from, qual) + "] ")
	}
	for i, it := range p.Items {
		if i > 0 {
			b.WriteByte(' ')
		}
		switch {
		case it.Param != nil:
			b.WriteString("[" + it.Param.Name + ": " + it.Param.HoleType(from, qual) + "]")
		case i == 0 && headQual != "":
			b.WriteString(headQual + "." + it.Word)
		default:
			b.WriteString(it.Word)
		}
	}
	for _, bs := range p.Blocks {
		if bs.Word != "" {
			b.WriteString(" " + bs.Word)
		}
		b.WriteString(" [" + bs.Param.Name + ": block]")
	}
	return b.String()
}
