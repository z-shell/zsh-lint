package parse

import (
	"errors"
	"strings"
	"testing"

	"github.com/z-shell/zsh-lint/third_party/mvdan-sh/syntax"
)

// A subscript written directly after a flagged one, as in ${a[(r)x][^b]},
// is read by the type of the value the flagged subscript returns, which the
// parser cannot know, so text that fails as arithmetic stays one literal, as
// the expression after a flagged pattern's `,` does (#277). Before later
// subscripts became typed children (#252), the second-subscript mask turned
// these sources into exactly that comma form, so they were accepted.
func TestParseLaterSubscriptAfterFlaggedSubscript(t *testing.T) {
	tests := []struct {
		src   string
		flags string
		later []string
	}{
		{"print -r -- ${a[(r)x][--$i]}\n", "r", []string{"--$i"}},
		{"print -r -- ${a[(r)x][^b]}\n", "r", []string{"^b"}},
		{"print -r -- \"${a[(r)x][^b]}\"\n", "r", []string{"^b"}},
		{"x=${a[(r)x][^b]}\n", "r", []string{"^b"}},
		{"f() { print -r -- ${a[(r)x][^b]}; }\n", "r", []string{"^b"}},
		{"print -r -- ${a[(i)x][^b]}\n", "i", []string{"^b"}},
		{"print -r -- ${a[(r)x][--[^:]##]}\n", "r", []string{"--[^:]##"}},
		{"print -r -- ${a[(r)x][[ab]]}\n", "r", []string{"[ab]"}},
		{"print -r -- ${a[(r)x][^b][2]}\n", "r", []string{"^b", "2"}},
		{"print -r -- ${a[(r)x][--$i][1]}\n", "r", []string{"--$i", "1"}},
	}
	for _, test := range tests {
		t.Run(test.src, func(t *testing.T) {
			file, err := Parse(strings.NewReader(test.src), "later.zsh")
			if err != nil {
				t.Fatalf("Parse() error: %v", err)
			}
			seconds := file.SecondSubscripts()
			if len(seconds) != 1 {
				t.Fatalf("SecondSubscripts() = %d entries, want 1", len(seconds))
			}
			exp := seconds[0].Expansion
			flagged, ok := exp.Index.(*syntax.FlagsArithm)
			if !ok || flagged.Flags == nil || flagged.Flags.Value != test.flags {
				t.Fatalf("Index = %T, want a FlagsArithm with flags %q", exp.Index, test.flags)
			}
			if len(exp.LaterIndexes) != len(test.later) {
				t.Fatalf("LaterIndexes = %d, want %d", len(exp.LaterIndexes), len(test.later))
			}
			for i, want := range test.later {
				if got := nodeText(test.src, exp.LaterIndexes[i]); got != want {
					t.Errorf("LaterIndexes[%d] = %q, want %q", i, got, want)
				}
			}
			word, ok := exp.LaterIndexes[0].(*syntax.Word)
			if !ok || len(word.Parts) != 1 {
				t.Fatalf("LaterIndexes[0] = %T, want a one-part Word", exp.LaterIndexes[0])
			}
			if lit, ok := word.Parts[0].(*syntax.Lit); !ok || lit.Value != test.later[0] {
				t.Errorf("LaterIndexes[0] part = %#v, want the literal %q", word.Parts[0], test.later[0])
			}
		})
	}
}

// The literal reading still needs a closed subscript, and it never reads a
// substitution, whose extent it does not decide.
func TestLaterSubscriptAfterFlaggedSubscriptBoundaries(t *testing.T) {
	// Another flagged expansion whose later subscript is arithmetic does not
	// disturb the verification of the literal one.
	if _, err := Parse(strings.NewReader("print -r -- ${b[(r)x][1+2]} ${a[(r)x][^b]}\n"), "later.zsh"); err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	// Without a flagged subscript on the error's line the search stops at
	// the line start, and the error keeps its position.
	_, err := Parse(strings.NewReader("print -r -- ${b[(r)x]}\nprint -r -- ${a[1][^b]}\n"), "later.zsh")
	var parseErr syntax.ParseError
	if !errors.As(err, &parseErr) || parseErr.Pos.Line() != 2 || parseErr.Pos.Col() != 20 {
		t.Fatalf("error = %v, want the original error at 2:20", err)
	}
	for _, src := range []string{
		"print -r -- ${a[(r)x][^b}\n",
		"print -r -- ${a[(r)x][^b\n",
	} {
		t.Run(src, func(t *testing.T) {
			if _, err := Parse(strings.NewReader(src), "later.zsh"); err == nil {
				t.Fatal("Parse() accepted an unterminated later subscript")
			}
		})
	}
	// A substitution in such a subscript is never hidden in literal text:
	// the source either fails or keeps the substitution as a typed node.
	src := "print -r -- ${a[(r)x][^$(b)]}\n"
	file, err := Parse(strings.NewReader(src), "later.zsh")
	if err != nil {
		return
	}
	found := false
	syntax.Walk(file.AST(), func(node syntax.Node) bool {
		if _, ok := node.(*syntax.CmdSubst); ok {
			found = true
		}
		return true
	})
	if !found {
		t.Fatal("the command substitution is hidden in literal subscript text")
	}
}
