// Package diag defines the analyzer's in-memory diagnostic model: the stable
// types that rules, suppressions, and output renderers all share.
//
// The package imports nothing outside the standard library. That keeps the
// model both renderer-agnostic (text/JSON formatting lives elsewhere and reuses
// these types) and parser-agnostic (the front end can be swapped — see issue
// #17 — with the syntax.Pos → diag.Position adapter living at the parser
// boundary, not here).
package diag

// Severity is the advisory level of a diagnostic. Values are ordered
// Info < Warning < Error so callers can threshold on them (for example, fail a
// run at Warning or above).
type Severity int

const (
	// Info is advisory output that does not indicate a problem.
	Info Severity = iota
	// Warning flags a likely issue that does not block.
	Warning
	// Error flags a problem that should fail a run.
	Error
)

// String returns the lowercase name of the severity, or "unknown" for a value
// outside the defined vocabulary.
func (s Severity) String() string {
	switch s {
	case Info:
		return "info"
	case Warning:
		return "warning"
	case Error:
		return "error"
	default:
		return "unknown"
	}
}

// Position is a single point in a source file. Line and Col are 1-based; Col
// counts bytes from the start of the line, matching the parser front end.
// Offset is the 0-based byte offset from the start of the file. A zero Line
// means the position is unknown.
type Position struct {
	Line   uint
	Col    uint
	Offset uint
}

// Valid reports whether the position refers to a real location, i.e. its Line
// is set.
func (p Position) Valid() bool {
	return p.Line > 0
}

// Range is a half-open source span [Start, End): Start is the first covered
// position and End is the position just past the last covered byte.
type Range struct {
	Start Position
	End   Position
}

// RuleID is a stable, greppable identifier for the rule that produced a
// diagnostic (for example "ZL0001").
type RuleID string

// Diagnostic is one analyzer finding. It is self-contained: it carries its own
// file path so a renderer needs no external lookup to report it.
type Diagnostic struct {
	Path     string
	Range    Range
	Severity Severity
	Rule     RuleID
	Message  string
}
