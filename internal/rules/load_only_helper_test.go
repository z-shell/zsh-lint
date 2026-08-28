package rules

import (
	"strings"
	"testing"

	"github.com/z-shell/zsh-lint/internal/analyzer"
	"github.com/z-shell/zsh-lint/internal/parse"
	"github.com/z-shell/zsh-lint/internal/projectconfig"
)

func TestLoadOnlyHelper(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want int
	}{
		{name: "leaked setup", src: "_example_setup() { :; }\n_example_setup\n", want: 1},
		{name: "removed setup", src: "_example_setup() { :; }\n_example_setup\nunfunction _example_setup\n"},
		{name: "persistent callback use", src: "_example_setup() { :; }\n_example_callback() { _example_setup; }\n_example_setup\n"},
		{name: "public function is not inferred", src: "example_setup() { :; }\nexample_setup\n"},
		{name: "private API without load role", src: "_example_refresh() { :; }\n_example_refresh\n"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			file, err := parse.Parse(strings.NewReader(test.src), "example.plugin.zsh")
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			context := sourceContext(projectconfig.KindPlugin, projectconfig.ProfileSourcedLibrary, "example")
			diagnostics := analyzer.New(LoadOnlyHelper{}).AnalyzeProject([]analyzer.ProjectInput{{
				File: file, Path: "example.plugin.zsh", Source: context,
			}})
			got := diagnosticsByID(diagnostics, "plugin/load-only-helper")
			if len(got) != test.want {
				t.Fatalf("findings = %+v, want %d", got, test.want)
			}
		})
	}
}
