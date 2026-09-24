package parse

import (
	"os"
	"strings"
	"testing"
)

// A command substitution's bytes stay inside the flagged pattern's raw literal,
// so nothing downstream reads them as the commands they are. scanFlagPatternBrackets
// therefore parses the body itself, and refuses the repair when the body is not a
// command list Zsh accepts.
//
// Every row here was measured with `zsh -f -n` on a file holding the row, and the
// wanted error is the one main produces for the same source, so a refusal cannot
// silently change the message a user sees.
func TestFlagPatternSubstitutionBodyRefusals(t *testing.T) {
	const operatorDollar = "not a valid parameter expansion operator: `$`"
	tests := []struct {
		name    string
		fixture string
		want    string
		why     string
	}{
		{
			name:    "reserved word alone",
			fixture: "testdata/invalid-379-body-reserved-word.txt",
			want:    operatorDollar,
			why:     "`$(done)` is `closing brace expected` in Zsh",
		},
		{
			name:    "else alone",
			fixture: "testdata/invalid-379-body-else.txt",
			want:    operatorDollar,
			why:     "upstream parses a lone `else` as a word, Zsh does not",
		},
		{
			name:    "leading pipe",
			fixture: "testdata/invalid-379-body-leading-pipe.txt",
			want:    operatorDollar,
			why:     "`$(| echo a)` has no command before the pipe",
		},
		{
			name:    "empty group",
			fixture: "testdata/invalid-379-body-empty-group.txt",
			want:    operatorDollar,
			why:     "`$(echo ())` is `closing brace expected` in Zsh",
		},
		{
			name:    "stray close paren after the substitution",
			fixture: "testdata/invalid-379-stray-paren-after.txt",
			want:    operatorDollar,
			why:     "the `)` after the substitution is `invalid subscript` in Zsh",
		},
		{
			name:    "case arm paren closes the substitution early",
			fixture: "testdata/invalid-379-body-case-paren.txt",
			want:    operatorDollar,
			why:     "the body parses alone, but its `)` ends the substitution and Zsh reports `bad substitution`",
		},
		{
			name:    "close paren glued to the substitution",
			fixture: "testdata/invalid-379-stray-paren-glued.txt",
			want:    operatorDollar,
			why:     "nothing opened the `)` that follows, which Zsh reports as `invalid subscript`",
		},
		{
			name:    "stray close paren after a closed group",
			fixture: "testdata/invalid-379-stray-paren-after-group.txt",
			want:    "not a valid parameter expansion operator: `)`",
			why:     "the group already closed, so the next `)` opens nothing and Zsh reports `invalid subscript`",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture, err := os.ReadFile(test.fixture)
			if err != nil {
				t.Fatalf("read fixture: %v", err)
			}
			_, parseErr := Parse(strings.NewReader(string(fixture)), "t.zsh")
			if parseErr == nil {
				t.Fatalf("Parse() accepted native-invalid source (%s)", test.why)
			}
			if !strings.Contains(parseErr.Error(), test.want) {
				t.Errorf("Parse() error = %q, want it to contain %q", parseErr.Error(), test.want)
			}
		})
	}
}

// substitutionBodyParses decides whether the repair may step over a substitution.
// The accepted rows keep the #379 fix working; the refused rows are the class that
// a balance-only scan let through.
func TestSubstitutionBodyParses(t *testing.T) {
	accepted := []string{
		"echo x",
		"echo a,b",
		"echo $y",
		"echo ${y}",
		"echo $(echo z)",
		"echo $((1+1))",
		"",
		"   ",
		"true; false",
		"if true; then echo a; fi",
		// Measured with `zsh -f -n`: Zsh accepts each of these alone.
		"in",
		"fo",
		"time",
		"coproc",
	}
	for _, body := range accepted {
		if !substitutionBodyParses([]byte(body)) {
			t.Errorf("substitutionBodyParses(%q) = false, want true", body)
		}
	}
	refused := []string{
		"done",
		"fi",
		"esac",
		"then",
		"do",
		"elif",
		"&& echo a",
		"| echo a",
		"echo (",
		"if true",
		"while true",
		// Upstream parses these as ordinary commands; Zsh reports a parse error.
		"else",
		"nocorrect",
		"repeat",
		"foreach",
		"end",
	}
	for _, body := range refused {
		if substitutionBodyParses([]byte(body)) {
			t.Errorf("substitutionBodyParses(%q) = true, want false", body)
		}
	}
}

