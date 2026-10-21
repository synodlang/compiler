package source

// Span is a range of source text on the positions of one file. End is
// exclusive and counts code points; Start == End is an empty span that marks
// one column (a caret).
type Span struct{ Start, End Pos }

// At returns the empty span at p.
func At(p Pos) Span { return Span{p, p} }

// Label is a secondary label: a span in any file with a short text.
type Label struct {
	File string `json:"file"`
	Line int    `json:"line"`
	Col  int    `json:"col"`
	End  Pos    `json:"end"`
	Text string `json:"text"`
}

// Edit replaces the text from (Line, Col) to End, exclusive, with New, in the
// file of the diagnostic that holds it. An edit spans one line, except that
// an insertion at column 1 whose New ends in "\n" inserts whole lines, and
// that an edit from column 1 to column 1 of the next line deletes a line.
type Edit struct {
	Line int    `json:"line"`
	Col  int    `json:"col"`
	End  Pos    `json:"end"`
	New  string `json:"new"`
}

// Help is advice, with the optional machine-applicable edits of one fix.
type Help struct {
	Text  string `json:"text"`
	Edits []Edit `json:"edits,omitempty"`
}

// Extra adds optional rich content to a diagnostic under construction.
type Extra func(*Diagnostic)

// WithLabel sets the text under the primary span.
func WithLabel(text string) Extra {
	return func(d *Diagnostic) { d.Label = text }
}

// Also adds a secondary label at sp in file (a display path).
func Also(file string, sp Span, text string) Extra {
	return func(d *Diagnostic) {
		d.Labels = append(d.Labels, Label{File: file, Line: sp.Start.Line, Col: sp.Start.Col, End: sp.End, Text: text})
	}
}

// HelpText adds help without an edit.
func HelpText(text string) Extra {
	return func(d *Diagnostic) { d.Help = append(d.Help, Help{Text: text}) }
}

// HelpEdit adds help with the edits that apply it.
func HelpEdit(text string, edits ...Edit) Extra {
	return func(d *Diagnostic) { d.Help = append(d.Help, Help{Text: text, Edits: edits}) }
}

// Note adds a statement of fact.
func Note(text string) Extra {
	return func(d *Diagnostic) { d.Notes = append(d.Notes, text) }
}

// EditAt returns the edit that replaces sp with new.
func EditAt(sp Span, new string) Edit {
	return Edit{Line: sp.Start.Line, Col: sp.Start.Col, End: sp.End, New: new}
}
