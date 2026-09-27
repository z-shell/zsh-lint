package syntax_test

import (
	"strings"
	"testing"

	"mvdan.cc/sh/v3/syntax"
)

// Issue #522. Zsh ends a word at a `(` directly followed by `)` and reads
// `()` as a token of its own (the e == ')' test at LX2_INPAR in gettokstr,
// Src/lex.c), so an empty group inside a `[[ ]]` operand is an error.
// Native verdicts use newline-terminated files and zsh -f -n.
func TestZshEmptyGroupReject(t *testing.T) {
	for _, tc := range []struct{ src, err string }{
		{"[[ x == a(()) ]]", "1:11: not a valid test operator: `(`"},
		{"[[ x == (()) ]]", "1:10: not a valid test operator: `(`"},
		{"[[ x == a(b()c) ]]", "1:12: not a valid test operator: `(`"},
		{"[[ x == a(b|()) ]]", "1:13: not a valid test operator: `(`"},
		{"[[ x == a(\"\"()) ]]", "1:13: not a valid test operator: `(`"},
		{"[[ x == *(()) ]]", "1:11: not a valid test operator: `(`"},
		{"[[ a(() == x ]]", "1:6: not a valid test operator: `(`"},
		{"f() { [[ x == a(()) ]]; }", "1:17: not a valid test operator: `(`"},
		// The `=~` operand has its own reader, where `()` ends the word
		// at any depth.
		{"[[ x =~ a(()) ]]", "1:11: not a valid test operator: `(`"},
		{"[[ x =~ a() ]]", "1:10: not a valid test operator: `(`"},
		{"[[ x =~ () ]]", "1:9: not a valid test operator: `(`"},
		{"[[ x =~ a(b)() ]]", "1:13: not a valid test operator: `(`"},
		{"[[ x =~ a(() && y ]]", "1:11: not a valid test operator: `(`"},
		// Operands of a `-NAME` condition use the condition group reader.
		{"[[ -n a(()) ]]", "1:9: a condition glob group cannot contain `()`"},
		{"[[ -n a(b()c) ]]", "1:10: a condition glob group cannot contain `()`"},
		{"[[ a -foo b(()) ]]", "1:13: a condition glob group cannot contain `()`"},
	} {
		t.Run(tc.src, func(t *testing.T) {
			_, err := syntax.NewParser(syntax.Variant(syntax.LangZsh)).Parse(strings.NewReader(tc.src+"\n"), "")
			if err == nil || err.Error() != tc.err {
				t.Fatalf("error = %v, want %s", err, tc.err)
			}
		})
	}
}

func TestZshEmptyGroupAccept(t *testing.T) {
	for _, src := range []string{
		// `a(( ))` and `a((b))` parse only through the nested-pattern
		// adapter; internal/parse covers them.
		"[[ x == a('()') ]]",
		"[[ x == a(\"()\") ]]",
		"[[ x == a(\\(\\)) ]]",
		"[[ x == a(${x:-()}) ]]",
		"[[ x == a($(())) ]]",
		"[[ x == a(<()) ]]",
		"[[ -n a(( )) ]]",
		"[[ -n a(<()) ]]",
		// An empty process substitution is not a `()` token.
		"[[ x =~ a(<()) ]]",
		"[[ x =~ a(>()) ]]",
		"[[ x =~ a(<()|b) ]]",
		"[[ x =~ <() ]]",
	} {
		t.Run(src, func(t *testing.T) {
			if _, err := syntax.NewParser(syntax.Variant(syntax.LangZsh)).Parse(strings.NewReader(src+"\n"), ""); err != nil {
				t.Fatal(err)
			}
		})
	}
}

// Bash has no Zsh glob groups; its regex reader keeps `()` in the operand.
func TestBashEmptyRegexGroupUnchanged(t *testing.T) {
	for _, src := range []string{"[[ x =~ a(()) ]]", "[[ x =~ a() ]]"} {
		if _, err := syntax.NewParser(syntax.Variant(syntax.LangBash)).Parse(strings.NewReader(src+"\n"), ""); err != nil {
			t.Fatalf("%s: %v", src, err)
		}
	}
}
