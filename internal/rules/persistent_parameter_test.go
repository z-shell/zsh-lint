package rules

import (
	"strings"
	"testing"

	"github.com/z-shell/zsh-lint/internal/analyzer"
	"github.com/z-shell/zsh-lint/internal/diag"
	"github.com/z-shell/zsh-lint/internal/parse"
	"github.com/z-shell/zsh-lint/internal/projectconfig"
)

func TestPersistentParameterNamespace(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want int
	}{
		{name: "private typed state", src: "typeset -gA _example_state\n"},
		{name: "private top-level state", src: "typeset _example_state=ready\n"},
		{name: "public configuration global", src: "typeset -g EXAMPLE_FEATURES=all\n", want: 1},
		{name: "namespaced public global", src: "typeset -g example_features=all\n", want: 1},
		{name: "bare top-level assignment", src: "feature_mode=all\n", want: 1},
		{name: "temporary command assignment", src: "feature_mode=all command print ok\n"},
		{name: "explicit global in function", src: "example_set() { typeset -g feature_mode=all; }\n", want: 1},
		{name: "ordinary function local", src: "example_set() { local feature_mode=all; }\n"},
		{name: "path contract handled elsewhere", src: "fpath+=( functions )\n"},
		{name: "hyphenated identifier", src: "typeset -gA _zsh_fancy_completions_state\n"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			identifier := "example"
			if test.name == "hyphenated identifier" {
				identifier = "zsh-fancy-completions"
			}
			diagnostics := analyzePersistentParameters(t, test.src, identifier)
			got := diagnosticsByID(diagnostics, "plugin/persistent-parameter-namespace")
			if len(got) != test.want {
				t.Fatalf("findings = %+v, want %d", got, test.want)
			}
			if len(got) != 0 && got[0].Severity != diag.Warning {
				t.Errorf("severity = %v, want warning", got[0].Severity)
			}
		})
	}
}

func TestSharedPluginsRegistry(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want int
	}{
		{name: "global declaration", src: "typeset -gA Plugins\n", want: 1},
		{name: "indexed write", src: "Plugins[EXAMPLE_DIR]=$PWD\n", want: 1},
		{name: "function global write", src: "example_reset() { Plugins[EXAMPLE_DIR]=$PWD; }\n", want: 1},
		{name: "explicit function global", src: "example_reset() { typeset -gA Plugins; }\n", want: 1},
		{name: "function local", src: "example_print() { local -A Plugins; Plugins[item]=value; }\n"},
		{name: "temporary command assignment", src: "Plugins=value command print ok\n"},
		{name: "read only", src: "print -r -- $Plugins[EXAMPLE_DIR]\n"},
		{name: "project state", src: "typeset -gA _example_state\n"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			file, err := parse.Parse(strings.NewReader(test.src), "example.plugin.zsh")
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			context := sourceContext(projectconfig.KindPlugin, projectconfig.ProfileSourcedLibrary, "example")
			diagnostics := analyzer.New(SharedPluginsRegistry{}).AnalyzeSource(file, "example.plugin.zsh", context)
			got := diagnosticsByID(diagnostics, "plugin/shared-plugins-registry")
			if len(got) != test.want {
				t.Fatalf("findings = %+v, want %d", got, test.want)
			}
		})
	}
}

func analyzePersistentParameters(t *testing.T, source, identifier string) diag.Diagnostics {
	t.Helper()
	file, err := parse.Parse(strings.NewReader(source), "example.plugin.zsh")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	context := sourceContext(projectconfig.KindPlugin, projectconfig.ProfileSourcedLibrary, identifier)
	return analyzer.New(PersistentParameterNamespace{}).AnalyzeSource(file, "example.plugin.zsh", context)
}
