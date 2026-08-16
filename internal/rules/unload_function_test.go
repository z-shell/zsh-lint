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
		wantDiag int
		wantSev  diag.Severity
	}{
		{
			name: "hook registered without unload function",
			src: `autoload -Uz add-zsh-hook
add-zsh-hook precmd _my_precmd
`,
			path:     "my-plugin.plugin.zsh",
			wantDiag: 1,
			wantSev:  diag.Hint,
		},
		{
			name: "unload function missing self-unfunction",
			src: `my_plugin_unload() {
  autoload -Uz add-zsh-hook
  add-zsh-hook -d precmd _my_precmd
  unfunction _my_precmd
}
`,
			path:     "my-plugin.plugin.zsh",
			wantDiag: 1,
			wantSev:  diag.Hint,
		},
		{
			name: "unload function with indiscriminate function wipe",
			src: `my_plugin_unload() {
  unfunction ${(k)functions}
  unfunction my_plugin_unload
}
`,
			path:     "my-plugin.plugin.zsh",
			wantDiag: 1,
			wantSev:  diag.Warning,
		},
		{
			name: "compliant hook and unload function",
			src: `autoload -Uz add-zsh-hook
add-zsh-hook precmd _my_precmd

my_plugin_unload() {
  autoload -Uz add-zsh-hook
  add-zsh-hook -d precmd _my_precmd
  unfunction _my_precmd my_plugin_unload
}
`,
			path:     "my-plugin.plugin.zsh",
			wantDiag: 0,
		},
		{
			name: "file inside functions directory",
			src: `add-zsh-hook precmd _my_precmd
`,
			path:     "functions/.handler",
			wantDiag: 0,
		},
		{
			name: "suppressed hook finding",
			src: `# zsh-lint disable=plugin/unload-function -- static plugin
add-zsh-hook precmd _my_precmd
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
			diags := analyzer.New(UnloadFunction{}).Analyze(file, tt.path)
			var relevant diag.Diagnostics
			for _, d := range diags {
				if d.RuleID == "plugin/unload-function" {
					relevant = append(relevant, d)
				}
			}
			if len(relevant) != tt.wantDiag {
				t.Fatalf("got %d diagnostics, want %d: %v", len(relevant), tt.wantDiag, relevant)
			}
			if tt.wantDiag > 0 && tt.wantSev != 0 {
				if relevant[0].Severity != tt.wantSev {
					t.Errorf("severity = %v, want %v", relevant[0].Severity, tt.wantSev)
				}
			}
		})
	}
}
