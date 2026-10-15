package rules

import (
	"strings"
	"testing"

	"github.com/z-shell/zsh-lint/internal/analyzer"
	"github.com/z-shell/zsh-lint/internal/diag"
	"github.com/z-shell/zsh-lint/internal/parse"
	"github.com/z-shell/zsh-lint/internal/projectconfig"
)

func TestFpathHygiene(t *testing.T) {
	tests := []struct {
		name     string
		src      string
		path     string
		wantDiag int
	}{
		{
			name:     "scalar FPATH replacement removes existing paths",
			src:      "FPATH=/opt/plugin/functions\n",
			path:     "my-plugin.plugin.zsh",
			wantDiag: 1,
		},
		{
			name: "destructive overwrite of fpath",
			src: `fpath=( "${0:h}/functions" )
`,
			path:     "my-plugin.plugin.zsh",
			wantDiag: 1,
		},
		{
			name: "adding bin directory to fpath",
			src: `fpath+=( "${0:h}/bin" )
`,
			path:     "my-plugin.plugin.zsh",
			wantDiag: 1,
		},
		{
			name: "adding hardcoded home path to fpath",
			src: `fpath+=( "/home/alice/.zsh/functions" )
`,
			path:     "my-plugin.plugin.zsh",
			wantDiag: 1,
		},
		{
			name: "compliant append to fpath",
			src: `fpath+=( "${0:h}/functions" "${0:h}/completions" )
`,
			path:     "my-plugin.plugin.zsh",
			wantDiag: 0,
		},
		{
			name: "compliant prepend preserving fpath",
			src: `fpath=( "${0:h}/functions" $fpath )
`,
			path:     "my-plugin.plugin.zsh",
			wantDiag: 0,
		},
		{
			name: "indexed element removal preserves other paths",
			src: `fpath[$index]=()
`,
			path:     "my-plugin.plugin.zsh",
			wantDiag: 0,
		},
		{
			name: "indexed element replacement preserves other paths",
			src: `fpath[1]="${0:h}/functions"
`,
			path:     "my-plugin.plugin.zsh",
			wantDiag: 0,
		},
		{
			name: "suppressed destructive fpath assignment",
			src: `# zsh-lint disable=plugin/fpath-hygiene -- test runner isolation
fpath=( "${0:h}/functions" )
`,
			path:     "my-plugin.plugin.zsh",
			wantDiag: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			file, err := parse.Parse(strings.NewReader(tt.src), tt.path)
			if err != nil {
				t.Fatalf("Parse failed: %v", err)
			}
			diags := analyzer.New(FpathHygiene{}).Analyze(file, tt.path)
			var relevant diag.Diagnostics
			for _, d := range diags {
				if d.RuleID == "plugin/fpath-hygiene" {
					relevant = append(relevant, d)
				}
			}
			if len(relevant) != tt.wantDiag {
				t.Fatalf("got %d diagnostics, want %d: %v", len(relevant), tt.wantDiag, relevant)
			}
		})
	}
}

