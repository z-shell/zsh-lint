package syntax_test

import (
	"strings"
	"testing"

	"mvdan.cc/sh/v3/syntax"
)

// Issue #529. A `]` inside a command substitution in a subscript flag
// argument ended the argument, so valid Zsh such as `${x[(r)$(echo [a])]}`
// was rejected. The `]` is text of the substitution; in a `${...}` Zsh
// requires the `[` and `]` bytes of such bodies to balance over the whole
// subscript. Native verdicts use newline-terminated files and zsh -f -n.
func TestZshFlagSubstBracketAccept(t *testing.T) {
	for _, src := range []string{
		"print ${x[(r)$(echo [a])]}",
		"print ${x[(r)$((x[1]+2))]}",
		"print ${x[(r)$(echo ${y[1]})]}",
		"print ${x[(r)$(echo $y[1])]}",
		// Balance is over the whole subscript, not each body.
		"print ${x[(r)$(echo ])$(echo [)]}",
		"print ${x[(r)$(echo ]; echo [)]}",
		"print ${x[(r)$(echo [),$(echo ])]}",
		"print ${x[(r)$(echo [a]),2]}",
		"print \"${x[(r)$(echo [a])]}\"",
		"x=${x[(r)$(echo [a])]}",
		"[[ ${x[(r)$(echo [a])]} == a ]]",
		// A short `$x[...]` does not count the brackets at all.
		"print $x[(r)$(echo ])]",
		"print $x[(r)$((x[1]+2))]",
		"print $x[(r)$(echo ]),2]",
		"print \"$x[(r)$(echo ])]\"",
		// Nor does an assignment's `a[...]`, or a `${...}` inside a short
		// subscript.
		"a[(r)$(echo ])]=b",
		"a[(r)$(echo [a])]=b",
		"print $x[${y[(r)$(echo [)]}]",
		"print $x[${y[(r)$(echo ])]}]",
		"print $x[$(echo ${y[(r)$(echo [)]})]",
		"print $x[(r)$(echo ${y[(r)$(echo [)]})]",
		// Counting ends with its `${...}`.
		"print ${x[1]}; a[(r)$(echo ])]=b",
		"print ${z:-$x[(r)$(echo ])]}",
		// A short subscript inside a `${...}` subscript is counted with it.
		"print ${x[$y[(r)$(echo [a])]]}",
		// A short subscript's backquoted body keeps its balanced brackets
		// too, and so does an assignment's.
		"print $x[(r)`echo [a]`]",
		"print \"$x[(r)`echo [[a]]`]\"",
		"a[(r)`echo [a]`]=b",
		// In a backquoted body only an unescaped bracket counts.
		"print ${x[(r)`echo [a]`]}",
		"print ${x[(r)`echo [``echo ]`]}",
		"print ${x[(r)`echo \\]`]}",
	} {
		t.Run(src, func(t *testing.T) {
			if _, err := syntax.NewParser(syntax.Variant(syntax.LangZsh)).Parse(strings.NewReader(src+"\n"), ""); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestZshFlagSubstBracketReject(t *testing.T) {
	const balance = "the `[` and `]` in command substitutions in a subscript must balance"
	for _, tc := range []struct{ src, err string }{
		{"print ${x[(r)$(echo ])]}", "1:21: " + balance},
		{"print ${x[(r)$(echo [)]}", "1:21: " + balance},
		// A quoted bracket counts as well.
		{"print ${x[(r)$(echo \"[\")]}", "1:22: " + balance},
		{"print ${x[(r)$(echo [)$(echo [)]}", "1:21: " + balance},
		{"print \"${x[(r)$(echo ])]}\"", "1:22: " + balance},
		// At a `,`, a `]` that no `[` opened is already an error.
		{"print ${x[(r)$(echo ]),2]}", "1:21: " + balance},
		{"print ${x[(r)$(echo ]),$(echo ])]}", "1:21: " + balance},
		// Backquoted and `$(...)` bodies do not balance each other.
		{"print ${x[(r)`echo [`$(echo [)]}", "1:20: " + balance},
		{"print ${x[(r)`echo ][`]}", "1:20: a `]` in a backquoted command in a subscript must close a `[` before it"},
		{"print ${x[(r)`echo [`]}", "1:20: " + balance},
		// In a short subscript a `]` that closes nothing in a backquoted
		// body still ends the argument, as before, leaving the backquote
		// open; Zsh rejects it as an invalid subscript.
		{"print $x[(r)`echo ]`]", "1:20: reached EOF without closing quote \"`\""},
		{"print ${x[$y[(r)$(echo ])]]}", "1:24: " + balance},
		// After a short subscript, a `${...}` counts again.
		{"print $x[1] ${y[(r)$(echo [)]}", "1:27: " + balance},
		{"print ${x[${y[(r)$(echo [)]}]}", "1:25: " + balance},
		// The body is still parsed as commands (#526).
		{"print ${x[(r)$(echo [a]; done)]}", "1:26: `done` can only be used to end a loop"},
		{"print $x[(r)$(echo ]; done)]", "1:23: `done` can only be used to end a loop"},
	} {
		t.Run(tc.src, func(t *testing.T) {
			_, err := syntax.NewParser(syntax.Variant(syntax.LangZsh)).Parse(strings.NewReader(tc.src+"\n"), "")
			if err == nil || err.Error() != tc.err {
				t.Fatalf("error = %v, want %s", err, tc.err)
			}
		})
	}
}
