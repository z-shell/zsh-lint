package parse

import (
	"errors"
	"os"
	"strings"
	"testing"

	"mvdan.cc/sh/v3/syntax"
)

// What follows the closing `}` of a brace-form `while` or `for` on the same
// line belongs to the loop statement, as native Zsh reads it (#275). The
// `for` adapter's synthetic `done` once ended its line, so a separator or an
// operator there started a statement of its own.
func TestAlternateCloserSameLineTail(t *testing.T) {
	for _, src := range []string{
		"for x (a b) { print $x }; print hi",
		"for x (a b) { print $x } && print hi",
		"for x (a b) { print $x } || print hi",
		"for x (a b) { print $x } | cat",
		"for x (a b) { print $x } & print hi",
		"for x (a b) { print $x } # note",
		"while (( 0 )) { x=1 }; print hi",
		"while (( 0 )) { x=1 } && print hi",
		"if (( 1 )) { x=1 }; print hi",
		"if (( 1 )) { x=1 } else { x=2 }; print hi",
	} {
		if _, err := Parse(strings.NewReader(src+"\n"), "t.zsh"); err != nil {
			t.Errorf("%q: %v", src, err)
		}
	}
}

// A redirect after the closer is the loop's own redirect, at its source
// position, and no statement without a command appears.
func TestAlternateCloserTailRedirect(t *testing.T) {
	for _, tt := range []struct {
		src, pos string
	}{
		{"for x (a b) { print $x } > out", "1:26"},
		{"while (( 0 )) { x=1 } > out", "1:23"},
	} {
		file, err := Parse(strings.NewReader(tt.src+"\n"), "t.zsh")
		if err != nil {
			t.Errorf("%q: %v", tt.src, err)
			continue
		}
		stmts := file.AST().Stmts
		if len(stmts) != 1 || stmts[0].Cmd == nil {
			t.Errorf("%q: want one statement with a command, got %d", tt.src, len(stmts))
			continue
		}
		if redirs := stmts[0].Redirs; len(redirs) != 1 || redirs[0].OpPos.String() != tt.pos {
			t.Errorf("%q: loop redirects = %v, want one at %s", tt.src, redirs, tt.pos)
		}
	}
}

// After the brace-form `if` closer native Zsh accepts only a separator or a
// newline, so a redirect or an operator there stays an error.
func TestAlternateIfTailStaysInvalid(t *testing.T) {
	for _, src := range []string{
		"if (( 1 )) { x=1 } 2>&1",
		"if (( 1 )) { x=1 } && print hi",
		"if (( 1 )) { x=1 } | cat",
	} {
		var parseErr syntax.ParseError
		if _, err := Parse(strings.NewReader(src+"\n"), "t.zsh"); !errors.As(err, &parseErr) {
			t.Errorf("%q: error = %v, want a parse error", src, err)
		}
	}
	src, err := os.ReadFile("testdata/invalid-275-alternate-if-tail-redirect.txt")
	if err != nil {
		t.Fatal(err)
	}
	var parseErr syntax.ParseError
	if _, err := Parse(strings.NewReader(string(src)), "invalid-275.zsh"); !errors.As(err, &parseErr) {
		t.Fatalf("Parse() error = %v, want a parse error", err)
	}
	if got := parseErr.Pos.String(); got != "2:20" {
		t.Errorf("error position = %s, want 2:20, at the tail", got)
	}
}
