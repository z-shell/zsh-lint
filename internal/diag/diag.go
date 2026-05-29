package diag

import "sort"

// Position is a single point in a source file. Line and Column are 1-based;
// Offset is a 0-based byte offset from the start of the file.
type Position struct {
	Line   int
	Column int
	Offset int
}

// IsValid reports whether the position has a set line. Line is 1-based, so a
// zero value is treated as unset; Column and Offset are not checked.
func (p Position) IsValid() bool { return p.Line > 0 }

// Range is a half-open span [Start, End) within a single file. The zero value
// is an invalid range, used for diagnostics with no specific location.
type Range struct {
	Start Position
	End   Position
}

// IsValid reports whether the range has a set start position. It does not
// validate End or require that Start precedes End.
func (r Range) IsValid() bool { return r.Start.IsValid() }

// RuleID is the stable identifier of the rule that produced a diagnostic. It is
// an opaque string; by convention rule IDs are hierarchical kebab slugs of the
// form "category/rule-name" (e.g. "quoting/unquoted-var"). The type does not
// validate the format, so an alternate scheme can be adopted later without a
// model change.
type RuleID string

// Diagnostic is one finding produced by the analyzer. It is pure data: no
// formatting behavior lives here (output formatting is issue #20). A zero Range
// means the diagnostic is unpositioned (whole-file or unknown location); an
// empty File means the source path is unknown.
type Diagnostic struct {
	RuleID   RuleID
	Severity Severity
	Message  string
	File     string
	Range    Range
}

// Diagnostics is an ordered collection of findings.
type Diagnostics []Diagnostic

// Sort orders diagnostics deterministically and independently of input order:
// by File, then by Range (Start then End, each Line, Column, Offset), then by
// RuleID, then by Severity, then by Message. Unpositioned diagnostics (zero
// range) sort before positioned ones within the same file.
func (d Diagnostics) Sort() {
	sort.SliceStable(d, func(i, j int) bool {
		a, b := d[i], d[j]
		if a.File != b.File {
			return a.File < b.File
		}
		// Unpositioned (invalid range) sorts before positioned within a file.
		if av, bv := a.Range.IsValid(), b.Range.IsValid(); av != bv {
			return !av
		}
		if a.Range.Start.Line != b.Range.Start.Line {
			return a.Range.Start.Line < b.Range.Start.Line
		}
		if a.Range.Start.Column != b.Range.Start.Column {
			return a.Range.Start.Column < b.Range.Start.Column
		}
		if a.Range.Start.Offset != b.Range.Start.Offset {
			return a.Range.Start.Offset < b.Range.Start.Offset
		}
		if a.Range.End.Line != b.Range.End.Line {
			return a.Range.End.Line < b.Range.End.Line
		}
		if a.Range.End.Column != b.Range.End.Column {
			return a.Range.End.Column < b.Range.End.Column
		}
		if a.Range.End.Offset != b.Range.End.Offset {
			return a.Range.End.Offset < b.Range.End.Offset
		}
		if a.RuleID != b.RuleID {
			return a.RuleID < b.RuleID
		}
		if a.Severity != b.Severity {
			return a.Severity < b.Severity
		}
		return a.Message < b.Message
	})
}
