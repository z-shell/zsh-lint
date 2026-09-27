package syntax_test

import (
	"strings"
	"testing"

	"mvdan.cc/sh/v3/syntax"
)

// Issue #526. A subscript flag argument is read as raw text up to its `,`
// or `]`, so the body of a command substitution in it was never parsed as
// commands. Zsh parses it, and rejects `${x[(r)$(done)]}` as it rejects
// `$(done)`. Native verdicts use newline-terminated files and zsh -f -n.
func TestZshFlagSubstBodyReject(t *testing.T) {
	for _, tc := range []struct{ src, err string }{
		{"print ${x[(r)$(done)]}", "1:16: `done` can only be used to end a loop"},
		{"print ${x[(r)a$(done)]}", "1:17: `done` can only be used to end a loop"},
		{"print ${x[(r)($(done))]}", "1:17: `done` can only be used to end a loop"},
		{"print ${x[(r)$(done),2]}", "1:16: `done` can only be used to end a loop"},
		{"print ${x[(r)\"$(done)\"]}", "1:17: `done` can only be used to end a loop"},
		{"print ${x[(r)$(echo $(done))]}", "1:23: `done` can only be used to end a loop"},
		{"print $x[(r)$(done)]", "1:15: `done` can only be used to end a loop"},
		{"print \"${x[(r)$(done)]}\"", "1:17: `done` can only be used to end a loop"},
		// A `'` in a subscript is text, as in double quotes, so this is
		// still a substitution.
		{"print ${x[(r)'$(done)']}", "1:17: `done` can only be used to end a loop"},
		// Quoting and escapes inside the body follow the shell: `\"` does
		// not close a `"`, a `'` string ends at the next `'`, and `\)` and
		// a backslash-newline are part of the body.
		{"print ${x[(r)$(echo \"a\\\"\"; done)]}", "1:28: `done` can only be used to end a loop"},
		{"print ${x[(r)$(echo 'a\\'; done)]}", "1:27: `done` can only be used to end a loop"},
		{"print ${x[(r)$(echo \\); done)]}", "1:25: `done` can only be used to end a loop"},
		{"print ${x[(r)$(echo a \\\n; done)]}", "2:3: `done` can only be used to end a loop"},
		// An escaped backslash does not stop the substitution.
		{"print ${x[(r)\\\\$(done)]}", "1:18: `done` can only be used to end a loop"},
		// Zsh counts an escaped `]` in the body too, so it is unbalanced
		// (#529).
		{"print ${x[(r)$(echo \\])]}", "1:22: the `[` and `]` in command substitutions in a subscript must balance"},
		// A quoted `)` does not close the substitution.
		{"print ${x[(r)$(echo ')'; done)]}", "1:26: `done` can only be used to end a loop"},
		// zsh -n parses these bodies only when they run; the parser
		// rejects `${x[`done`]}` without a flag too.
		{"print ${x[(i)$(if true)]}", "1:16: `if <cond>` must be followed by `then`"},
		{"print ${x[(r)`done`]}", "1:15: `done` can only be used to end a loop"},
	} {
		t.Run(tc.src, func(t *testing.T) {
			_, err := syntax.NewParser(syntax.Variant(syntax.LangZsh)).Parse(strings.NewReader(tc.src+"\n"), "")
			if err == nil || err.Error() != tc.err {
				t.Fatalf("error = %v, want %s", err, tc.err)
			}
		})
	}
}

func TestZshFlagSubstBodyAccept(t *testing.T) {
	for _, src := range []string{
		"print ${x[(r)$(true)]}",
		"print ${x[(r)a$(echo x)b]}",
		// A `,` inside the substitution is part of it, not the range.
		"print ${x[(r)$(echo ,)]}",
		"print ${x[(r)$(echo ,),2]}",
		"print ${x[(r)$((1+2))]}",
		"print ${x[(r)$((1<<2))]}",
		// Escaped, a `$(` is text of the pattern.
		"print ${x[(r)\\$(done)]}",
		"print $x[(r)\\$(done)]",
		"print \"${x[(r)\\$(done)]}\"",
		"print ${x[(r)'$(true)']}",
		// A quoted or escaped `)` stays in the substitution.
		"print $x[(I)$(echo ')')]",
		"print $x[(I)$(echo \")\")]",
		"print $x[(I)$(echo \\))]",
		// In a pattern, `<(z)` is `<` and a group, not a process
		// substitution: it finds an element `<z`.
		"print ${x[(i)<(done)]}",
		"print ${x[(r)`echo \\`true\\``]}",
		// An escaped `]` in a backquoted body does not end the argument.
		"print ${x[(r)`echo \\]`]}",
	} {
		t.Run(src, func(t *testing.T) {
			if _, err := syntax.NewParser(syntax.Variant(syntax.LangZsh)).Parse(strings.NewReader(src+"\n"), ""); err != nil {
				t.Fatal(err)
			}
		})
	}
}
