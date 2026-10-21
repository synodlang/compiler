package source

// Code is a stable diagnostic code; the full list is in spec/11 section 11.2.
type Code string

// Compile-time diagnostic codes (spec/11 section 11.2).
const (
	Syntax           Code = "syntax"
	Indent           Code = "indent"
	NonCanonical     Code = "non-canonical"
	BadHeader        Code = "bad-header"
	UnknownPhrase    Code = "unknown-phrase"
	AmbiguousPhrase  Code = "ambiguous-phrase"
	DuplicateName    Code = "duplicate-name"
	Shadowing        Code = "shadowing"
	ReservedWord     Code = "reserved-word"
	BadPattern       Code = "bad-pattern"
	BadReceiver      Code = "bad-receiver"
	BadPlace         Code = "bad-place"
	TypeMismatch     Code = "type-mismatch"
	AmbiguousEmpty   Code = "ambiguous-empty"
	MissingResult    Code = "missing-result"
	ValueAsStatement Code = "value-as-statement"
	ActionAsValue    Code = "action-as-value"
	Unexported       Code = "unexported"
	UnusedName       Code = "unused-name"
	UnusedImport     Code = "unused-import"
	UnneededAlias    Code = "unneeded-alias"
	UnknownImport    Code = "unknown-import"
	ImportCycle      Code = "import-cycle"
	MissingMain      Code = "missing-main"
	ExtraMain        Code = "extra-main"
	Context          Code = "context"
	ForeignState     Code = "foreign-state"
	ColorLiteral     Code = "color-literal"
	NativeOutsideStd Code = "native-outside-std"
)
