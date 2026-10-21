package pipeline

import (
	"bytes"
	"encoding/json"
	"io"
	"sort"

	"github.com/synodlang/compiler/internal/driver"
	"github.com/synodlang/compiler/internal/load"
	"github.com/synodlang/compiler/internal/resolve"
	"github.com/synodlang/compiler/internal/source"
)

// stdFiles returns the display paths of the std files in the graph.
func stdFiles(g *load.Graph) map[string]bool {
	out := map[string]bool{}
	if g == nil {
		return out
	}
	for _, p := range g.Packages {
		if p.IsStd {
			for _, f := range p.Files {
				out[f.Src.Path] = true
			}
		}
	}
	return out
}

// UserDiags returns the diagnostics of the files that are not std.
func UserDiags(res driver.Result) []source.Diagnostic {
	std := stdFiles(res.Graph)
	var out []source.Diagnostic
	for _, d := range res.Diags {
		if !std[d.File] {
			out = append(out, d)
		}
	}
	return out
}

type tokenDef struct {
	File string `json:"file"`
	Line int    `json:"line"`
	Col  int    `json:"col"`
}

type tokenJSON struct {
	Line int       `json:"line"`
	Col  int       `json:"col"`
	Len  int       `json:"len"`
	Role string    `json:"role"`
	Def  *tokenDef `json:"def,omitempty"`
}

type fileTokens struct {
	File   string      `json:"file"`
	Tokens []tokenJSON `json:"tokens"`
}

type checkTokensJSON struct {
	Format      string              `json:"format"`
	OK          bool                `json:"ok"`
	Diagnostics []source.Diagnostic `json:"diagnostics"`
	Files       []fileTokens        `json:"files"`
}

// WriteCheckJSON writes check.json (04-formats.md) for diags, the user
// diagnostics of res. With tokens the document has the "files" key.
func WriteCheckJSON(w io.Writer, diags []source.Diagnostic, res driver.Result, tokens bool) error {
	if !tokens {
		return source.WriteJSON(w, diags)
	}
	if diags == nil {
		diags = []source.Diagnostic{}
	}
	std := stdFiles(res.Graph)
	out := checkTokensJSON{Format: "synod-check/1", OK: len(diags) == 0, Diagnostics: diags, Files: []fileTokens{}}
	for path, toks := range res.Tokens {
		if std[path] {
			continue
		}
		ft := fileTokens{File: path, Tokens: make([]tokenJSON, 0, len(toks))}
		sorted := append([]resolve.Token(nil), toks...)
		sort.SliceStable(sorted, func(i, j int) bool {
			if sorted[i].Line != sorted[j].Line {
				return sorted[i].Line < sorted[j].Line
			}
			return sorted[i].Col < sorted[j].Col
		})
		for _, t := range sorted {
			tj := tokenJSON{Line: t.Line, Col: t.Col, Len: t.Len, Role: string(t.Role)}
			if t.Def != nil {
				tj.Def = &tokenDef{File: t.Def.File, Line: t.Def.Pos.Line, Col: t.Def.Pos.Col}
			}
			ft.Tokens = append(ft.Tokens, tj)
		}
		out.Files = append(out.Files, ft)
	}
	sort.Slice(out.Files, func(i, j int) bool { return out.Files[i].File < out.Files[j].File })
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(out); err != nil {
		return err
	}
	_, err := w.Write(buf.Bytes())
	return err
}
