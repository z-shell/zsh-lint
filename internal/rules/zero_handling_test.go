package rules

import (
	"strings"
	"testing"

	"github.com/z-shell/zsh-lint/internal/analyzer"
	"github.com/z-shell/zsh-lint/internal/diag"
	"github.com/z-shell/zsh-lint/internal/parse"
	"github.com/z-shell/zsh-lint/internal/projectconfig"
)

func TestZeroHandling(t *testing.T) {
	tests := []struct {
		name     string
		src      string
		path     string
		wantDiag int
	}{
		{name: "uninitialized 0 in fpath addition", src: "fpath+=( \"${0:h}/functions\" )\n", path: "my-plugin.plugin.zsh", wantDiag: 1},
		{name: "uninitialized 0 in variable assignment", src: "PLUGIN_DIR=\"${0:h}\"\n", path: "my-plugin.plugin.zsh", wantDiag: 1},
		{
			name: "compliant ZERO idiom initialization",
			src:  "0=\"${ZERO:-${${0:#$ZSH_ARGZERO}:-${(%):-%N}}}\"\n0=\"${${(M)0:#/*}:-$PWD/$0}\"\nfpath+=( \"${0:h}/functions\" )\n",
			path: "my-plugin.plugin.zsh",
		},
		{name: "compliant prompt expansion initialization", src: "0=\"${(%):-%N}\"\nfpath+=( \"${0:h}/functions\" )\n", path: "my-plugin.plugin.zsh"},
		{name: "usage of 0 inside function body", src: "my_func() { print -r -- \"Function name: $0\" }\n", path: "my-plugin.plugin.zsh"},
		{name: "file inside functions directory", src: "print -r -- \"Arg zero: $0\"\n", path: "functions/.handler"},
		{name: "suppressed finding", src: "# zsh-lint disable=plugin/zero-handling -- direct execution script\nfpath+=( \"${0:h}/functions\" )\n", path: "my-plugin.plugin.zsh"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			file, err := parse.Parse(strings.NewReader(test.src), test.path)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			diagnostics := analyzer.New(ZeroHandling{}).Analyze(file, test.path)
			var relevant diag.Diagnostics
			for _, diagnostic := range diagnostics {
				if diagnostic.RuleID == "plugin/zero-handling" {
					relevant = append(relevant, diagnostic)
				}
			}
			if len(relevant) != test.wantDiag {
				t.Fatalf("diagnostics = %+v, want %d", relevant, test.wantDiag)
			}
		})
	}
}

func TestConfiguredZeroHandlingPreservesCallerState(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want int
	}{
		{name: "top-level zero assignment is rejected", src: "0=\"${(%):-%N}\"\nfpath+=( \"${0:h}/functions\" )\n", want: 1},
		{name: "canonical anonymous function argument is accepted", src: "() {\n  builtin emulate -L zsh\n  local -r source_path=${1:a}\n  local -r plugin_dir=${source_path:h}\n  fpath+=( \"${plugin_dir}/functions\" )\n} \"${ZERO:-${${0:#$ZSH_ARGZERO}:-${(%):-%N}}}\"\n"},
		{name: "named function zero is not entrypoint location", src: "helper() { print -r -- \"$0\" }\n"},
		{name: "literal tokens do not initialize zero", src: "print -r -- 'ZERO %N %x'\nfpath+=( \"${0:h}/functions\" )\n", want: 1},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			file, err := parse.Parse(strings.NewReader(test.src), "plugin.zsh")
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			diagnostics := analyzer.New(ZeroHandling{}).AnalyzeSource(
				file,
				"plugin.zsh",
				configuredSource(projectconfig.KindPlugin, projectconfig.ProfileSourcedLibrary, ""),
			)
			if len(diagnostics) != test.want {
				t.Fatalf("diagnostics = %+v, want %d", diagnostics, test.want)
			}
		})
	}
}