func TestFpathHygieneMutationForms(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want int
	}{
		{"global array declaration", "typeset -ga fpath=( /opt/functions )\n", 1},
		{"top-level array declaration", "typeset -a fpath=( /opt/functions )\n", 1},
		{"global scalar declaration", "typeset -g FPATH=/opt/functions\n", 1},
		{"top-level exported scalar", "typeset -x FPATH=/opt/functions\n", 1},
		{"temporary command prefix", "FPATH=/opt/functions print hello\n", 0},
		{"set clustered options", "set -eA fpath /opt/functions\n", 1},
		{"set named option", "set -o errexit -A fpath /opt/functions\n", 1},
		{"set attached array name", "set -Afpath /opt/functions\n", 1},
		{"set attached empty array", "set -Afpath\n", 1},
		{"set invalid option", "set -zA fpath /opt/functions\n", 0},
		{"set unknown array", "set -A other /opt/functions\n", 0},
		{"set without options", "set\n", 0},
		{"set dynamic named option", "set -o $option -A fpath /opt/functions\n", 0},
		{"set A consumes remaining letters as name", "set -Ae fpath /opt/functions\n", 0},
		{"set named-option data is not parsed as flags", "set -o A fpath /opt/functions\n", 0},
		{"declaration append", "typeset -ga fpath+=( /opt/functions )\n", 0},
		{"declaration retaining paths", "typeset -ga fpath=( /opt/functions $fpath )\n", 0},
		{"display declaration", "typeset -p fpath\n", 0},
		{"set array replacement", "set -A fpath /opt/functions\n", 1},
		{"set empty array", "set -A fpath\n", 1},
		{"set array retaining paths", "set -A fpath /opt/functions $fpath\n", 0},
		{"set array directory check", "set -A fpath /opt/bin $fpath\n", 1},
		{"builtin set array", "builtin set -A fpath /opt/functions\n", 1},
		{"external set is not builtin", "command set -A fpath /opt/functions\n", 0},
		{"set positional args", "set -- -A fpath /opt/functions\n", 0},
		{"set array preserves tail", "set +A fpath /opt/functions\n", 0},
		{"set another array", "set -A other /opt/functions\n", 0},
		{"scalar retaining paths", "FPATH=/opt/functions:$FPATH\n", 0},
		{"scalar append", "FPATH+=:/opt/functions\n", 0},
		{"tilde user directory", "fpath+=( ~/functions )\n", 1},
		{"tilde bin produces one path warning", "fpath+=( ~/functions/bin )\n", 1},
		{"quoted tilde is literal", "fpath+=( '~/functions' )\n", 0},
		{"colon-separated user directory", "FPATH+=:/home/alice/functions:/opt/functions\n", 1},
		{"declaration user directory", "typeset -ga fpath+=( ~/functions )\n", 1},
		{"set user directory", "set -A fpath ~/functions $fpath\n", 1},
		{"count does not preserve paths", "fpath=( ${#fpath} )\n", 1},
		{"set array values are not options", "set -A fpath -- /opt/functions\n", 1},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			file, err := parse.Parse(strings.NewReader(test.src), "plugin.zsh")
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			got := analyzer.New(FpathHygiene{}).Analyze(file, "plugin.zsh")
			if len(got) != test.want {
				t.Fatalf("diagnostics = %+v, want %d", got, test.want)
			}
		})
	}
}

