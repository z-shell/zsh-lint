package rules

import (
	"strings"
	"testing"

	"github.com/z-shell/zsh-lint/internal/analyzer"
	"github.com/z-shell/zsh-lint/internal/diag"
	"github.com/z-shell/zsh-lint/internal/parse"
	"github.com/z-shell/zsh-lint/internal/projectconfig"
)

func TestSpecialParamShadowPlatformSimulation(t *testing.T) {
	tests := []struct {
		name    string
		source  string
		path    string
		profile projectconfig.Profile
		want    diag.Severity
	}{
		{"anonymous test platform", "() { local OSTYPE=darwin; }\n", "tests/platform.zsh", "", diag.Info},
		{"empty test initialization", "() { local OSTYPE=; }\n", "tests/platform.zsh", "", diag.Info},
		{"quoted empty test initialization", "() { local 'OSTYPE='; }\n", "tests/platform.zsh", "", diag.Info},
		{"named test platform", "simulate() { typeset OSTYPE=freebsd14.0; }\n", "Test/platform.zsh", "", diag.Info},
		{"declaration is the complete function body", "simulate() local OSTYPE=darwin\n", "tests/platform.zsh", "", diag.Info},
		{"quoted test initialization", "() { local 'OSTYPE=darwin'; }\n", "tests/platform.zsh", "", diag.Info},
		{"quoted test append", "() { local 'OSTYPE+=darwin'; }\n", "tests/platform.zsh", "", diag.Warning},
		{"tied test parameter", "() { typeset -T OSTYPE=darwin os_parts; }\n", "tests/platform.zsh", "", diag.Warning},
		{"mixed-case test directory", "() { local OSTYPE=darwin; }\n", "TeStS/platform.zsh", "", diag.Info},
		{"windows test directory", "() { local OSTYPE=darwin; }\n", "plugin\\Test\\platform.zsh", "", diag.Info},
		{"configured fixture outside test path", "() { local OSTYPE=darwin; }\n", "fixtures/platform.zsh", projectconfig.ProfileTestFixture, diag.Info},
		{"configured production in test path", "() { local OSTYPE=darwin; }\n", "tests/platform.zsh", projectconfig.ProfileSourcedLibrary, diag.Warning},
		{"configured executable in test path", "() { local OSTYPE=darwin; }\n", "tests/platform.zsh", projectconfig.ProfileStandaloneExecutable, diag.Warning},
		{"production local", "() { local OSTYPE=darwin; }\n", "plugin.zsh", "", diag.Warning},
		{"test substring", "() { local OSTYPE=darwin; }\n", "contest/platform.zsh", "", diag.Warning},
		{"top-level test declaration", "typeset OSTYPE=darwin\n", "tests/platform.zsh", "", diag.Warning},
		{"valueless test local", "() { local OSTYPE; }\n", "tests/platform.zsh", "", diag.Warning},
		{"array test declaration", "() { local OSTYPE=(darwin); }\n", "tests/platform.zsh", "", diag.Warning},
		{"indexed test declaration", "() { local OSTYPE[1]=darwin; }\n", "tests/platform.zsh", "", diag.Warning},
		{"append is not initialization", "() { local OSTYPE+=darwin; }\n", "tests/platform.zsh", "", diag.Warning},
		{"version test override unchanged", "() { local ZSH_VERSION=5.9.2; }\n", "tests/platform.zsh", "", diag.Warning},
		{"status test override unchanged", "() { local -h status=7; }\n", "tests/platform.zsh", "", diag.Warning},
		{"pipestatus test override unchanged", "() { local -h pipestatus=(7); }\n", "tests/platform.zsh", "", diag.Warning},
		{"following top-level declaration", "() { :; }\ntypeset OSTYPE=darwin\n", "tests/platform.zsh", "", diag.Warning},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			file, err := parse.Parse(strings.NewReader(test.source), test.path)
			if err != nil {
				t.Fatal(err)
			}
			context := projectconfig.SourceContext{}
			if test.profile != "" {
				context = configuredSource(projectconfig.KindPlugin, test.profile, "")
			}
			got := analyzer.New(SpecialParamShadow{}).AnalyzeSource(file, test.path, context)
			if len(got) != 1 || got[0].RuleID != "compat/special-param-shadow" || got[0].Severity != test.want {
				t.Fatalf("diagnostics = %+v, want one %s", got, test.want)
			}
			if test.want == diag.Info {
				if !strings.Contains(got[0].Message, "platform simulation") || strings.Contains(got[0].Message, "different parameter name") {
					t.Errorf("test guidance = %q", got[0].Message)
				}
			} else if !strings.Contains(got[0].Message, "different parameter name") {
				t.Errorf("production guidance changed: %q", got[0].Message)
			}
			start := strings.Index(test.source, "OSTYPE")
			if start >= 0 && !strings.Contains(test.source, "'OSTYPE") && (got[0].Range.Start.Offset != start || got[0].Range.End.Offset != start+len("OSTYPE")) {
				t.Errorf("range = %+v, want OSTYPE name at %d", got[0].Range, start)
			}
		})
	}
}

func TestSpecialParamShadowTestContextPreservesScopeGuards(t *testing.T) {
	for _, source := range []string{
		"() { typeset -g OSTYPE=darwin; }\n",
		"() { typeset $flags OSTYPE=darwin; }\n",
		"() { typeset -x OSTYPE=darwin; }\n",
		"() { typeset -p OSTYPE; }\n",
		"# zsh-lint disable=compat/special-param-shadow -- platform fixture\n() { local OSTYPE=darwin; }\n",
	} {
		file, err := parse.Parse(strings.NewReader(source), "tests/platform.zsh")
		if err != nil {
			t.Fatal(err)
		}
		if got := analyzer.New(SpecialParamShadow{}).Analyze(file, "tests/platform.zsh"); len(got) != 0 {
			t.Errorf("%q: diagnostics = %+v, want none", source, got)
		}
	}
}
