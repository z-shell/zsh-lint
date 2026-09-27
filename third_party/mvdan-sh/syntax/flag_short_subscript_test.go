package syntax_test

import (
	"strings"
	"testing"

	"mvdan.cc/sh/v3/syntax"
)

// Issue #532. A short subscript with flags, as in `$y[(r)a]`, inside a
// subscript flag argument ended the argument at its own `]`, so valid Zsh
// such as `${x[(r)$y[(r)a]]}` was rejected. Native verdicts use
// newline-terminated files and zsh -f -n.
func TestZshFlagShortSubscriptAccept(t *testing.T) {
	for _, src := range []string{
		"print ${x[(r)$y[(r)a]]}",
		"print ${x[(r)$y[(i)b]]}",
		"print ${x[(r)$y[(r)a[b]c]]}",
		"print ${x[(r)$y[(r)(a|b)]]}",
		"print ${x[(r)$y[(r)$(echo [a])]]}",
		"print ${x[(r)$y[(r)$z[(r)a]]]}",
		"print ${x[(r)$y[(r)a]$z[(r)b]]}",
		"print ${x[(r)$y[(r)a][1]]}",
		"print ${x[(r)$y[(r)a][(r)b]]}",
		"print ${x[(r)$_y[(r)a]]}",
		"print ${x[(r)$1[(r)a]]}",
		// A `,` inside the short subscript is its own; one after it is
		// the outer range.
		"print ${x[(r)$y[(r)a,2]]}",
		"print ${x[(r)$y[(r)a],2]}",
		"print ${x[(r)$y[(r)a]b]}",
		// Quotes around it do not hide it, and an escaped `]` inside it
		// is its own.
		"print ${x[(r)'$y[(r)a]']}",
		"print ${x[(r)\"$y[(r)a]\"]}",
		"print ${x[(r)$y[(r)\\]]]}",
		"print \"${x[(r)$y[(r)a]]}\"",
		"print ${x[(r)$y[(r)a]]:-z}",
		"x=${x[(r)$y[(r)a]]}",
		"[[ ${x[(r)$y[(r)a]]} == a ]]",
		// A quoted, escaped or balanced `}` inside it does not close the
		// outer expansion.
		"print ${x[(r)$y[(r)'}']]}",
		"print ${x[(r)$y[(r)\\}]]}",
		"print ${x[(r)$y[(r){a}]]}",
		"print ${x[(r)$y[(r)${z}]]}",
		// An escaped `[` after it is text, not another subscript.
		"print ${x[(r)$y[(r)a]\\[b]}",
		// In double quotes an escaped `]`, `[` or `}` inside it is text.
		"print \"${x[(r)$y[(r)\\]]]}\"",
		"print \"${x[(r)$y[(r)\\[]]}\"",
		"print \"${x[(r)$y[(r)\\}]]}\"",
		// In a short outer subscript nothing inside is counted.
		"print $x[(r)$y[(r)}]]",
	} {
		t.Run(src, func(t *testing.T) {
			if _, err := syntax.NewParser(syntax.Variant(syntax.LangZsh)).Parse(strings.NewReader(src+"\n"), ""); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestZshFlagShortSubscriptReject(t *testing.T) {
	for _, tc := range []struct{ src, err string }{
		// A short subscript left open to the outer `]` no longer lets the
		// argument close there: Zsh reports an invalid subscript.
		{"print ${x[(r)$y[(r)a]}", "1:10: reached EOF without matching `[` with `]`"},
		{"print ${x[(r)$y[(r)a\\]]}", "1:10: reached EOF without matching `[` with `]`"},
		// Text after the outer `]` is still an error.
		{"print ${x[(r)$y[(r)a]]x}", "1:23: `x` cannot be followed by a word"},
		// A stray `}` inside it closes the outer expansion in Zsh.
		{"print ${x[(r)$y[(r)}]]}", "1:20: `}` can only be used to close a block"},
		{"print \"${x[(r)$y[(r)'}']]}\"", "1:22: `}` can only be used to close a block"},
		// A `{` inside it counts with the outer expansion.
		{"print ${x[(r)$y[(r){a]]}", "1:20: a `{` in a subscript must be closed before its `]`"},
		// An escaped `$` starts no short subscript, so its `[` opens nothing
		// and the `[` after it is left as before.
		{"print \"${x[(r)\\$y[(r)a]b[]}\"", "1:24: `x` cannot be followed by a word"},
		// A `[` after an unflagged one is left as before.
		{"print ${x[(r)$y[1]b[]}", "1:19: `x` cannot be followed by a word"},
		// The bracket balance of #529 still counts inside it.
		{"print ${x[(r)$y[(r)$(echo ])]]}", "1:27: the `[` and `]` in command substitutions in a subscript must balance"},
	} {
		t.Run(tc.src, func(t *testing.T) {
			_, err := syntax.NewParser(syntax.Variant(syntax.LangZsh)).Parse(strings.NewReader(tc.src+"\n"), "")
			if err == nil || err.Error() != tc.err {
				t.Fatalf("error = %v, want %s", err, tc.err)
			}
		})
	}
}
