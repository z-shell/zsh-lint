package parse

import (
	"bytes"
	"errors"
	"os"
	"testing"

	"mvdan.cc/sh/v3/syntax"
)

// Issue #522: an empty group inside a nested alternation is an error, and the
// nested-pattern adapter must not mask the `()` pair into an accepted source.
func TestEmptyGroupRejected(t *testing.T) {
	src, err := os.ReadFile("testdata/invalid-522-empty-group.txt")
	if err != nil {
		t.Fatal(err)
	}
	_, err = Parse(bytes.NewReader(src), "invalid-522.zsh")
	var pe syntax.ParseError
	if !errors.As(err, &pe) {
		t.Fatalf("error = %v, want syntax.ParseError", err)
	}
	if pe.Text != "not a valid test operator: `(`" || pe.Pos.Line() != 4 || pe.Pos.Col() != 15 {
		t.Fatalf("error = %v, want 4:15: not a valid test operator: `(`", err)
	}
	// Without the adapter's own check, a valid nested alternation elsewhere
	// runs the adapter, which masks the `()` pair into an accepted source.
	for _, tc := range []struct{ src, err string }{
		{"[[ x == (a|(b)|()) ]]\n", "1:15: not a valid test operator: `|`"},
		{"[[ x == (a|(b|c)) && y == a(()) ]]\n", "1:29: not a valid test operator: `(`"},
	} {
		if _, err := Parse(bytes.NewReader([]byte(tc.src)), ""); err == nil || err.Error() != tc.err {
			t.Fatalf("%q: error = %v, want %s", tc.src, err, tc.err)
		}
	}
}

// A group that is not empty still nests through the adapter.
func TestNonEmptyNestedGroupAccepted(t *testing.T) {
	for _, src := range []string{
		"[[ x == a(( )) ]]\n",
		"[[ x == a((b)) ]]\n",
		"[[ x == (a|(b|c)) ]]\n",
	} {
		if _, err := Parse(bytes.NewReader([]byte(src)), "ok-522.zsh"); err != nil {
			t.Fatalf("%q: %v", src, err)
		}
	}
}