func TestFpathHygienePrecision(t *testing.T) {
	tests := []struct {
		name string
		src  string
		path string
		want int
	}{
		{"saved array restore", "saved=( $fpath ); fpath=( $saved )\n", "plugin.zsh", 0},
		{"cross-file saved array restore", "fpath=( \"${_plugin_original_fpath[@]}\" )\n", "plugin.zsh", 0},
		{"filtered array restore", "fpath=( \"${new[@]}\" )\n", "plugin.zsh", 0},
		{"preserve unknown array among directories", "fpath=( /opt/functions $old )\n", "plugin.zsh", 0},
		{"set preserves unknown array", "set -A fpath /opt/functions $old\n", "plugin.zsh", 0},
		{"local array and assignment", "f() { local -a fpath; fpath=( /opt/functions ); }\n", "plugin.zsh", 0},
		{"local scalar ties array", "f() { local FPATH=/opt/functions; fpath=( /other/functions ); }\n", "plugin.zsh", 0},
		{"typeset local", "f() { typeset -a fpath=( /opt/functions ); }\n", "plugin.zsh", 0},
		{"local scope does not leak", "f() { local -a fpath; }; fpath=( /opt/functions )\n", "plugin.zsh", 1},
		{"mutation before local", "f() { fpath=( /opt/functions ); local -a fpath; }\n", "plugin.zsh", 1},
		{"global declaration in function", "f() { typeset -ga fpath=( /opt/functions ); }\n", "plugin.zsh", 1},
		{"local set array", "f() { local -a fpath; set -A fpath /opt/functions; }\n", "plugin.zsh", 0},
		{"conditional local cannot hide global mutation", "f() { false && local fpath; fpath=( /opt/functions ); }\n", "plugin.zsh", 1},
		{"local in then does not hide else mutation", "f() { if false; then local fpath; else fpath=( /opt/functions ); fi; }\n", "plugin.zsh", 1},
		{"local in then covers that branch", "f() { if true; then local fpath; fpath=( /opt/functions ); fi; }\n", "plugin.zsh", 0},
		{"one conditional branch cannot localize afterward", "f() { if false; then local fpath; fi; fpath=( /opt/functions ); }\n", "plugin.zsh", 1},
		{"loop local does not cover later mutation", "f() { for i in; do local fpath; done; fpath=( /opt/functions ); }\n", "plugin.zsh", 1},
		{"subshell local does not cover later mutation", "f() { (local fpath); fpath=( /opt/functions ); }\n", "plugin.zsh", 1},
		{"both branches localize", "f() { if condition; then local fpath; else local FPATH; fi; fpath=( /opt/functions ); }\n", "plugin.zsh", 0},
		{"while local does not cover later mutation", "f() { while false; do local fpath; done; fpath=( /opt/functions ); }\n", "plugin.zsh", 1},
		{"repeat local does not cover later mutation", "f() { repeat 0 do local fpath; done; fpath=( /opt/functions ); }\n", "plugin.zsh", 1},
		{"case local does not cover sibling", "f() { case x in a) local fpath;; b) fpath=( /opt/functions );; esac; }\n", "plugin.zsh", 1},
		{"exhaustive case localizes", "f() { case $1 in a) local fpath;; *) local FPATH;; esac; fpath=( /opt/functions ); }\n", "plugin.zsh", 0},
		{"nonexhaustive case cannot localize", "f() { case $1 in a) local fpath;; esac; fpath=( /opt/functions ); }\n", "plugin.zsh", 1},
		{"quoted star is not a default", "f() { case $1 in a) local fpath;; '*') local FPATH;; esac; fpath=( /opt/functions ); }\n", "plugin.zsh", 1},
		{"escaped star is not a default", "f() { case $1 in a) local fpath;; \\*) local FPATH;; esac; fpath=( /opt/functions ); }\n", "plugin.zsh", 1},
		{"case default alone is insufficient", "f() { case $1 in a) :;; *) local fpath;; esac; fpath=( /opt/functions ); }\n", "plugin.zsh", 1},
		{"command substitution local does not cover later mutation", "f() { print \"$(local fpath)\"; fpath=( /opt/functions ); }\n", "plugin.zsh", 1},
		{"background local does not cover later mutation", "f() { local fpath & fpath=( /opt/functions ); }\n", "plugin.zsh", 1},
		{"loop header still traversed", "for i in $(fpath=( /opt/functions )); do :; done\n", "plugin.zsh", 1},
		{"case word still traversed", "case $(fpath=( /opt/functions )) in x) :;; esac\n", "plugin.zsh", 1},
		{"global flag still uses an existing local", "f() { local -a fpath; typeset -ga fpath=( /opt/functions ); }\n", "plugin.zsh", 0},
		{"unconfigured test setup", "fpath=( /test/functions )\n", "tests/setup.zsh", 0},
		{"windows test setup", "fpath=( /test/functions )\n", `plugin\tests\setup.zsh`, 0},
		{"parameter pattern is not a directory", "fpath=( ${fpath:#/home/alice/functions} )\n", "plugin.zsh", 0},
		{"parameter suffix is not a directory", "fpath=( ${fpath%/bin} )\n", "plugin.zsh", 0},
		{"single quoted directory", "fpath+=( '/opt/bin' )\n", "plugin.zsh", 1},
		{"command args are not path text", "fpath+=( $(print /opt/bin) )\n", "plugin.zsh", 0},
		{"nested expansion is not preservation", "fpath=( /opt/functions $(print $fpath) )\n", "plugin.zsh", 1},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			file, err := parse.Parse(strings.NewReader(test.src), test.path)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			got := analyzer.New(FpathHygiene{}).Analyze(file, test.path)
			if len(got) != test.want {
				t.Fatalf("diagnostics = %+v, want %d", got, test.want)
			}
		})
	}
}

