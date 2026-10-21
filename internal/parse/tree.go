// Package parse turns the tokens of a Synod file into a syntax tree
// (spec/02 section 2.1) that keeps every comment, so that the formatter can
// print the file back.
//
// The tree is purely structural: it knows lines, blocks, sentences and items,
// and nothing about what the words mean.
//
// # Comments
//
// Every own-line comment ends up in exactly one place:
//
//   - File.Header, for a leading "# package <name>" comment;
//   - Line.Leading or Continuation.Leading, when it stands directly above a
//     line (blank lines inside a block, and between a block and its
//     continuation lines, are ignored);
//   - Block.Trailing, when it follows the last line of a block and is
//     indented at least as deep as that block's lines;
//   - a CommentGroup in File.Body, for a top-level run of comments cut off
//     from the line below by a blank line;
//   - File.Trailing, for comments after the last top-level line.
//
// A comment after a sentence on the same line is Line.Comment or
// Continuation.Comment.
//
// # Continuation lines
//
// A line holding one bare word and a block, directly after a line that has a
// block (blank and comment lines may sit between them), is not a Line of its
// own: it is appended to the previous line's Conts, so that "if" and "else"
// form one Line.
package parse

import "github.com/synodlang/compiler/internal/source"

// File is a parsed source file.
type File struct {
	Source *source.File
	// Header is the first non-blank line when it is an own-line comment
	// whose text starts with "package ".
	Header *Comment
	// Package is the name in the header when the header is exactly
	// "# package <word>", and "" otherwise (also when there is no header).
	Package string
	// Body holds the top level in source order: *Line or *CommentGroup.
	Body []Node
	// Trailing holds the own-line comments after the last top-level line.
	Trailing []*Comment
}

// Node is an element of File.Body: a *Line or a *CommentGroup.
type Node interface{ node() }

// CommentGroup is a run of top-level own-line comments that a blank line
// separates from the line below them.
type CommentGroup struct{ Comments []*Comment }

// Comment is one comment.
type Comment struct {
	Pos source.Pos
	// Text is the comment after "#" and one space, right-trimmed. A bare "#"
	// has the empty text.
	Text string
	// Blank reports that a blank line lies directly above the comment.
	Blank bool
}

// Line is a statement or declaration: a sentence with an optional trailing
// comment and an optional block, followed by continuation lines.
type Line struct {
	Pos      source.Pos
	Leading  []*Comment      // own-line comments directly above the line
	Sentence *Sentence       // never nil
	Comment  *Comment        // trailing comment on the sentence's line
	Block    *Block          // nil if the line has none
	Conts    []*Continuation // only when Block != nil
}

// Continuation is a continuation line such as "else": a bare word with a
// block, belonging to the Line it follows.
type Continuation struct {
	Leading []*Comment
	Word    *Word
	Comment *Comment
	Block   *Block // never nil
}

// Block is an indented run of lines.
type Block struct {
	Lines []*Line // at least one
	// Trailing holds the own-line comments after the last line that are
	// indented at least as deep as the block's lines.
	Trailing []*Comment
}

// Sentence is a comma separated list of groups of items.
type Sentence struct {
	// Groups has at least one group, each with at least one item.
	Groups [][]Item
}

// Item is one element of a sentence: the unit an inline hole receives.
type Item interface {
	Pos() source.Pos
	item()
}

// Word is a bare name.
type Word struct {
	P    source.Pos
	Name string
}

// Member is a dotted name such as n.Known.
type Member struct {
	P     source.Pos
	Parts []string // at least two
}

// Number is a number literal, as written.
type Number struct {
	P    source.Pos
	Text string
}

// Duration is a duration literal, as written.
type Duration struct {
	P    source.Pos
	Text string
}

// Text is a text literal, quotes included, as written.
type Text struct {
	P   source.Pos
	Raw string
}

// Color is a color literal, as written.
type Color struct {
	P    source.Pos
	Text string
}

// Path is the import path of a "use" line.
type Path struct {
	P    source.Pos
	Text string
}

// Paren is a parenthesized sentence.
type Paren struct {
	P source.Pos
	S *Sentence
}

// List is a list literal "{a, b}".
type List struct {
	P     source.Pos
	Items []Item
}

// Hole is a typed hole "[name: type]".
type Hole struct {
	P    source.Pos
	Name *Word
	Type Type
}

// Type is the type of a hole.
type Type interface {
	Pos() source.Pos
	typ()
}

// NamedType is a type name: "Node", "raft.Node" or "block".
type NamedType struct {
	P    source.Pos
	Name string
}

// ListType is "list of T".
type ListType struct {
	P    source.Pos
	Elem Type
}

// MapType is "map of K to V".
type MapType struct {
	P          source.Pos
	Key, Value Type
}

func (*Line) node()         {}
func (*CommentGroup) node() {}

func (*Word) item()     {}
func (*Member) item()   {}
func (*Number) item()   {}
func (*Duration) item() {}
func (*Text) item()     {}
func (*Color) item()    {}
func (*Path) item()     {}
func (*Paren) item()    {}
func (*List) item()     {}
func (*Hole) item()     {}

// Pos returns the position of the item.
func (i *Word) Pos() source.Pos     { return i.P }
func (i *Member) Pos() source.Pos   { return i.P }
func (i *Number) Pos() source.Pos   { return i.P }
func (i *Duration) Pos() source.Pos { return i.P }
func (i *Text) Pos() source.Pos     { return i.P }
func (i *Color) Pos() source.Pos    { return i.P }
func (i *Path) Pos() source.Pos     { return i.P }
func (i *Paren) Pos() source.Pos    { return i.P }
func (i *List) Pos() source.Pos     { return i.P }
func (i *Hole) Pos() source.Pos     { return i.P }

func (*NamedType) typ() {}
func (*ListType) typ()  {}
func (*MapType) typ()   {}

// Pos returns the position of the type.
func (t *NamedType) Pos() source.Pos { return t.P }
func (t *ListType) Pos() source.Pos  { return t.P }
func (t *MapType) Pos() source.Pos   { return t.P }

// IsWord reports whether it is the bare word name.
func IsWord(it Item, name string) bool {
	w, ok := it.(*Word)
	return ok && w.Name == name
}
