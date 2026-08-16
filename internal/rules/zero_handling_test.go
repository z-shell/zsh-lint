package rules

import (
	"strings"
	"testing"

	"github.com/z-shell/zsh-lint/internal/analyzer"
	"github.com/z-shell/zsh-lint/internal/diag"
	"github.com/z-shell/zsh-lint/internal/parse"
)

func TestZeroHandling(t *testing.T) {
	tests := []struct {
		name     string
		src      string
		path     string
		wantDiag int
	}{
		{
			name: "uninitialized 0 in fpath addition",
			src: `fpath+=( "${0:h}/functions" )
`,
			path:     "my-plugin.plugin.zsh",
			wantDiag: 1,
		},
		{
			name: "uninitialized 0 in variable assignment",
			src: `PLUGIN_DIR="${0:h}"
`,
			path:     "my-plugin.plugin.zsh",
			wantDiag: 1,
		},
		{
			name: "compliant ZERO idiom initialization",
			src: `0="${ZERO:-${${0:#$ZSH_ARGZERO}:-${(%):-%N}}}"
0="${${(M)0:#/*}:-$PWD/$0}"
fpath+=( "${0:h}/functions" )
`,
			path:     "my-plugin.plugin.zsh",
			wantDiag: 0,
		},
		{
			name: "compliant prompt expansion initialization",
			src: `0="${(%):-%N}"
fpath+=( "${0:h}/functions" )
`,
			path:     "my-plugin.plugin.zsh",
			wantDiag: 0,
		},
		{
			name: "usage of 0 inside function body",
			src: `my_func() {
  print -r -- "Function name: $0"
}
`,
			path:     "my-plugin.plugin.zsh",
			wantDiag: 0,
		},
		{
			name: "file inside functions directory",
			src: `print -r -- "Arg zero: $0"
`,
			path:     "functions/.handler",
			wantDiag: 0,
		},
		{
			name: "suppressed finding",
			src: `# zsh-lint disable=plugin/zero-handling -- direct execution script
fpath+=( "${0:h}/functions" )
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
			diags := analyzer.New(ZeroHandling{}).Analyze(file, tt.path)
			var relevant diag.Diagnostics
			for _, d := range diags {
				if d.RuleID == "plugin/zero-handling" {
					relevant = append(relevant, d)
				}
			}
			if len(relevant) != tt.wantDiag {
				t.Errorf("got %d diagnostics, want %d: %v", len(relevant), tt.wantDiag, relevant)
			}
		})
	}
}
