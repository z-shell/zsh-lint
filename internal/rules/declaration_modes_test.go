package rules

import (
	"strings"
	"testing"

	"github.com/z-shell/zsh-lint/internal/analyzer"
	"github.com/z-shell/zsh-lint/internal/diag"
	"github.com/z-shell/zsh-lint/internal/parse"
	"github.com/z-shell/zsh-lint/internal/projectconfig"
)

func TestDeclarationCallModesAndRanges(t *testing.T) {
	for _, test := range []struct {
		name     string
		src      string
		path     string
		want     int
		severity diag.Severity
	}{
		{"quoted operands", "example_f() { builtin local \"ZSH_VERSION=$1\"; }\n", "example.zsh", 1, diag.Warning},
		{"query", "example_f() { builtin typeset -p ZSH_VERSION; }\n", "example.zsh", 0, 0},
		{"plus query", "example_f() { noglob typeset +p ZSH_VERSION; }\n", "example.zsh", 0, 0},
		{"function query", "example_f() { - typeset -f ZSH_VERSION; }\n", "example.zsh", 0, 0},
		{"global", "example_f() { nocorrect typeset -g ZSH_VERSION=1; }\n", "example.zsh", 0, 0},
		{"dynamic option", "example_f() { builtin local $flags ZSH_VERSION=1; }\n", "example.zsh", 0, 0},
		{"external", "example_f() { command local ZSH_VERSION=1; }\n", "example.zsh", 0, 0},
		{"quoted nocorrect", "example_f() { 'nocorrect' local ZSH_VERSION=1; }\n", "example.zsh", 0, 0},
		{"modifiers quoted", "example_f() { 'noglob' builtin local ZSH_VERSION=1; }\n", "example.zsh", 1, diag.Warning},
		{"scalar test platform", "example_f() { builtin local OSTYPE=darwin; }\n", "tests/example.zsh", 1, diag.Info},
		{"valueless test platform", "example_f() { builtin local OSTYPE; }\n", "tests/example.zsh", 1, diag.Warning},
		{"dynamic scalar test platform", "example_f() { builtin local \"OSTYPE=$1\"; }\n", "tests/example.zsh", 1, diag.Info},
		{"suppression", "# zsh-lint disable=compat/special-param-shadow -- deliberate fixture\nexample_f() { builtin local ZSH_VERSION=1; }\n", "example.zsh", 0, 0},
		{"multiline", "example_f() {\n builtin local \\\n ZSH_VERSION=1\n}\n", "example.zsh", 1, diag.Warning},
	} {
		t.Run(test.name, func(t *testing.T) {
			file, err := parse.Parse(strings.NewReader(test.src), test.path)
			if err != nil {
				t.Fatal(err)
			}
			got := analyzer.New(SpecialParamShadow{}).Analyze(file, test.path)
			if len(got) != test.want {
				t.Fatalf("diagnostics = %+v, want %d", got, test.want)
			}
			if test.want == 0 {
				return
			}
			if got[0].Severity != test.severity {
				t.Fatalf("severity = %v, want %v", got[0].Severity, test.severity)
			}
			start, end := got[0].Range.Start.Offset, got[0].Range.End.Offset
			if start < 0 || end > len(test.src) || end <= start || !strings.Contains(test.src[start:end], "OSTYPE") && !strings.Contains(test.src[start:end], "ZSH_VERSION") {
				t.Fatalf("bad range: %+v", got[0].Range)
			}
		})
	}
}

func TestDeclarationCallPersistentDeduplication(t *testing.T) {
	for _, source := range []string{"builtin typeset -g foreign_state=1\n", "nocorrect typeset -g foreign_state=1\n"} {
		file, err := parse.Parse(strings.NewReader(source), "example.zsh")
		if err != nil {
			t.Fatal(err)
		}
		got := analyzer.New(PersistentParameterNamespace{}).AnalyzeSource(file, "example.zsh", sourceContext(projectconfig.KindPlugin, projectconfig.ProfileSourcedLibrary, "example"))
		if len(got) != 1 {
			t.Fatalf("diagnostics = %+v, want one", got)
		}
	}
}
