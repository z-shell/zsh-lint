package parse

import (
	"bytes"
	"errors"
	"os"
	"testing"

	"mvdan.cc/sh/v3/syntax"
)

// Issue #519: the body of a command substitution inside a glob group is
// parsed as commands, so an invalid one is reported where it stands.
func TestGroupSubstBodyRejected(t *testing.T) {
	src, err := os.ReadFile("testdata/invalid-519-group-subst-body.txt")
	if err != nil {
		t.Fatal(err)
	}
	_, err = Parse(bytes.NewReader(src), "invalid-519.zsh")
	var pe syntax.ParseError
	if !errors.As(err, &pe) {
		t.Fatalf("error = %v, want syntax.ParseError", err)
	}
	if pe.Text != "`done` can only be used to end a loop" || pe.Pos.Line() != 4 || pe.Pos.Col() != 11 {
		t.Fatalf("error = %v, want 4:11: `done` can only be used to end a loop", err)
	}
}

// The nested conditional-pattern adapter masks a nested group into a
// retry; a substitution body in such a pattern is still checked.
func TestGroupSubstBodyInNestedPattern(t *testing.T) {
	for _, tc := range []struct{ src, err string }{
		{"[[ x == (a|(b|$(done))) ]]\n", "1:17: `done` can only be used to end a loop"},
		{"[[ x == (a|(b|$(echo ok))) ]]\n", ""},
	} {
		_, err := Parse(bytes.NewReader([]byte(tc.src)), "")
		got := ""
		if err != nil {
			got = err.Error()
		}
		if got != tc.err {
			t.Fatalf("%q: error = %q, want %q", tc.src, got, tc.err)
		}
	}
}
