package diag

import "testing"

func TestPositionIsValid(t *testing.T) {
	if (Position{Line: 0, Column: 1}).IsValid() {
		t.Error("Position with Line 0 should be invalid")
	}
	if !(Position{Line: 1, Column: 1, Offset: 0}).IsValid() {
		t.Error("Position with Line 1 should be valid")
	}
}

func TestRangeIsValid(t *testing.T) {
	var zero Range
	if zero.IsValid() {
		t.Error("zero Range should be invalid")
	}
	r := Range{Start: Position{Line: 2, Column: 3}, End: Position{Line: 2, Column: 9}}
	if !r.IsValid() {
		t.Error("Range with valid Start should be valid")
	}
}

func TestDiagnosticConstruction(t *testing.T) {
	d := Diagnostic{
		RuleID:   "quoting/unquoted-var",
		Severity: Warning,
		Message:  "variable used unquoted",
		File:     "plugin.zsh",
		Range:    Range{Start: Position{Line: 4, Column: 7, Offset: 42}},
	}
	if d.RuleID != "quoting/unquoted-var" {
		t.Errorf("RuleID = %q", d.RuleID)
	}
	if d.Severity != Warning {
		t.Errorf("Severity = %d; want %d", int(d.Severity), int(Warning))
	}
	if !d.Range.IsValid() {
		t.Error("expected a valid range")
	}
}

func TestDiagnosticUnpositioned(t *testing.T) {
	d := Diagnostic{RuleID: "meta/parse-error", Severity: Error, Message: "cannot parse file"}
	if d.Range.IsValid() {
		t.Error("zero-range diagnostic should be unpositioned")
	}
}

func TestDiagnosticsSortDeterministic(t *testing.T) {
	// Intentionally unsorted: different files, positions, an unpositioned one,
	// and two at the same position differing only by RuleID.
	in := Diagnostics{
		{RuleID: "b", File: "z.zsh", Range: Range{Start: Position{Line: 1, Column: 1}}},
		{RuleID: "a", File: "z.zsh", Range: Range{Start: Position{Line: 1, Column: 1}}},
		{RuleID: "early", File: "a.zsh", Range: Range{Start: Position{Line: 5, Column: 2}}},
		{RuleID: "wholefile", File: "a.zsh"}, // unpositioned
	}
	in.Sort()

	wantOrder := []RuleID{"wholefile", "early", "a", "b"}
	if len(in) != len(wantOrder) {
		t.Fatalf("length changed: got %d, want %d", len(in), len(wantOrder))
	}
	for i, want := range wantOrder {
		if in[i].RuleID != want {
			t.Errorf("position %d: got RuleID %q, want %q", i, in[i].RuleID, want)
		}
	}
}

func TestDiagnosticsSortStableEmpty(t *testing.T) {
	var d Diagnostics
	d.Sort() // must not panic on nil/empty
	if len(d) != 0 {
		t.Errorf("empty Sort changed length to %d", len(d))
	}
}
