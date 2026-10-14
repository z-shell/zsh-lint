package rules

import (
	"strings"
	"testing"

	"github.com/z-shell/zsh-lint/internal/parse"
	"mvdan.cc/sh/v3/syntax"
)

// commandForms lists ways to run name as a builtin or function: plain, after
// the precommand modifiers Zsh strips (zshmisc, Precommand Modifiers), and
// with quoting that quote removal undoes. Each form runs name under zsh -f.
func commandForms(name string) []string {
	forms := []string{name, "builtin " + name, `\` + name, `"` + name + `"`, "'" + name + "'",
		"noglob " + name, "- " + name, "exec " + name, "exec -a argv0 " + name, "nocorrect " + name,
		"noglob builtin " + name, "builtin noglob " + name, "nocorrect noglob - builtin " + name}
	if len(name) > 1 {
		forms = append(forms, name[:1]+`\`+name[1:])
	}
	return forms
}

// externalCommandForms adds the command modifier forms, which run name only as
// an external command, to commandForms.
func externalCommandForms(name string) []string {
	return append(commandForms(name), "command "+name, "command -p "+name, "command -- "+name,
		"builtin command -p "+name, "noglob command "+name)
}

// builtinOnlyCommandForms run no builtin or function named name: the command
// modifier looks up only external commands (posix_builtins off), and a
// misplaced or quoted nocorrect is an ordinary command name.
func builtinOnlyCommandForms(name string) []string {
	return []string{"command " + name, "command -p " + name, "command -- " + name,
		"builtin command " + name, `"nocorrect" ` + name, "noglob nocorrect " + name}
}

func parseCall(t *testing.T, source string) *syntax.CallExpr {
	t.Helper()
	file, err := parse.Parse(strings.NewReader(source), "test.zsh")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	call, ok := file.AST().Stmts[0].Cmd.(*syntax.CallExpr)
	if !ok {
		t.Fatal("expected a call")
	}
	return call
}

// TestResolveCommand pins each row to what zsh 5.9.2 runs under zsh -f.
func TestResolveCommand(t *testing.T) {
	tests := []struct {
		source   string
		name     string
		args     int
		external bool
		exec     bool
	}{
		// Dynamic or missing names stay unresolved but keep their arguments.
		{`eval$x arg`, "", 1, false, false},
		{`"$cmd" arg`, "", 1, false, false},
		{`builtin $cmd arg`, "", 1, false, false},
		{`exec $cmd arg`, "", 1, false, true},
		{`command $cmd arg`, "", 1, true, false},
		{`builtin`, "", 0, false, false},
		{`value=x`, "", 0, false, false},
		{`exec`, "", 0, false, true},
		{`exec {fd}>&-`, "", 0, false, true},
		{`exec -a argv0`, "", 0, false, true},
		{`exec -a`, "", 0, false, true},
		// Quote removal: backslashes and quotes inside the name.
		{`e"val" arg`, "eval", 1, false, false},
		{`e\val arg`, "eval", 1, false, false},
		{`e\v\al arg`, "eval", 1, false, false},
		{`'e'val arg`, "eval", 1, false, false},
		{`$'e\x76al' arg`, "eval", 1, false, false},
		{`\\eval arg`, `\eval`, 1, false, false},
		{`"\eval" arg`, `\eval`, 1, false, false},
		{`'\eval' arg`, `\eval`, 1, false, false},
		{`"e\val" arg`, `e\val`, 1, false, false},
		// Precommand modifiers, in any order and quoted.
		{`noglob eval arg`, "eval", 1, false, false},
		{`- eval arg`, "eval", 1, false, false},
		{`"-" eval arg`, "eval", 1, false, false},
		{`- - eval arg`, "eval", 1, false, false},
		{`"noglob" eval arg`, "eval", 1, false, false},
		{`"builtin" eval arg`, "eval", 1, false, false},
		{`noglob builtin eval arg`, "eval", 1, false, false},
		{`builtin noglob eval arg`, "eval", 1, false, false},
		{`noglob - exec eval arg`, "eval", 1, false, true},
		// Modifiers take no option terminator: the next word is the name.
		{`noglob -- eval`, "--", 1, false, false},
		{`builtin -- eval`, "--", 1, false, false},
		// exec and its options.
		{`exec eval arg`, "eval", 1, false, true},
		{`"exec" eval arg`, "eval", 1, false, true},
		{`\exec eval arg`, "eval", 1, false, true},
		{`exec -a foo eval arg`, "eval", 1, false, true},
		{`exec -afoo eval arg`, "eval", 1, false, true},
		{`exec -c eval arg`, "eval", 1, false, true},
		{`exec -cl eval arg`, "eval", 1, false, true},
		{`exec -la foo eval arg`, "eval", 1, false, true},
		{`exec -a foo -c eval arg`, "eval", 1, false, true},
		{`exec -- eval arg`, "eval", 1, false, true},
		{`exec - eval arg`, "eval", 1, false, true},
		{`exec -l - eval arg`, "eval", 1, false, true},
		{`exec -ac foo eval`, "foo", 1, false, true},
		{`exec -x eval arg`, "", 2, false, true},
		// nocorrect is a reserved word: only bare, and only first.
		{`nocorrect eval arg`, "eval", 1, false, false},
		{`nocorrect nocorrect eval arg`, "eval", 1, false, false},
		{`nocorrect noglob eval arg`, "eval", 1, false, false},
		{`x=1 nocorrect eval arg`, "eval", 1, false, false},
		{`"nocorrect" eval arg`, "nocorrect", 2, false, false},
		{`\nocorrect eval arg`, "nocorrect", 2, false, false},
		{`noglob nocorrect eval arg`, "nocorrect", 2, false, false},
		{`nocorrect"" eval arg`, "nocorrect", 2, false, false},
		{`nocorrect'' eval arg`, "nocorrect", 2, false, false},
		// command ends the modifiers and names an external command.
		{`command eval arg`, "eval", 1, true, false},
		{`command -p eval arg`, "eval", 1, true, false},
		{`command -- grep arg`, "grep", 1, true, false},
		{`command -p -- grep arg`, "grep", 1, true, false},
		{`command noglob eval arg`, "noglob", 2, true, false},
		{`command builtin eval arg`, "builtin", 2, true, false},
		{`builtin command -p grep arg`, "grep", 1, true, false},
		{`exec command grep arg`, "grep", 1, true, true},
		{`command - echo arg`, "-", 2, true, false},
		{`command -p`, "", 0, true, false},
		{`command -v eval`, "", 1, true, false},
		{`command -V eval`, "", 1, true, false},
		{`command -pv eval`, "", 1, true, false},
		{`command -p -v eval`, "", 1, true, false},
		{`noglob command -v eval`, "", 1, true, false},
		{`"command" -v eval`, "", 1, true, false},
	}
	for _, test := range tests {
		t.Run(test.source, func(t *testing.T) {
			call := parseCall(t, test.source)
			got := resolveCommand(call)
			if got.name != test.name || len(got.args) != test.args || got.external != test.external || got.exec != test.exec {
				t.Fatalf("resolveCommand = {%q, %d args, external %v, exec %v}; want {%q, %d, %v, %v}",
					got.name, len(got.args), got.external, got.exec, test.name, test.args, test.external, test.exec)
			}
			if len(got.args) > 0 && got.args[0] != call.Args[len(call.Args)-len(got.args)] {
				t.Fatal("arguments must retain their original word nodes")
			}
		})
	}
	if got := resolveCommand(nil); got.name != "" || got.args != nil {
		t.Fatalf("nil call = %+v", got)
	}
	// A lone nocorrect is a native parse error, so build the call directly: the
	// reserved word leaves no command name behind.
	lone := &syntax.CallExpr{Args: []*syntax.Word{{Parts: []syntax.WordPart{&syntax.Lit{Value: "nocorrect"}}}}}
	if got := resolveCommand(lone); got.name != "" || len(got.args) != 0 {
		t.Fatalf("lone nocorrect = %+v", got)
	}
}

func TestEffectiveCommand(t *testing.T) {
	for _, form := range commandForms("eval") {
		t.Run(form, func(t *testing.T) {
			name, args := effectiveCommand(parseCall(t, form+" first second"))
			if name != "eval" || len(args) != 2 {
				t.Fatalf("command = %q, args = %d; want eval, 2", name, len(args))
			}
		})
	}
	for _, form := range append(builtinOnlyCommandForms("eval"), "command -v eval") {
		t.Run(form, func(t *testing.T) {
			if name, _ := effectiveCommand(parseCall(t, form+" first")); name == "eval" {
				t.Fatalf("command = eval; Zsh does not run the eval builtin for %s", form)
			}
		})
	}
	if name, args := effectiveCommand(nil); name != "" || args != nil {
		t.Fatalf("nil call = %q, %v", name, args)
	}
}
