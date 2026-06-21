package rules

import (
	"strings"
	"testing"

	"github.com/z-shell/zsh-lint/internal/analyzer"
	"github.com/z-shell/zsh-lint/internal/parse"
)

func analyzeFunctionScopedOptions(t *testing.T, path, src string) []int {
	t.Helper()

	file, err := parse.Parse(strings.NewReader(src), path)
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}

	diags := analyzer.New(FunctionScopedOptions{}).Analyze(file, path)
	lines := make([]int, len(diags))
	for i, diagnostic := range diags {
		if diagnostic.RuleID != "plugin/function-scoped-options" {
			t.Fatalf("diagnostic %d rule ID = %q", i, diagnostic.RuleID)
		}
		lines[i] = diagnostic.Range.Start.Line
	}
	return lines
}

func TestFunctionScopedOptionsStatementOrder(t *testing.T) {
	tests := []struct {
		name      string
		path      string
		src       string
		wantLines []int
	}{
		{
			name:      "reports executable function file",
			path:      "functions/.my-widget",
			src:       "local -a matches\nmatches=( $~pattern )\n",
			wantLines: []int{1},
		},
		{
			name:      "reports absolute function path",
			path:      "/checkout/plugin/functions/handler",
			src:       "rehash\n",
			wantLines: []int{1},
		},
		{
			name:      "reports windows-separated function path",
			path:      `C:\checkout\plugin\functions\handler`,
			src:       "rehash\n",
			wantLines: []int{1},
		},
		{
			name: "ignores sourced library",
			path: "lib/helpers.zsh",
			src:  "rehash\n",
		},
		{
			name: "ignores partial directory-name match",
			path: "my-functions/handler",
			src:  "rehash\n",
		},
		{
			name: "accepts builtin emulate",
			path: "functions/handler",
			src:  "builtin emulate -L zsh\nrehash\n",
		},
		{
			name: "accepts bare emulate",
			path: "functions/handler",
			src:  "emulate -L zsh\nrehash\n",
		},
		{
			name: "accepts grouped emulate flags and trailing argument",
			path: "functions/handler",
			src:  "builtin emulate -LR zsh -o extended_glob\nrehash\n",
		},
		{
			name:      "rejects dynamic emulate trailing argument",
			path:      "functions/handler",
			src:       "emulate -L zsh \"$dynamic\"\nrehash\n",
			wantLines: []int{1},
		},
		{
			name:      "rejects emulate without local flag",
			path:      "functions/handler",
			src:       "emulate -R zsh\nrehash\n",
			wantLines: []int{1},
		},
		{
			name:      "rejects emulate for another shell",
			path:      "functions/handler",
			src:       "emulate -L sh\nrehash\n",
			wantLines: []int{1},
		},
		{
			name: "accepts normalized local options",
			path: "functions/handler",
			src:  "setopt LOCAL_OPTIONS extended_glob\nrehash\n",
		},
		{
			name: "accepts builtin setopt and compact option name",
			path: "functions/handler",
			src:  "builtin setopt localoptions\nrehash\n",
		},
		{
			name:      "rejects dynamic setopt argument",
			path:      "functions/handler",
			src:       "setopt \"$dynamic\" local_options\nrehash\n",
			wantLines: []int{1},
		},
		{
			name:      "reports command before later scoping",
			path:      "functions/handler",
			src:       "print preparing\nemulate -L zsh\n",
			wantLines: []int{1},
		},
		{
			name:      "reports assignment before later scoping",
			path:      "functions/handler",
			src:       "mode=fast\nemulate -L zsh\n",
			wantLines: []int{1},
		},
		{
			name: "ignores empty file",
			path: "functions/handler",
		},
		{
			name: "ignores comment-only file",
			path: "functions/handler",
			src:  "# no executable logic\n",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := analyzeFunctionScopedOptions(t, test.path, test.src)
			if len(got) != len(test.wantLines) {
				t.Fatalf("diagnostic lines = %v, want %v", got, test.wantLines)
			}
			for i := range got {
				if got[i] != test.wantLines[i] {
					t.Errorf("diagnostic lines = %v, want %v", got, test.wantLines)
				}
			}
		})
	}
}
