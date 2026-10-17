package rules

import (
	"strings"
	"testing"

	"github.com/z-shell/zsh-lint/internal/analyzer"
	"github.com/z-shell/zsh-lint/internal/diag"
	"github.com/z-shell/zsh-lint/internal/parse"
	"github.com/z-shell/zsh-lint/internal/projectconfig"
)

func TestFunctionScopedOptionsInitializerMessage(t *testing.T) {
	const dynamic = "${=${options[xtrace]:#off}:+-o xtrace}"
	for _, test := range []struct {
		name, source string
		specific     bool
		line         int
	}{
		{"bare body", "builtin emulate -L zsh " + dynamic + "\nprint ready\n", true, 1},
		{"reset flags", "builtin emulate -LR zsh " + dynamic + "\n", true, 1},
		{"reset flags reversed", "builtin emulate -RL zsh " + dynamic + "\n", true, 1},
		{"wrapper", "handler() {\n builtin emulate -L zsh " + dynamic + "\n print ready\n}\n", true, 2},
		{"different dynamic", "builtin emulate -L zsh ${=other}\n", false, 1},
		{"literal source text", "print -r -- '" + dynamic + "'\n", false, 1},
		{"wrong emulation", "builtin emulate -L sh " + dynamic + "\n", false, 1},
		{"no local flag", "builtin emulate zsh " + dynamic + "\n", false, 1},
		{"listing mode", "builtin emulate -lL zsh " + dynamic + "\n", false, 1},
		{"concatenated suffix", "builtin emulate -L zsh " + dynamic + "suffix\n", false, 1},
		{"safe splitting remains conservative", "builtin emulate -L zsh ${(s: :)${options[xtrace]:#off}:+-o xtrace}\n", false, 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			file, err := parse.Parse(strings.NewReader(test.source), "functions/handler")
			if err != nil {
				t.Fatal(err)
			}
			for _, context := range []projectconfig.SourceContext{{}, sourceContext(projectconfig.KindPlugin, projectconfig.ProfileAutoloadFunction, "example")} {
				got := analyzer.New(FunctionScopedOptions{}).AnalyzeSource(file, "functions/handler", context)
				if len(got) != 1 || got[0].RuleID != "plugin/function-scoped-options" || got[0].Severity != diag.Hint || got[0].Range.Start.Line != test.line {
					t.Fatalf("diagnostics = %+v, want one unchanged positioned Hint", got)
				}
				wantMessage := "Scope function options with 'builtin emulate -L zsh' or 'setopt local_options' before executable logic"
				if test.specific {
					wantMessage = "The xtrace initializer splits on inherited IFS and can make emulate fail before options are localized; use static 'builtin emulate -L zsh'"
				}
				if got[0].Message != wantMessage {
					t.Fatalf("message = %q, want %q", got[0].Message, wantMessage)
				}
				lines := strings.Split(test.source, "\n")
				offset := 0
				for _, line := range lines[:test.line-1] {
					offset += len(line) + 1
				}
				line := lines[test.line-1]
				indent := len(line) - len(strings.TrimLeft(line, " "))
				wantRange := diag.Range{
					Start: diag.Position{Line: test.line, Column: indent + 1, Offset: offset + indent},
					End:   diag.Position{Line: test.line, Column: len(line) + 1, Offset: offset + len(line)},
				}
				if got[0].Range != wantRange {
					t.Fatalf("range = %+v, want %+v", got[0].Range, wantRange)
				}
			}
		})
	}
	for _, source := range []string{
		"# zsh-lint disable=plugin/function-scoped-options -- known caller IFS\nbuiltin emulate -L zsh " + dynamic + "\n",
		"# zsh-lint disable=plugin/function-scoped-options -- known caller IFS\nhandler() {\n builtin emulate -L zsh " + dynamic + "\n}\n",
	} {
		if got := analyzeFunctionScopedOptions(t, "functions/handler", source); len(got) != 0 {
			t.Fatalf("suppression changed: %v", got)
		}
	}
}
