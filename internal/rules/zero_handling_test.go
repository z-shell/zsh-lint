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
		{name: "initialization inside brace block", src: "{ 0=${(%):-%N}; print -r -- $0; }\nprint -r -- $0\n", path: "my-plugin.plugin.zsh"},
		{name: "canonical idiom inside brace block", src: "{ 0=${${ZERO:-${0:#$ZSH_ARGZERO}}:-${(%):-%N}}; 0=${${(M)0:#/*}:-$PWD/$0}; print -r -- $0; }\n", path: "my-plugin.plugin.zsh"},
		{name: "use before initialization inside block", src: "{ print -r -- $0; 0=${(%):-%N}; print -r -- $0; }\n", path: "my-plugin.plugin.zsh", wantDiag: 1},
		{name: "initialization inside if", src: "if true; then 0=${(%):-%N}; print -r -- $0; fi\n", path: "my-plugin.plugin.zsh"},
		{name: "canonical idiom inside if", src: "if true; then 0=${${ZERO:-${0:#$ZSH_ARGZERO}}:-${(%):-%N}}; 0=${${(M)0:#/*}:-$PWD/$0}; print -r -- $0; fi\n", path: "my-plugin.plugin.zsh"},
		{name: "conditional initialization does not escape", src: "if true; then 0=${(%):-%N}; print -r -- $0; fi\nprint -r -- $0\n", path: "my-plugin.plugin.zsh", wantDiag: 1},
		{name: "both if branches initialize", src: "if true; then 0=${(%):-%N}; else 0=$ZERO; fi\nprint -r -- $0\n", path: "my-plugin.plugin.zsh"},
		{name: "if branches keep separate state", src: "if true; then 0=${(%):-%N}; else print -r -- $0; fi\n", path: "my-plugin.plugin.zsh", wantDiag: 1},
		{name: "then-only initialization does not escape if else", src: "if true; then 0=$ZERO; else :; fi\nprint -r -- $0\n", path: "my-plugin.plugin.zsh", wantDiag: 1},
		{name: "else-only initialization does not escape if else", src: "if true; then :; else 0=$ZERO; fi\nprint -r -- $0\n", path: "my-plugin.plugin.zsh", wantDiag: 1},
		{name: "elif and else initialize", src: "if false; then 0=$ZERO; elif true; then 0=${(%):-%N}; else 0=${(%):-%x}; fi\nprint -r -- $0\n", path: "my-plugin.plugin.zsh"},
		{name: "condition use precedes branch initialization", src: "if [[ $0 == zsh ]]; then 0=$ZERO; fi\n", path: "my-plugin.plugin.zsh", wantDiag: 1},
		{name: "nested blocks and if", src: "{ if true; then { 0=$ZERO; print -r -- $0; }; fi; }\n", path: "my-plugin.plugin.zsh"},
		{name: "background block does not initialize caller", src: "{ 0=$ZERO; print -r -- $0; } &\nprint -r -- $0\n", path: "my-plugin.plugin.zsh", wantDiag: 1},
		{name: "function in block does not initialize caller", src: "{ helper() { 0=$ZERO; }; print -r -- $0; }\n", path: "my-plugin.plugin.zsh", wantDiag: 1},
		{name: "block redirection precedes initialization", src: "{ 0=$ZERO; } >\"${0:h}/log\"\n", path: "my-plugin.plugin.zsh", wantDiag: 1},
		{name: "if redirection precedes initialization", src: "if true; then 0=$ZERO; fi >\"${0:h}/log\"\n", path: "my-plugin.plugin.zsh", wantDiag: 1},
		{name: "ordinary zero assignment is not source initialization", src: "0=$value\nprint -r -- $0\n", path: "my-plugin.plugin.zsh", wantDiag: 1},
		{name: "ARGZERO filter is not a path use", src: "source_path=\"${ZERO:-${${0:#$ZSH_ARGZERO}:-${(%):-%N}}}\"\n", path: "my-plugin.plugin.zsh"},
		{name: "ARGZERO filter does not initialize later uses", src: "print -r -- ${0:#$ZSH_ARGZERO} $0\n", path: "my-plugin.plugin.zsh", wantDiag: 1},
		{name: "other filter is a zero use", src: "print -r -- ${0:#$OTHER}\n", path: "my-plugin.plugin.zsh", wantDiag: 1},
		{name: "ARGZERO length is a different filter", src: "print -r -- ${0:#${#ZSH_ARGZERO}}\n", path: "my-plugin.plugin.zsh", wantDiag: 1},
		{name: "match flag reverses ARGZERO filtering", src: "print -r -- ${(M)0:#$ZSH_ARGZERO}\n", path: "my-plugin.plugin.zsh", wantDiag: 1},
		{name: "subscripted zero is a different ARGZERO filter", src: "print -r -- ${0[1]:#$ZSH_ARGZERO}\n", path: "my-plugin.plugin.zsh", wantDiag: 1},
		{name: "subscript use of zero remains visible", src: "print -r -- ${0[$0]:#$ZSH_ARGZERO}\n", path: "my-plugin.plugin.zsh", wantDiag: 2},
		{name: "zero length is a different ARGZERO filter", src: "print -r -- ${#0:#$ZSH_ARGZERO}\n", path: "my-plugin.plugin.zsh", wantDiag: 1},
		{name: "nested background initializer does not initialize caller", src: "{ { 0=$ZERO; } & print -r -- $0; }\n", path: "my-plugin.plugin.zsh", wantDiag: 1},
		{name: "if condition initializes before branches", src: "if 0=$ZERO; then print -r -- $0; fi\nprint -r -- $0\n", path: "my-plugin.plugin.zsh"},
		{name: "initialized block redirection is accepted", src: "0=$ZERO\n{ print -r -- $0; } >\"${0:h}/log\"\n", path: "my-plugin.plugin.zsh"},
		{name: "ARGZERO pattern with literal text is a zero use", src: "print -r -- ${0:#prefix$ZSH_ARGZERO}\n", path: "my-plugin.plugin.zsh", wantDiag: 1},
		{name: "zero assignment word after command is not initialization", src: "print -r -- 0=$value\nprint -r -- $0\n", path: "my-plugin.plugin.zsh", wantDiag: 1},
		{name: "usage of 0 inside function body", src: "my_func() { print -r -- \"Function name: $0\" }\n", path: "my-plugin.plugin.zsh"},
		{name: "file inside functions directory", src: "print -r -- \"Arg zero: $0\"\n", path: "functions/.handler"},
		{name: "mixed case functions segment", src: "print -r -- $0\n", path: "plugin/Functions/handler"},
		{name: "test script", src: "print -r -- $0\n", path: "plugin/tests/check.zsh"},
		{name: "executable script", src: "#!/usr/bin/env zsh\nprint -r -- $0\n", path: "scripts/check.zsh"},
		{name: "shebang only at first line", src: "# comment\n#!/bin/zsh\nprint -r -- $0\n", path: "plugin.zsh", wantDiag: 1},
		{name: "windows test path", src: "print -r -- $0\n", path: `plugin\tests\check.zsh`},
		{name: "path segments must match completely", src: "print -r -- $0\n", path: "plugin/my-functions/tests.plugin.zsh", wantDiag: 1},
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
		path string
		want int
	}{
		{name: "top-level zero assignment is rejected", src: "0=\"${(%):-%N}\"\nfpath+=( \"${0:h}/functions\" )\n", want: 1},
		{name: "block zero assignment is rejected", src: "{ 0=$ZERO; print -r -- $0; }\n", want: 1},
		{name: "branch zero assignments are rejected", src: "if true; then 0=$ZERO; else 0=${(%):-%N}; fi\n", want: 2},
		{name: "configured tests path remains analyzed", src: "print -r -- $0\n", path: "tests/plugin.zsh", want: 1},
		{name: "configured functions path remains analyzed", src: "print -r -- $0\n", path: "Functions/plugin.zsh", want: 1},
		{name: "configured shebang remains analyzed", src: "#!/usr/bin/env zsh\nprint -r -- $0\n", want: 1},
		{name: "configured ARGZERO filter is accepted", src: "print -r -- ${0:#$ZSH_ARGZERO}\n"},
		{name: "zero assignment argument does not assign caller state", src: "print -r -- 0=value\n"},
		{name: "quoted zero assignment word does not assign caller state", src: "\"0=value\"\n"},
		{name: "canonical anonymous function argument is accepted", src: "() {\n  builtin emulate -L zsh\n  local -r source_path=${1:a}\n  local -r plugin_dir=${source_path:h}\n  fpath+=( \"${plugin_dir}/functions\" )\n} \"${ZERO:-${${0:#$ZSH_ARGZERO}:-${(%):-%N}}}\"\n"},
		{name: "named function zero is not entrypoint location", src: "helper() { print -r -- \"$0\" }\n"},
		{name: "literal tokens do not initialize zero", src: "print -r -- 'ZERO %N %x'\nfpath+=( \"${0:h}/functions\" )\n", want: 1},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path := test.path
			if path == "" {
				path = "plugin.zsh"
			}
			file, err := parse.Parse(strings.NewReader(test.src), path)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			diagnostics := analyzer.New(ZeroHandling{}).AnalyzeSource(
				file,
				path,
				configuredSource(projectconfig.KindPlugin, projectconfig.ProfileSourcedLibrary, ""),
			)
			if len(diagnostics) != test.want {
				t.Fatalf("diagnostics = %+v, want %d", diagnostics, test.want)
			}
		})
	}
}
