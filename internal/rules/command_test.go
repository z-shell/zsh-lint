package rules

import (
	"strings"
	"testing"

	"github.com/z-shell/zsh-lint/internal/parse"
	"mvdan.cc/sh/v3/syntax"
)

func commandForms(name string) []string {
	return []string{name, "builtin " + name, "command " + name, "command -p " + name,
		`\` + name, `"` + name + `"`, "'" + name + "'", "builtin command -p " + name}
}

func TestEffectiveCommand(t *testing.T) {
	tests := []struct {
		source string
		name   string
		args   int
	}{
		{`eval$x arg`, "", 1},
		{`"$cmd" arg`, "", 1},
		{`builtin $cmd arg`, "", 1},
		{`e"val" arg`, "eval", 1},
		{`command -v eval`, "-v", 1},
		{`command -V eval`, "-V", 1},
		{`builtin`, "", 0},
		{`command -p`, "", 0},
		{`value=x`, "", 0},
		{`\\eval arg`, `\eval`, 1},
	}
	for _, form := range commandForms("eval") {
		tests = append(tests, struct {
			source string
			name   string
			args   int
		}{form + " first second", "eval", 2})
	}
	for _, test := range tests {
		t.Run(test.source, func(t *testing.T) {
			file, err := parse.Parse(strings.NewReader(test.source), "test.zsh")
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			call, ok := file.AST().Stmts[0].Cmd.(*syntax.CallExpr)
			if !ok {
				t.Fatal("expected a call")
			}
			name, args := effectiveCommand(call)
			if name != test.name || len(args) != test.args {
				t.Fatalf("command = %q, args = %d; want %q, %d", name, len(args), test.name, test.args)
			}
			if len(args) > 0 && args[0] != call.Args[len(call.Args)-len(args)] {
				t.Fatal("arguments must retain their original word nodes")
			}
		})
	}
	if name, args := effectiveCommand(nil); name != "" || args != nil {
		t.Fatalf("nil call = %q, %v", name, args)
	}
}
