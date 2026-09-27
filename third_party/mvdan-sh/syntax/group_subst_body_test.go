package syntax_test

import (
	"strings"
	"testing"

	"mvdan.cc/sh/v3/syntax"
)

// Issue #519. A glob group reader keeps a command substitution, backquoted
// command or process substitution inside the group as text of its literal,
// so the body was never parsed as commands. Zsh parses it, and rejects
// `a($(done))` as it rejects `$(done)`. Native verdicts use
// newline-terminated files and zsh -f -n.
func TestZshGroupSubstBodyReject(t *testing.T) {
	for _, tc := range []struct{ src, err string }{
		{"print a($(done))", "1:11: `done` can only be used to end a loop"},
		{"print a(b|$(done))", "1:13: `done` can only be used to end a loop"},
		{"print a(`done`)", "1:10: `done` can only be used to end a loop"},
		{"print a(<(done))", "1:11: `done` can only be used to end a loop"},
		{"print a(>(done))", "1:11: `done` can only be used to end a loop"},
		{"print a(\"$(done)\")", "1:12: `done` can only be used to end a loop"},
		{"print a(${x:-$(done)})", "1:16: `done` can only be used to end a loop"},
		{"print a($(echo $(done)))", "1:18: `done` can only be used to end a loop"},
		{"print a($( (done) ))", "1:13: `done` can only be used to end a loop"},
		{"print a(b)($(done))", "1:14: `done` can only be used to end a loop"},
		{"[[ x == a($(done)) ]]", "1:13: `done` can only be used to end a loop"},
		{"[[ x == (a|(b|$(done))) ]]", "1:17: `done` can only be used to end a loop"},
		{"[[ -n a($(done)) ]]", "1:11: `done` can only be used to end a loop"},
		{"case x in a($(done))) ;; esac", "1:15: `done` can only be used to end a loop"},
		// zsh -n parses an assignment's substitution only when it runs, so
		// this passes zsh -f -n and fails when run; the parser rejects
		// `x=$(if true)` outside a group too.
		{"x=a($(if true))", "1:7: `if <cond>` must be followed by `then`"},
		{"print a($(| a))", "1:11: `|` can only immediately follow a statement"},
		// Positions follow the body across lines and escaped newlines.
		{"print a(\n$(done))", "2:3: `done` can only be used to end a loop"},
		{"print a($(echo a\ndone))", "2:1: `done` can only be used to end a loop"},
		{"print a($(echo a \\\n; done))", "2:3: `done` can only be used to end a loop"},
		// Inside backquotes `\$`, `` \` `` and `\\` are escapes of the
		// backquoted command; the position counts them.
		{"print a(`echo \\$(done)`)", "1:18: `done` can only be used to end a loop"},
		{"print a(`echo \\$x; done`)", "1:20: `done` can only be used to end a loop"},
		// An escape on an earlier line shifts the offset but not the column.
		{"print a(`echo \\$x\ndone`)", "2:1: `done` can only be used to end a loop"},
	} {
		t.Run(tc.src, func(t *testing.T) {
			_, err := syntax.NewParser(syntax.Variant(syntax.LangZsh)).Parse(strings.NewReader(tc.src+"\n"), "")
			if err == nil || err.Error() != tc.err {
				t.Fatalf("error = %v, want %s", err, tc.err)
			}
		})
	}
}

func TestZshGroupSubstBodyAccept(t *testing.T) {
	for _, src := range []string{
		"print a($(true))",
		"print a($(echo a; echo b))",
		"print a(b|$(echo x))",
		"print a(`true`)",
		"print a(<(true))",
		"print a($(cat <<E\nx\nE\n))",
		"print a($(echo ')'))",
		"print a($(echo \\)))",
		"print a($(# c\n))",
		"print a($(echo \\\ndone))",
		// Arithmetic, and a body that is only quoted text.
		"print a($((1+2)))",
		"print a($(( 1 + 2 )))",
		"print a('$(done)')",
		// Escapes of the backquoted command are removed before the body
		// is read: `\`true\`` is a nested command substitution.
		"print a(`echo \\`true\\``)",
		"print a(\"`echo \\\"a\\\"`\")",
		"print a(`print \\\\`)",
	} {
		t.Run(src, func(t *testing.T) {
			if _, err := syntax.NewParser(syntax.Variant(syntax.LangZsh)).Parse(strings.NewReader(src+"\n"), ""); err != nil {
				t.Fatal(err)
			}
		})
	}
}

// The reported position is a full source position, byte offset included:
// an escape removed from a backquoted body is counted back.
func TestZshGroupSubstBodyOffset(t *testing.T) {
	for _, tc := range []struct {
		src  string
		offs uint
	}{
		{"print a($(done))", 10},
		{"print a(`echo \\$x; done`)", 19},
		{"print a(`echo \\$x\ndone`)", 18},
	} {
		_, err := syntax.NewParser(syntax.Variant(syntax.LangZsh)).Parse(strings.NewReader(tc.src+"\n"), "")
		perr, ok := err.(syntax.ParseError)
		if !ok {
			t.Fatalf("%q: error = %v, want a ParseError", tc.src, err)
		}
		if got := perr.Pos.Offset(); got != tc.offs || tc.src[got:got+4] != "done" {
			t.Fatalf("%q: offset = %d, want %d at `done`", tc.src, got, tc.offs)
		}
	}
}
