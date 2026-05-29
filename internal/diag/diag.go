package diag

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
