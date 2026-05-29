package diag

import "testing"

func TestSeverityString(t *testing.T) {
	tests := []struct {
		name string
		sev  Severity
		want string
	}{
		{"info", Info, "info"},
		{"warning", Warning, "warning"},
		{"error", Error, "error"},
		{"unknown", Severity(99), "unknown"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.sev.String(); got != tt.want {
				t.Errorf("Severity(%d).String() = %q, want %q", tt.sev, got, tt.want)
			}
		})
	}
}

// The severity vocabulary is ordered so callers can compare levels
// (e.g. "fail the build at Warning or above"). Lock the ordering.
func TestSeverityOrdering(t *testing.T) {
	if !(Info < Warning && Warning < Error) {
		t.Fatalf("severity ordering broken: want Info(%d) < Warning(%d) < Error(%d)",
			Info, Warning, Error)
	}
}

func TestPositionValid(t *testing.T) {
	tests := []struct {
		name string
		pos  Position
		want bool
	}{
		{"first line is valid", Position{Line: 1, Col: 1, Offset: 0}, true},
		{"zero value is invalid", Position{}, false},
		{"offset without line is invalid", Position{Offset: 42}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.pos.Valid(); got != tt.want {
				t.Errorf("Position%+v.Valid() = %v, want %v", tt.pos, got, tt.want)
			}
		})
	}
}

// A Diagnostic is a plain value: every field is set by the producer and read
// back by a renderer with no external lookup. This proves the model is usable
// and self-contained (no parser or renderer dependency).
func TestDiagnosticValue(t *testing.T) {
	d := Diagnostic{
		Path:     "plugin.zsh",
		Range:    Range{Start: Position{Line: 3, Col: 5, Offset: 40}, End: Position{Line: 3, Col: 9, Offset: 44}},
		Severity: Warning,
		Rule:     RuleID("ZL0001"),
		Message:  "example finding",
	}

	if d.Path != "plugin.zsh" {
		t.Errorf("Path = %q, want %q", d.Path, "plugin.zsh")
	}
	if d.Severity != Warning {
		t.Errorf("Severity = %v, want %v", d.Severity, Warning)
	}
	if d.Rule != RuleID("ZL0001") {
		t.Errorf("Rule = %q, want %q", d.Rule, "ZL0001")
	}
	if d.Message != "example finding" {
		t.Errorf("Message = %q, want %q", d.Message, "example finding")
	}
	if !d.Range.Start.Valid() || !d.Range.End.Valid() {
		t.Errorf("Range endpoints should be valid, got %+v", d.Range)
	}
	if d.Range.Start.Offset != 40 || d.Range.End.Offset != 44 {
		t.Errorf("Range offsets = %d..%d, want 40..44", d.Range.Start.Offset, d.Range.End.Offset)
	}
}
