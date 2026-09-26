package parse

import (
	"bytes"
	"errors"
	"os"
	"testing"

	"mvdan.cc/sh/v3/syntax"
)

// Each source is rejected by `zsh -f -n`: Zsh keeps command position after an
// assignment prefix, so the reserved word that follows opens a construct that
// cannot follow an assignment (#278).
func TestParseRejectsReservedWordAfterAssignment(t *testing.T) {
	tests := []struct {
		fixture  string
		wantPos  string
		wantText string
	}{
		{"invalid-278-assignment-before-bang.txt", "5:5", "`!` cannot follow an assignment"},
		{"invalid-278-assignment-before-cond.txt", "4:5", "`[[` cannot follow an assignment"},
	}
	for _, test := range tests {
		t.Run(test.fixture, func(t *testing.T) {
			src, err := os.ReadFile("testdata/" + test.fixture)
			if err != nil {
				t.Fatalf("read fixture: %v", err)
			}
			_, err = Parse(bytes.NewReader(src), test.fixture)
			var perr syntax.ParseError
			if !errors.As(err, &perr) {
				t.Fatalf("Parse() error = %v, want syntax.ParseError", err)
			}
			if got := perr.Pos.String(); got != test.wantPos {
				t.Errorf("position = %s, want %s", got, test.wantPos)
			}
			if perr.Text != test.wantText {
				t.Errorf("text = %q, want %q", perr.Text, test.wantText)
			}
		})
	}
}
