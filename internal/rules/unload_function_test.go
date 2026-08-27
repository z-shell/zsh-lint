package rules

import (
	"strings"
	"testing"

	"github.com/z-shell/zsh-lint/internal/analyzer"
	"github.com/z-shell/zsh-lint/internal/diag"
	"github.com/z-shell/zsh-lint/internal/parse"
)

func TestUnloadFunction(t *testing.T) {
	tests := []struct {
		name     string
		src      string
		path     string
		want     int
		severity diag.Severity
	}{
		{name: "hook registered without unload function", src: "autoload -Uz add-zsh-hook\nadd-zsh-hook precmd _my_precmd\n", path: "my-plugin.plugin.zsh", want: 1, severity: diag.Hint},
		{name: "unload function missing self-unfunction", src: "my_plugin_unload() {\n  add-zsh-hook -d precmd _my_precmd\n  unfunction _my_precmd\n}\n", path: "my-plugin.plugin.zsh", want: 1, severity: diag.Hint},
		{name: "unload function with indiscriminate function wipe", src: "my_plugin_unload() {\n  unfunction ${(k)functions}\n  unfunction my_plugin_unload\n}\n", path: "my-plugin.plugin.zsh", want: 1, severity: diag.Warning},
		{name: "compliant hook and unload function", src: "add-zsh-hook precmd _my_precmd\nmy_plugin_unload() {\n  add-zsh-hook -d precmd _my_precmd\n  unfunction _my_precmd my_plugin_unload\n}\n", path: "my-plugin.plugin.zsh"},
		{name: "file inside functions directory", src: "add-zsh-hook precmd _my_precmd\n", path: "functions/.handler"},
		{name: "suppressed hook finding", src: "# zsh-lint disable=plugin/unload-function -- static plugin\nadd-zsh-hook precmd _my_precmd\n", path: "my-plugin.plugin.zsh"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			file, err := parse.Parse(strings.NewReader(test.src), test.path)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			diagnostics := analyzer.New(UnloadFunction{}).Analyze(file, test.path)
			if len(diagnostics) != test.want {
				t.Fatalf("diagnostics = %+v, want %d", diagnostics, test.want)
			}
			if test.want > 0 && diagnostics[0].Severity != test.severity {
				t.Fatalf("severity = %v, want %v", diagnostics[0].Severity, test.severity)
			}
		})
	}
}

func TestUnloadFunctionExactLifecycleMatching(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want int
	}{
		{name: "near-miss unload suffix does not satisfy lifecycle", src: "add-zsh-hook precmd _tick\nexample_unload() { unfunction example_unload }\n", want: 1},
		{name: "substring does not satisfy self removal", src: "example_plugin_unload() { unfunction prefix-example_plugin_unload-suffix }\n", want: 1},
		{name: "exact parameter satisfies self removal", src: "example_plugin_unload() { unfunction \"$0\" }\n"},
		{name: "zle widget registration requires unload", src: "zle -N example-widget example_widget\n", want: 1},
		{name: "zle widget deletion is not registration", src: "zle -D example-widget\n"},
		{name: "hook deletion is not registration", src: "add-zsh-hook -d precmd _tick\n"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			file, err := parse.Parse(strings.NewReader(test.src), "plugin.zsh")
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			diagnostics := analyzer.New(UnloadFunction{}).Analyze(file, "plugin.zsh")
			if len(diagnostics) != test.want {
				t.Fatalf("diagnostics = %+v, want %d", diagnostics, test.want)
			}
		})
	}
}
