package scope_test

import (
	"strings"
	"testing"

	"github.com/z-shell/zsh-lint/internal/parse"
	"github.com/z-shell/zsh-lint/internal/scope"
	"mvdan.cc/sh/v3/syntax"
)

func TestIsDeclaredArray(t *testing.T) {
	tests := []struct {
		src  string
		want bool
	}{
		{`local -a opts; print $opts`, true},
		{`typeset -a opts; print $opts`, true},
		{`declare -a opts; print $opts`, true},
		{`opts=(a b); print $opts`, true},
		{`f() { opts=(a); print $opts; }`, true},
		{`f() { local -a opts; }; print $opts`, false},
		{`print $opts; opts=(a)`, false},
		{`opts[1]=a; print $opts`, false},
		{`typeset +a opts; print $opts`, false},
		{`typeset -A opts; print $opts`, false},
		{`f() { local -a opts; g() { :; }; print $opts; }`, true},
		{`f() { local -a opts; g() { print $opts; }; }`, false},
	}
	for _, test := range tests {
		t.Run(test.src, func(t *testing.T) {
			file, err := parse.Parse(strings.NewReader(test.src), "test.zsh")
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			index := scope.NewMap()
			index.Index(file.AST())
			found := false
			syntax.Walk(file.AST(), func(node syntax.Node) bool {
				if param, ok := node.(*syntax.ParamExp); ok {
					found = true
					if got := index.IsDeclaredArray(param.Param.Value, param.Pos()); got != test.want {
						t.Errorf("IsDeclaredArray = %v, want %v", got, test.want)
					}
				}
				return true
			})
			if !found {
				t.Fatal("missing test expansion")
			}
		})
	}
}