// The words in zshIncompleteWords are refused by name because upstream accepts
// them where Zsh does not. A word that Zsh accepts alone must never join them, or
// valid source starts failing, so this pins the divergence in both directions.
func TestZshIncompleteWords(t *testing.T) {
	for _, word := range []string{"else", "nocorrect", "repeat", "foreach", "end"} {
		if !isZshIncompleteWord([]byte(word)) {
			t.Errorf("isZshIncompleteWord(%q) = false, want true", word)
		}
	}
	// Measured `zsh -f -n` exit 0: refusing any of these would reject valid source.
	for _, word := range []string{"in", "fo", "time", "coproc", "echo", "done x", ""} {
		if isZshIncompleteWord([]byte(word)) {
			t.Errorf("isZshIncompleteWord(%q) = true, want false", word)
		}
	}
}

// The #379 repair must still accept the source it was written for, including the
// rows whose substitution sits beside a bracket expression or a glob group.
func TestFlagPatternSubstitutionStillAccepted(t *testing.T) {
	tests := []struct {
		name string
		src  string
	}{
		{"substitution after a bracket expression", "local -a m=( a1 abx abc )\nprint ${m[(i)a[bc]$(echo x)]}\n"},
		{"substitution after plain text", "local -a m=( a1 abx abc )\nprint ${m[(i)a_bc_$(echo x)]}\n"},
		{"comma inside the body", "local -a m=( a1 abx abc )\nprint ${m[(i)a[bc]$(echo a,b)]}\n"},
		{"glob group before the substitution", "local -a m=( a1 abx abc )\nprint ${m[(i)(a|b)$(echo x)]}\n"},
		{"arithmetic expansion", "local -a m=( a1 abx abc )\nprint ${m[(i)a[bc]$((1+1))]}\n"},
		{"backquoted substitution", "local -a m=( a1 abx abc )\nprint ${m[(i)a[bc]`echo x`]}\n"},
		{"empty substitution", "local -a m=( a1 abx abc )\nprint ${m[(i)a[bc]$()]}\n"},
		{"range flag with a trailing index", "local -a m=( a1 abx abc )\nprint ${m[(r)a[bc]$(echo x),3]}\n"},
		// A group may open before the substitution and close after it, so the
		// scan must carry its parenthesis count across the substitution. Each
		// row holds a bracket expression too, so it reaches the repair: a row
		// without one parses upstream unaided and proves nothing here.
		{"group with a bracket expression then a substitution", "local -a m=( a1 abx abc )\nprint ${m[(i)(a[bc]|b)$(echo x)]}\n"},
		{"group closed before the substitution", "local -a m=( a1 abx abc )\nprint ${m[(i)(a[bc])$(echo x)]}\n"},
		{"group left open across the substitution", "local -a m=( a1 abx abc )\nprint ${m[(i)(a[bc]$(echo x))]}\n"},
		{"alternation left open across the substitution", "local -a m=( a1 abx abc )\nprint ${m[(i)(a[bc]|b$(echo x))]}\n"},
		{"nested groups before the substitution", "local -a m=( a1 abx abc )\nprint ${m[(i)((a[bc]))$(echo x)]}\n"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := Parse(strings.NewReader(test.src), "t.zsh"); err != nil {
				t.Errorf("Parse() error = %v, want the row accepted", err)
			}
		})
	}
}
