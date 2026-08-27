package rules

import (
	"strings"
	"testing"

	"github.com/z-shell/zsh-lint/internal/analyzer"
	"github.com/z-shell/zsh-lint/internal/diag"
	"github.com/z-shell/zsh-lint/internal/parse"
	"github.com/z-shell/zsh-lint/internal/projectconfig"
)

func TestConfiguredPluginRuleApplicability(t *testing.T) {
	tests := []struct {
		name    string
		rule    analyzer.Rule
		source  string
		path    string
		context projectconfig.SourceContext
		want    int
	}{
		{
			name:    "autoload metadata activates function scoping outside functions path",
			rule:    FunctionScopedOptions{},
			source:  "rehash\n",
			path:    "autoload/handler",
			context: configuredSource(projectconfig.KindPlugin, projectconfig.ProfileAutoloadFunction, ""),
			want:    1,
		},
		{
			name:    "completion role retains autoload function scoping",
			rule:    FunctionScopedOptions{},
			source:  "rehash\n",
			path:    "completions/_example",
			context: configuredSource(projectconfig.KindPlugin, projectconfig.ProfileAutoloadFunction, projectconfig.RoleCompletion),
			want:    1,
		},
		{
			name:    "sourced metadata disables function scoping beneath functions path",
			rule:    FunctionScopedOptions{},
			source:  "rehash\n",
			path:    "functions/plugin.zsh",
			context: configuredSource(projectconfig.KindPlugin, projectconfig.ProfileSourcedLibrary, ""),
		},
		{
			name:    "tool metadata disables function scoping",
			rule:    FunctionScopedOptions{},
			source:  "rehash\n",
			path:    "functions/handler",
			context: configuredSource(projectconfig.KindTool, projectconfig.ProfileAutoloadFunction, ""),
		},
		{
			name:    "sourced plugin activates zero handling beneath functions path",
			rule:    ZeroHandling{},
			source:  "fpath+=( \"${0:h}/functions\" )\n",
			path:    "functions/plugin.zsh",
			context: configuredSource(projectconfig.KindPlugin, projectconfig.ProfileSourcedLibrary, ""),
			want:    1,
		},
		{
			name:    "autoload metadata disables zero handling",
			rule:    ZeroHandling{},
			source:  "fpath+=( \"${0:h}/functions\" )\n",
			path:    "plugin.zsh",
			context: configuredSource(projectconfig.KindPlugin, projectconfig.ProfileAutoloadFunction, ""),
		},
		{
			name:    "sourced annex defers unload presence to project validator",
			rule:    UnloadFunction{},
			source:  "add-zsh-hook precmd _example_precmd\n",
			path:    "annex.zsh",
			context: configuredSource(projectconfig.KindZiAnnex, projectconfig.ProfileSourcedLibrary, ""),
		},
		{
			name:    "library metadata disables unload lifecycle",
			rule:    UnloadFunction{},
			source:  "add-zsh-hook precmd _example_precmd\n",
			path:    "plugin.zsh",
			context: configuredSource(projectconfig.KindLibrary, projectconfig.ProfileSourcedLibrary, ""),
		},
		{
			name:    "sourced plugin activates fpath hygiene",
			rule:    FpathHygiene{},
			source:  "fpath=( \"${0:h}/functions\" )\n",
			path:    "plugin.zsh",
			context: configuredSource(projectconfig.KindPlugin, projectconfig.ProfileSourcedLibrary, ""),
			want:    1,
		},
		{
			name:    "application metadata disables fpath hygiene",
			rule:    FpathHygiene{},
			source:  "fpath=( \"${0:h}/functions\" )\n",
			path:    "plugin.zsh",
			context: configuredSource(projectconfig.KindApplication, projectconfig.ProfileSourcedLibrary, ""),
		},
		{
			name:    "theme is not inferred as plugin",
			rule:    FpathHygiene{},
			source:  "fpath=( \"${0:h}/functions\" )\n",
			path:    "theme.plugin.zsh",
			context: configuredSource(projectconfig.KindTheme, projectconfig.ProfileSourcedLibrary, ""),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			file, err := parse.Parse(strings.NewReader(test.source), test.path)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			diagnostics := analyzer.New(test.rule).AnalyzeSource(file, test.path, test.context)
			if got := len(diagnostics); got != test.want {
				t.Fatalf("diagnostics = %+v (count %d), want %d", diagnostics, got, test.want)
			}
		})
	}
}

func TestInactiveConfiguredRuleSuppressionIsKnown(t *testing.T) {
	const source = "# zsh-lint disable=plugin/zero-handling -- tool source\nprint ok\n"
	file, err := parse.Parse(strings.NewReader(source), "tool.zsh")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	ruleSet, err := ForProfile(CurrentProjectProfile)
	if err != nil {
		t.Fatalf("ForProfile: %v", err)
	}
	diagnostics := analyzer.New(ruleSet...).AnalyzeSource(
		file,
		"tool.zsh",
		configuredSource(projectconfig.KindTool, projectconfig.ProfileSourcedLibrary, ""),
	)
	if len(diagnostics) != 1 || diagnostics[0].RuleID != diag.RuleID("meta/unused-suppression") {
		t.Fatalf("diagnostics = %+v, want one unused suppression", diagnostics)
	}
	if strings.Contains(diagnostics[0].Message, "unknown rule ID") {
		t.Errorf("inactive registered rule was treated as unknown: %q", diagnostics[0].Message)
	}
}

func configuredSource(kind projectconfig.ProjectKind, profile projectconfig.Profile, role projectconfig.SourceRole) projectconfig.SourceContext {
	return projectconfig.SourceContext{
		ConfigVersion: projectconfig.CurrentVersion,
		ProjectKind:   kind,
		Profile:       profile,
		Role:          role,
	}
}
