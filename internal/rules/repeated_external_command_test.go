package rules

import (
	"strings"
	"testing"

	"github.com/z-shell/zsh-lint/internal/analyzer"
	"github.com/z-shell/zsh-lint/internal/diag"
	"github.com/z-shell/zsh-lint/internal/parse"
	"github.com/z-shell/zsh-lint/internal/projectconfig"
)

func TestRepeatedExternalCommand(t *testing.T) {
	tests := []struct {
		name    string
		source  string
		context projectconfig.SourceContext
		want    int
	}{
		{
			name:    "external command in completion loop",
			source:  "for item in one two; do git status --short; done\n",
			context: configuredSource(projectconfig.KindPlugin, projectconfig.ProfileAutoloadFunction, projectconfig.RoleCompletion),
			want:    1,
		},
		{
			name:    "zsh builtin is ignored",
			source:  "for item in one two; do print -r -- $item; done\n",
			context: configuredSource(projectconfig.KindPlugin, projectconfig.ProfileAutoloadFunction, projectconfig.RoleCompletion),
		},
		{
			name:    "non-completion source is outside contract",
			source:  "for item in one two; do git status --short; done\n",
			context: configuredSource(projectconfig.KindPlugin, projectconfig.ProfileAutoloadFunction, ""),
		},
		{
			name:   "unconfigured source is outside opt-in profile",
			source: "while true; do grep pattern file; done\n",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			file, err := parse.Parse(strings.NewReader(test.source), "completion")
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			diagnostics := analyzer.New(RepeatedExternalCommand{}).AnalyzeSource(file, "completion", test.context)
			if len(diagnostics) != test.want {
				t.Fatalf("diagnostics = %+v, want %d", diagnostics, test.want)
			}
			if test.want > 0 && diagnostics[0].Severity != diag.Info {
				t.Fatalf("severity = %v, want info", diagnostics[0].Severity)
			}
		})
	}
}
