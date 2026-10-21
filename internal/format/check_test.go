package format

import (
	"testing"

	"github.com/synodlang/compiler/internal/source"
)

func TestCheckMessages(t *testing.T) {
	tests := []struct {
		name, src string
		line, col int
		msg, hint string
	}{
		{"bom", "\xef\xbb\xbfa\n", 1, 1, "byte order mark at start of file", ""},
		{"crlf", "define a\r\n  b\r\n", 1, 9, "line ends with CR LF", "use LF line endings"},
		{"trailing", "define a\n  b  \n", 2, 4, "trailing spaces", ""},
		{"extra blank", "define a\n\n  b\n", 2, 1, "unexpected blank line", ""},
		{"missing blank", "# package x\nuse a/b\ndefine c\n", 3, 1, "missing blank line", ""},
		{"no newline", "define a\n  b", 2, 4, "missing newline at end of file", ""},
		{"blank at end", "a\n\n", 3, 1, "unexpected blank line", ""},
		{"not canonical", "define a\n  b  c\n", 2, 5, "line is not canonical", `write "  b c"`},
		{"spaces in parens", "a ( b c )\n", 1, 4, "line is not canonical", `write "a (b c)"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := Check(source.NewFile("x.synod", []byte(tt.src)))
			if d == nil {
				t.Fatal("canonical")
			}
			if d.Line != tt.line || d.Col != tt.col || d.Message != tt.msg || d.Hint != tt.hint || d.Code != source.NonCanonical {
				t.Errorf("got %+v", *d)
			}
		})
	}
}

func TestCheckParseError(t *testing.T) {
	d := Check(source.NewFile("x.synod", []byte("a x-1\n")))
	if d == nil || d.Code != source.Syntax {
		t.Fatalf("got %v", d)
	}
}

func TestUseWithoutPath(t *testing.T) {
	if out, d := Format(source.NewFile("x.synod", []byte("use(\"\")"))); d == nil || out != nil {
		t.Fatalf("got %q, %v", out, d)
	}
}
