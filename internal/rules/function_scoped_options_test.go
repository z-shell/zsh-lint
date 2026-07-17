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
			name: "accepts redirected emulate",
			path: "functions/handler",
			src:  "emulate -L zsh 2>/dev/null\nrehash\n",
		},
		{
			name:      "rejects background emulate",
			path:      "functions/handler",
			src:       "emulate -L zsh &\nrehash\n",
			wantLines: []int{1},
		},
		{
			name:      "rejects emulate command mode",
			path:      "functions/handler",
			src:       "emulate -L zsh -c true\nrehash\n",
			wantLines: []int{1},
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
			name: "accepts redirected setopt",
			path: "functions/handler",
			src:  "setopt local_options >/dev/null\nrehash\n",
		},
		{
			name: "accepts negated setopt",
			path: "functions/handler",
			src:  "! setopt local_options\nrehash\n",
		},
		{
			name:      "rejects background setopt",
			path:      "functions/handler",
			src:       "setopt local_options &\nrehash\n",
			wantLines: []int{1},
		},
		{
			name:      "rejects disabling local options",
			path:      "functions/handler",
			src:       "setopt +o local_options\nrehash\n",
			wantLines: []int{1},
		},
		{
			name:      "rejects inverse local options after enabling",
			path:      "functions/handler",
			src:       "setopt local_options no_local_options\nrehash\n",
			wantLines: []int{1},
		},
		{
			name: "accepts local options after inverse",
			path: "functions/handler",
			src:  "setopt no_local_options local_options\nrehash\n",
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

func TestFunctionScopedOptionsLeadingGuards(t *testing.T) {
	tests := []struct {
		name      string
		src       string
		wantLines []int
	}{
		{
			name: "accepts or-return guard",
			src:  "(( $+commands[eza] )) || return 1\nemulate -L zsh\nrehash\n",
		},
		{
			name: "accepts builtin return guard",
			src:  "(( $+commands[eza] )) || builtin return 1\nsetopt local_options\nrehash\n",
		},
		{
			name: "accepts bare return guard",
			src:  "(( $+commands[eza] )) || return\nemulate -L zsh\nrehash\n",
		},
		{
			name: "accepts option-terminated literal return status",
			src:  "(( $+commands[eza] )) || return -- 1\nemulate -L zsh\nrehash\n",
		},
		{
			name:      "rejects too many literal return arguments",
			src:       "(( $+commands[eza] )) || return 1 ignored\nemulate -L zsh\n",
			wantLines: []int{1},
		},
		{
			name: "accepts negated guard condition",
			src:  "! (( $+commands[eza] )) || return 1\nemulate -L zsh\nrehash\n",
		},
		{
			name: "accepts single-return if guard",
			src:  "if [[ $TERM == dumb ]]; then\n  return 0\nfi\nemulate -L zsh\nrehash\n",
		},
		{
			name: "accepts contiguous leading guards",
			src:  "(( $+commands[eza] )) || return 1\nif [[ $TERM == dumb ]]; then\n  builtin return 0\nfi\nemulate -L zsh\nrehash\n",
		},
		{
			name:      "rejects non-return or branch",
			src:       "(( $+commands[eza] )) || print missing\nemulate -L zsh\n",
			wantLines: []int{1},
		},
		{
			name:      "rejects assigned return branch",
			src:       "(( $+commands[eza] )) || status=1 return 1\nemulate -L zsh\n",
			wantLines: []int{1},
		},
		{
			name:      "rejects background return branch",
			src:       "(( $+commands[eza] )) || return 1 &\nemulate -L zsh\n",
			wantLines: []int{1},
		},
		{
			name:      "rejects redirected return branch",
			src:       "(( $+commands[eza] )) || return 1 >/dev/null\nemulate -L zsh\n",
			wantLines: []int{1},
		},
		{
			name:      "rejects negated return branch",
			src:       "(( $+commands[eza] )) || ! return 1\nemulate -L zsh\n",
			wantLines: []int{1},
		},
		{
			name:      "rejects if body with work before return",
			src:       "if [[ $TERM == dumb ]]; then\n  print skipped\n  return 0\nfi\nemulate -L zsh\n",
			wantLines: []int{1},
		},
		{
			name:      "rejects if with else branch",
			src:       "if [[ $TERM == dumb ]]; then\n  return 0\nelse\n  return 1\nfi\nemulate -L zsh\n",
			wantLines: []int{1},
		},
		{
			name:      "rejects if with elif branch",
			src:       "if [[ $TERM == dumb ]]; then\n  return 0\nelif [[ $TERM == unknown ]]; then\n  return 1\nfi\nemulate -L zsh\n",
			wantLines: []int{1},
		},
		{
			name:      "reports first statement after accepted guard",
			src:       "(( $+commands[eza] )) || return 1\nprint preparing\nemulate -L zsh\n",
			wantLines: []int{2},
		},
		{
			name:      "later guard does not rescue ordinary statement",
			src:       "print preparing\n(( $+commands[eza] )) || return 1\nemulate -L zsh\n",
			wantLines: []int{1},
		},
		{
			name:      "rejects dynamic return status",
			src:       "(( $+commands[eza] )) || return \"$status\"\nemulate -L zsh\n",
			wantLines: []int{1},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := analyzeFunctionScopedOptions(t, "functions/handler", test.src)
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

func TestFunctionScopedOptionsSuppression(t *testing.T) {
	src := `# zsh-lint disable=plugin/function-scoped-options -- intentionally inherits caller options
rehash
`
	file, err := parse.Parse(strings.NewReader(src), "functions/handler")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	diags := analyzer.New(FunctionScopedOptions{}).Analyze(file, "functions/handler")
	if len(diags) != 0 {
		t.Fatalf("suppressed diagnostics = %v, want none", diags)
	}
}
