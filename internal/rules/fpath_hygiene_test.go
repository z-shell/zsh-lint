package rules

import (
	"strings"
	"testing"

	"github.com/z-shell/zsh-lint/internal/analyzer"
	"github.com/z-shell/zsh-lint/internal/diag"
	"github.com/z-shell/zsh-lint/internal/parse"
)

func TestFpathHygiene(t *testing.T) {
	tests := []struct {
		name     string
		src      string
		path     string
		wantDiag int
	}{
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
