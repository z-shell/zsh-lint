package syntax_test

import (
	"strings"
	"testing"

	"mvdan.cc/sh/v3/syntax"
)

// Issue #531. Zsh counts the `[` and `]` bytes inside the command
// substitutions and backquoted commands of a `${...}` subscript over the
// whole subscript, including across a `,` and in an unflagged subscript,
// and rejects the expansion unless each kind balances. A `[` open at the
// `,` was accepted. Native verdicts use newline-terminated files and
// zsh -f -n.
func TestZshSubscriptBracketBalanceReject(t *testing.T) {
	const balance = "the `[` and `]` in command substitutions in a subscript must balance"
	for _, tc := range []struct{ src, err string }{
		{"print ${x[(r)$(echo [),2]}", "1:21: " + balance},
		{"print ${x[(r)$(echo [)a,2]}", "1:21: " + balance},
		{"print ${x[(r)$(echo [),$(echo [)]}", "1:21: " + balance},
		{"print ${x[(r)$(echo [[),$(echo ])]}", "1:21: " + balance},
		{"print ${x[(r)a,$(echo [)]}", "1:23: " + balance},
		{"print ${x[(r)a,$(echo ])]}", "1:23: " + balance},
		// Unflagged subscripts count too.
		{"print ${x[1,$(echo [)]}", "1:20: " + balance},
		{"print ${x[1,$(echo ])]}", "1:20: " + balance},
		{"print ${x[$(echo [),2]}", "1:18: " + balance},
		{"print ${x[${z:-$(echo [)}]}", "1:23: " + balance},
		// Escaped and quoted brackets in a body count as well.
		{"print ${x[1,$(echo \\[)]}", "1:21: " + balance},
		{"print ${x[(r)$(echo [),$(echo '[')$(echo ']')]}", "1:21: " + balance},
		// Backquoted brackets count on their own.
		{"print ${x[(r)`echo [`,2]}", "1:20: " + balance},
		{"print ${x[(r)`echo [`,$(echo ])]}", "1:30: " + balance},
		{"print ${x[(r)$(echo [),`echo ]`]}", "1:30: a `]` in a backquoted command in a subscript must close a `[` before it"},
		{"print \"${x[1,$(echo [)]}\"", "1:21: " + balance},
		// Positions follow the source across lines and continuations.
		{"print ${x[1,\n$(echo [)]}", "2:8: " + balance},
		{"print ${x[1,$(echo \\\n[)]}", "2:1: " + balance},
		{"print ${x[\\\n1,$(echo [)]}", "2:10: " + balance},
		// Brackets in a `${...}` word count with them.
		{"print ${x[(r)${z:-[}]}", "1:19: " + balance},
		{"print ${x[1,${z:-[}]}", "1:18: " + balance},
		{"print ${x[1,$(echo \\\r\n[)]}", "2:1: " + balance},
		// Columns count bytes, as elsewhere in the parser.
		{"print ${x[(r)é,$(echo [)]}", "1:24: " + balance},
		// Inside a body, quotes and nested parentheses do not end it.
		{"print ${x[1,$(echo '\\' [)]}", "1:24: " + balance},
		{"print ${x[1,$(echo ')' [)]}", "1:24: " + balance},
		{"print ${x[1,$(echo \")\" [)]}", "1:24: " + balance},
		{"print ${x[1,$( (echo a) ; echo [)]}", "1:32: " + balance},
		{"print ${x[1,$(echo $(echo a) [)]}", "1:30: " + balance},
		// Nested subscripts are checked on their own.
		{"print ${x[${y[1,$(echo [)]}]}", "1:24: " + balance},
		{"print ${x[${y[\\\n1,$(echo [)]}]}", "2:10: " + balance},
	} {
		t.Run(tc.src, func(t *testing.T) {
			_, err := syntax.NewParser(syntax.Variant(syntax.LangZsh)).Parse(strings.NewReader(tc.src+"\n"), "")
			if err == nil || err.Error() != tc.err {
				t.Fatalf("error = %v, want %s", err, tc.err)
			}
		})
	}
}

func TestZshSubscriptBracketBalanceAccept(t *testing.T) {
	for _, src := range []string{
		"print ${x[(r)$(echo [),$(echo ])]}",
		"print ${x[(r)$(echo [[),$(echo ]])]}",
		"print ${x[$(echo [),$(echo ])]}",
		"print ${x[$(echo ]),$(echo [)]}",
		"print ${x[1,$(echo [a])]}",
		"print ${x[(r)`echo [`,`echo ]`]}",
		"print ${x[(r)$(echo [),\"$(echo ])\"]}",
		"print ${x[(r)$(echo [),'$(echo ])']}",
		"print ${x[(r)$(echo [),${z:-$(echo ])}]}",
		"print ${x[(r)${z:-[},$(echo ])]}",
		"print ${x[${z:-[}$(echo ])]}",
		"print ${x[1,${y[(r)a]}]}",
		// A double-quoted `[` in a `${...}` word is text there.
		"print ${x[(r)${z:-\"[\"}]}",
		"print ${x[1,${z:-\"[\"}]}",
		// A short subscript counts no substitution brackets.
		"print $x[1,$(echo [)]",
		"print $x[(r)$(echo [),2]",
		// Text without brackets in a substitution is unaffected.
		"print ${x[1,$(echo a)]}",
	} {
		t.Run(src, func(t *testing.T) {
			if _, err := syntax.NewParser(syntax.Variant(syntax.LangZsh)).Parse(strings.NewReader(src+"\n"), ""); err != nil {
				t.Fatal(err)
			}
		})
	}
}

// The reported offset counts a backslash-newline before the bracket, at
// the start of the subscript and inside a nested one.
func TestZshSubscriptBracketBalanceOffset(t *testing.T) {
	for _, tc := range []struct {
		src  string
		offs uint
	}{
		{"print ${x[\\\n1,$(echo [)]}", 21},
		{"print ${x[${y[\\\n1,$(echo [)]}]}", 25},
	} {
		_, err := syntax.NewParser(syntax.Variant(syntax.LangZsh)).Parse(strings.NewReader(tc.src+"\n"), "")
		pe, ok := err.(syntax.ParseError)
		if !ok || pe.Pos.Offset() != tc.offs {
			t.Errorf("%q: error = %v, want offset %d", tc.src, err, tc.offs)
		}
	}
}

// A subscript far into a long input is still checked at its own position,
// after the read buffer has been refilled.
func TestZshSubscriptBracketBalanceLongInput(t *testing.T) {
	src := "print " + strings.Repeat("a", 1500) + " ${x[1," + strings.Repeat("$(echo a)", 300) + "$(echo [)]}"
	_, err := syntax.NewParser(syntax.Variant(syntax.LangZsh)).Parse(strings.NewReader(src+"\n"), "")
	want := "1:4221: the `[` and `]` in command substitutions in a subscript must balance"
	if err == nil || err.Error() != want {
		t.Fatalf("error = %v, want %s", err, want)
	}
	ok := "print " + strings.Repeat("a", 1500) + " ${x[1," + strings.Repeat("$(echo a)", 300) + "$(echo [a])]}"
	if _, err := syntax.NewParser(syntax.Variant(syntax.LangZsh)).Parse(strings.NewReader(ok+"\n"), ""); err != nil {
		t.Fatal(err)
	}
}
