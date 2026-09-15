package parse

import (
	"errors"
	"strings"
	"testing"

	"mvdan.cc/sh/v3/syntax"
)

// The front end has no node for repeat or foreach. mvdan/sh v3.14.1 parses
// both as ordinary calls, so these sources would otherwise yield a tree of the
// wrong shape with no error. Each row is `zsh -f -n` valid.
func TestParseRejectsUnsupportedLoopWords(t *testing.T) {
	tests := []struct {
		name     string
		src      string
		wantLine uint
		wantCol  uint
		wantText string
	}{
		{"repeat short form", "repeat 3; print hi\n", 1, 1, "z-shell/zsh-lint#208"},
		{"repeat nested short form", "if true; then\n  repeat 2; print hi\nfi\n", 2, 3, "z-shell/zsh-lint#208"},
		{"foreach end form", "foreach v ($a)\n  cmd $v\nend\n", 1, 1, "z-shell/zsh-lint#214"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := Parse(strings.NewReader(test.src), test.name+".zsh")
			var perr syntax.ParseError
			if !errors.As(err, &perr) {
				t.Fatalf("Parse() error = %v, want syntax.ParseError", err)
			}
			if perr.Pos.Line() != test.wantLine || perr.Pos.Col() != test.wantCol {
				t.Errorf("position = %d:%d, want %d:%d", perr.Pos.Line(), perr.Pos.Col(), test.wantLine, test.wantCol)
			}
			if !strings.Contains(perr.Text, test.wantText) {
				t.Errorf("text = %q, want reference to %s", perr.Text, test.wantText)
			}
		})
	}
}

// The do and brace forms already fail in the parser itself; the guard only
// has to cover the forms that would otherwise parse silently.
func TestParseStillRejectsLoudRepeatForms(t *testing.T) {
	for _, src := range []string{
		"repeat 3; do print hi; done\n",
		"repeat 3 { print hi }\n",
	} {
		if _, err := Parse(strings.NewReader(src), "loud.zsh"); err == nil {
			t.Errorf("Parse(%q) error = nil, want a parse error", src)
		}
	}
}

// The guard keys on the command name only: a quoted word, an argument, or a
// function named after the reserved word is not a loop.
func TestParseKeepsOrdinaryUsesOfLoopWords(t *testing.T) {
	for _, src := range []string{
		"print repeat foreach\n",
		"\\repeat 3\n",
		"x=repeat\n",
		"command repeat\n",
	} {
		if _, err := Parse(strings.NewReader(src), "ordinary.zsh"); err != nil {
			t.Errorf("Parse(%q) error = %v, want nil", src, err)
		}
	}
}
