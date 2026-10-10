package rules

import (
	"strings"
	"testing"

	"github.com/z-shell/zsh-lint/internal/analyzer"
	"github.com/z-shell/zsh-lint/internal/parse"
)

func TestPreferDoubleBrackets(t *testing.T) {
	src := `
if [ "$foo" = "bar" ]; then echo 1; fi
if test "$foo" = "bar"; then echo 1; fi
if [[ "$foo" == "bar" ]]; then echo 1; fi
`
	f, err := parse.Parse(strings.NewReader(src), "test.zsh")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	analyzerInst := analyzer.New(PreferDoubleBrackets{})
	diags := analyzerInst.Analyze(f, "test.zsh")

	if len(diags) != 2 {
		t.Errorf("expected 2 diagnostics, got %d", len(diags))
		for _, d := range diags {
			t.Logf("diag: %v", d.Message)
		}
	}
}

func TestPreferDoubleBracketsCommandForms(t *testing.T) {
	for _, name := range []string{"test", "["} {
		for _, command := range commandForms(name) {
			t.Run(command, func(t *testing.T) {
				src := "f() { " + command + " -f x"
				if name == "[" {
					src += " ]"
				}
				file, err := parse.Parse(strings.NewReader(src+"; }"), "test.zsh")
				if err != nil {
					t.Fatalf("parse: %v", err)
				}
				if got := analyzer.New(PreferDoubleBrackets{}).Analyze(file, "test.zsh"); len(got) != 1 {
					t.Fatalf("diagnostics = %v, want one", got)
				}
			})
		}
	}
}
