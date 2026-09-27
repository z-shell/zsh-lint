package parse

import (
	"bytes"
	"errors"
	"os"
	"strings"
	"testing"

	"mvdan.cc/sh/v3/syntax"
)

// Issue #363: in Zsh a leading `+` in a braced parameter expansion tests
// whether a named parameter is set. Anything but a name after it, a nested
// expansion, a quote, a special parameter, an operator or a second `+`, is a
// bad substitution. The fixtures are at the top level, where `zsh -f -n`
// expands the word and reports it, except the assignment row, which `-n`
// leaves unexpanded and Zsh rejects only when the line runs (#287).
func TestIsSetPrefixRequiresName(t *testing.T) {
	const text = "`${+name}` requires a parameter name after `+`"
	for _, test := range []struct {
		fixture string
		col     uint
	}{
		{"testdata/invalid-363-command-substitution.txt", 9},
		{"testdata/invalid-363-nested-parameter.txt", 9},
		{"testdata/invalid-363-doubled-plus.txt", 9},
		{"testdata/invalid-363-assignment-context.txt", 5},
	} {
		t.Run(test.fixture, func(t *testing.T) {
			src, err := os.ReadFile(test.fixture)
			if err != nil {
				t.Fatalf("read invalid fixture: %v", err)
			}
			_, err = Parse(bytes.NewReader(src), "invalid-363.zsh")
			if err == nil {
				t.Fatal("Parse() accepted a source native Zsh rejects")
			}
			var parseErr syntax.ParseError
			if !errors.As(err, &parseErr) {
				t.Fatalf("error = %v (%T), want a syntax.ParseError", err, err)
			}
			if parseErr.Text != text {
				t.Errorf("error text = %q, want %q", parseErr.Text, text)
			}
			if parseErr.Pos.Line() != 2 || parseErr.Pos.Col() != test.col {
				t.Errorf("error at %d:%d, want 2:%d", parseErr.Pos.Line(), parseErr.Pos.Col(), test.col)
			}
		})
	}
}

// Each row is `bad substitution` under `zsh -f -n` at the top level.
func TestIsSetPrefixRejectsNonName(t *testing.T) {
	for _, src := range []string{
		"print ${+\"$x\"}",
		"print ${+\"${x}\"}",
		"print ${+$x}",
		"print ${+${x}[1]}",
		"print ${=+$(echo 1)}",
		"print ${~+${x}}",
		"print ${(f)+${x}}",
		"print \"${+${x}}\"",
		"print ${+@}",
		"print ${+*}",
		"print ${+#}",
		"print ${+?}",
		"print ${+-}",
		"print ${+$}",
		"print ${+!}",
		"print ${+}",
		"print ${+ x}",
		"print ${+:-a}",
		"print ${+[1]}",
		"print ${+(f)x}",
		"print ${+~x}",
		"print ${+.x}",
		"print ${+/a}",
		"print ${+%a}",
		"print ${+^x}",
		"print ${+=x}",
		"print ${++x}",
	} {
		if _, err := Parse(strings.NewReader(src+"\n"), "t.zsh"); err == nil {
			t.Errorf("invalid Zsh accepted: %q", src)
		}
	}
}

// Each row is valid under `zsh -f -n` and runs. `$+` without braces, a
// leading `#` that makes `+` the alternate-value operator, and a prefix
// before `+` all keep their meaning.
func TestIsSetPrefixKeepsValidForms(t *testing.T) {
	for _, test := range []struct {
		src   string
		isSet bool // the outermost expansion is the ${+name} test
	}{
		{"print ${+x}", true},
		{"print ${+x[1]}", true},
		{"print ${+x[(i)a]}", true},
		{"print ${+_}", true},
		{"print ${+x_1}", true},
		{"print ${+1}", true},
		{"print ${+12}", true},
		{"print ${=+x}", true},
		{"print ${^+x}", true},
		{"print ${(f)+x}", true},
		{"print ${#+$(echo 1)}", false},
		{"print ${#+${x}}", false},
		{"print ${#+x}", false},
		{"print $+x", true},
		{"print $+x[1]", true},
		{"print $+1", true},
		{"print ${x+${y}}", false},
		{"print ${x:+$(echo 1)}", false},
	} {
		t.Run(test.src, func(t *testing.T) {
			f, err := Parse(strings.NewReader(test.src+"\n"), "t.zsh")
			if err != nil {
				t.Fatalf("valid Zsh rejected: %v", err)
			}
			var got *syntax.ParamExp
			syntax.Walk(f.AST(), func(node syntax.Node) bool {
				if pe, ok := node.(*syntax.ParamExp); ok && got == nil {
					got = pe
				}
				return got == nil
			})
			if got == nil {
				t.Fatal("no parameter expansion in the tree")
			}
			if got.IsSet != test.isSet {
				t.Errorf("IsSet = %v, want %v", got.IsSet, test.isSet)
			}
		})
	}

	// A name that starts with a non-ASCII letter is valid Zsh (`${+ä}` runs
	// and prints 0). The check leaves such names to the upstream path, which
	// accepts them, so the gate must not reject one.
	if _, err := Parse(strings.NewReader("print ${+ä}\n"), "t.zsh"); err != nil {
		t.Errorf("valid Zsh rejected: %q: %v", "print ${+ä}", err)
	}
}
