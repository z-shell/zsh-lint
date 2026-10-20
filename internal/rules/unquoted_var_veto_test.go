package rules

import (
	"strings"
	"testing"

	"github.com/z-shell/zsh-lint/internal/parse"
)

func TestUnquotedVarMixedDefaultCoarseVetoes(t *testing.T) {
	const use = "print -r -- ${mode:-prefix$other}\n"
	for _, command := range []string{
		"setopt", "unsetopt", "setopt +o no_glob_subst", "setopt +o no_rc_expand_param",
		"set -o glob_subst", "set +o no_glob_subst", "set -o", "emulate -L zsh",
		"builtin setopt no_beep", "command setopt no_beep", "noglob unsetopt glob_subst",
		"options[glob_subst]=on", "options=(glob_subst on)",
		"set -x", "set +x", "command builtin setopt no_beep",
		"print ${options[glob_subst]::=on}",
		"builtin typeset 'options[globsubst]=on'",
		"builtin export 'options[globsubst]=on'",
		"review_tmp=$(setopt glob_subst) setopt no_beep",
		"review_tmp=$(unsetopt no_glob_subst) true",
	} {
		t.Run(command, func(t *testing.T) {
			assertMixedDefaultCount(t, command+"\n"+use, 1)
		})
	}
	for _, builtin := range []string{"typeset", "declare", "local", "integer", "float", "readonly", "export"} {
		for _, prefix := range []string{"", "builtin ", "command ", "command builtin "} {
			src := prefix + builtin + " '-A' other\n" + use
			t.Run(src, func(t *testing.T) {
				assertMixedDefaultCount(t, src, 1)
			})
		}
	}
	for _, declaration := range []string{"typeset -A mode", "typeset '-a' mode", "builtin typeset -a mode"} {
		for _, expansion := range []string{"${mode:-prefix$other}", "${mode:=prefix$other}", "${mode:-\"prefix$other\"}"} {
			src := declaration + "; mode[1]=''; mode[2]=''; other=; print -- " + expansion
			t.Run(src, func(t *testing.T) {
				assertMixedDefaultCount(t, src, 1)
			})
		}
	}
	for _, test := range []struct {
		src  string
		want int
	}{
		{use + "typeset mode\n", 1},
		{"f() { local other; }\n" + use, 1},
		{"other=()\n" + use, 1},
		{"mode[1]=''\n" + use, 1},
		{use + "other=()\n", 1},
		{"typeset unrelated\n" + use, 0},
		{"builtin typeset unrelated\n" + use, 0},
		{"command export unrelated=value\n" + use, 0},
		{"builtin typeset $flags other\n" + use, 2},
		{"builtin typeset \"$declared_name\"\n" + use, 1},
		{"builtin typeset 'other=value'\n" + use, 1},
		{"builtin typeset 'unrelated=other'\n" + use, 1},
		{"unrelated=()\n" + use, 0},
		{"set -- a b\n" + use, 0},
		{"setopt glob_subst; print ${mode:-literal}\n", 0},
		{"x= y=; print ${x:-$y:s/a/b/}\n", 1},
		{"x= y=; print ${x:=$y:s/a/b/}\n", 1},
		{"print ${mode:-\"prefix:$other\"}\n", 1},
		{"set -- '' ''; other=; print ${argv:-prefix$other}\n", 1},
		{"set -- '' ''; other=; print ${argv:=prefix$other}\n", 1},
		{"set -- '' ''; other=; print ${argv:-\"prefix$other\"}\n", 1},
		{"print ${mode:-prefix$argv}\n", 1},
		{"print ${mode:-prefix$path}\n", 1},
		{"print ${mode:-prefix$match}\n", 1},
		{"print ${mode:-prefix$words}\n", 1},
		{"print ${mode:-prefix$funcstack}\n", 1},
		{"print ${mode:-prefix$BUFFER}\n", 1},
		{"x= y='*(N)'; setopt +o no_glob_subst; print ${x:-prefix$y}\n", 1},
		{"make_array() { y=(); }; make_array; x=; setopt +o no_rc_expand_param; print ${x:-prefix$y}\n", 1},
	} {
		t.Run(test.src, func(t *testing.T) {
			assertMixedDefaultCount(t, test.src, test.want)
		})
	}
	for _, name := range []string{"a", "z", "A", "Z", "n0", "n9", "_state"} {
		src := "builtin typeset '" + name + "=value'\nprint ${mode:-prefix$" + name + "}"
		t.Run(name, func(t *testing.T) {
			assertMixedDefaultCount(t, src, 1)
		})
	}
	for _, name := range strings.Fields(mixedDefaultShellNames) {
		t.Run("shell-managed/"+name, func(t *testing.T) {
			assertMixedDefaultCount(t, "print ${mode:-prefix$"+name+"}", 1)
		})
	}
}

func assertMixedDefaultCount(t *testing.T, src string, want int) {
	t.Helper()
	file, err := parse.Parse(strings.NewReader(src), "test.zsh")
	if err != nil {
		t.Fatal(err)
	}
	if got := analyzeUnquotedVar(file, "test.zsh"); len(got) != want {
		t.Fatalf("diagnostics = %v, want %d", got, want)
	}
}