func TestFpathHygieneRangesAndSuppression(t *testing.T) {
	tests := []struct {
		src   string
		start int
	}{
		{"FPATH=/opt/functions\n", 0},
		{"typeset -ga fpath=( /opt/functions )\n", 12},
		{"set -A fpath /opt/functions\n", 7},
		{"fpath+=( ~/functions )\n", 9},
	}
	for _, test := range tests {
		t.Run(strings.TrimSpace(test.src), func(t *testing.T) {
			file, err := parse.Parse(strings.NewReader(test.src), "plugin.zsh")
			if err != nil {
				t.Fatal(err)
			}
			got := analyzer.New(FpathHygiene{}).Analyze(file, "plugin.zsh")
			if len(got) != 1 || got[0].RuleID != "plugin/fpath-hygiene" || got[0].Severity != diag.Warning {
				t.Fatalf("diagnostics = %+v, want one fpath Warning", got)
			}
			end := len(strings.TrimSuffix(test.src, "\n"))
			if strings.Contains(test.src, "~/") {
				end = test.start + len("~/functions")
			}
			if got[0].Range.Start.Offset != test.start || got[0].Range.End.Offset != end || got[0].Range.Start.Line != 1 || got[0].Range.Start.Column != test.start+1 {
				t.Fatalf("range = %+v, want offsets [%d,%d)", got[0].Range, test.start, end)
			}
			suppressed := "# zsh-lint disable=plugin/fpath-hygiene -- deliberate isolation\n" + test.src
			file, err = parse.Parse(strings.NewReader(suppressed), "plugin.zsh")
			if err != nil {
				t.Fatal(err)
			}
			if got := analyzer.New(FpathHygiene{}).Analyze(file, "plugin.zsh"); len(got) != 0 {
				t.Fatalf("suppressed diagnostics = %+v", got)
			}
		})
	}
}

func TestConfiguredFpathHygieneExecutionScope(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want int
	}{
		{
			name: "anonymous local search path",
			src:  "() { local -a fpath=( /opt/functions ); } argument\n",
		},
		{
			name: "anonymous global declaration",
			src:  "() { typeset -ga fpath=( /opt/functions ); } argument\n",
			want: 1,
		},
		{
			name: "set array entrypoint mutation",
			src:  "set -A fpath /opt/functions\n",
			want: 1,
		},
		{
			name: "scalar entrypoint mutation",
			src:  "FPATH=/opt/functions\n",
			want: 1,
		},
		{
			name: "direct top-level assignment",
			src:  "fpath=( /tmp/functions )\n",
			want: 1,
		},
		{
			name: "top-level conditional assignment",
			src:  "if true; then fpath=( /tmp/functions ); fi\n",
			want: 1,
		},
		{
			name: "named function definition is not executed",
			src:  "helper() { fpath=( /tmp/functions ) }\n",
		},
		{
			name: "invoked anonymous function is executed",
			src:  "() { fpath=( /tmp/functions ) } argument\n",
			want: 1,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			file, err := parse.Parse(strings.NewReader(test.src), "plugin.zsh")
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			got := analyzer.New(FpathHygiene{}).AnalyzeSource(
				file,
				"plugin.zsh",
				configuredSource(projectconfig.KindPlugin, projectconfig.ProfileSourcedLibrary, ""),
			)
			if len(got) != test.want {
				t.Fatalf("diagnostics = %+v, want %d", got, test.want)
			}
		})
	}
}
