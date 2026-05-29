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
