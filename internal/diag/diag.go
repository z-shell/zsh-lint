package diag

import "sort"

// Position is a single point in a source file. Line and Column are 1-based;
// Offset is a 0-based byte offset from the start of the file.
type Position struct {
	Line   int
	Column int
	Offset int
}

// IsValid reports whether the position refers to a real location.
func (p Position) IsValid() bool { return p.Line > 0 }

// Range is a half-open span [Start, End) within a single file. The zero value
// is an invalid range, used for diagnostics with no specific location.
type Range struct {
	Start Position
	End   Position
}

// IsValid reports whether the range refers to a real span.
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

// Sort orders diagnostics deterministically: by File, then by Range.Start
// (Line, Column, Offset), then by RuleID. Unpositioned diagnostics (zero range)
// sort before positioned ones within the same file.
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
		return a.RuleID < b.RuleID
	})
}
