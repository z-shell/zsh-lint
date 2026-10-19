package scope_test

import (
	"strings"
	"testing"

	"github.com/z-shell/zsh-lint/internal/parse"
	"github.com/z-shell/zsh-lint/internal/scope"
	"github.com/z-shell/zsh-lint/third_party/mvdan-sh/syntax"
)

// TestIsDeclaredNumeric pins the integer and float declaration index that lets
// quoting/unquoted-var skip names that can never expand empty (#705).
func TestIsDeclaredNumeric(t *testing.T) {
	tests := []struct {
		src  string
		want bool
	}{
		{`integer n; print $n`, true},
		{`float n=0.5; print $n`, true},
		{`integer a n; print $n`, true},
		{`local -i n; print $n`, true},
		{`typeset -E n; print $n`, true},
		{`declare -F n; print $n`, true},
		{`typeset -ia n; print $n`, true},
		{`f() { integer n; print $n; }`, true},
		{`typeset +i n; print $n`, false},
		{`typeset -i n; typeset +i m; print $n`, true},
		{`typeset -a n; print $n`, false},
		{`local n=3; print $n`, false},
		{`print $n; integer n`, false},
		{`integer n[1]; print $n`, false},
		{`integer n; unset n; print $n`, false},
		{`integer n; unset 'n[1]'; print $n`, false},
		{`integer n; unset 'n'; print $n`, false},
		{`integer n; unset "n"; print $n`, false},
		{`integer n; unset -v n; print $n`, false},
		{`integer n; unset "$n"; print $n`, true},
		{`f() { integer n; unset n; print $n; }`, false},
		{`f() { integer n; }; print $n`, false},
		{`f() { integer n; }; g() { print $n; }`, false},
		{`f() { unset n; }; integer n; print $n`, true},
		{`integer n; unset m; print $n`, true},
		{`integer n; print -r -- unset n; print $n`, true},
		// A command substitution runs in a subshell: its declarations and
		// unsets count only inside it (#252).
		{`x=$(integer n); print $n`, false},
		{"x=`integer n`; print $n", false},
		{`print ${a[$(integer n)]}; print $n`, false},
		{`print ${a[1][$(integer n)]}; print $n`, false},
		{`print $(integer n)$n`, false},
		{`x=$(integer n); y=$(print $n)`, false},
		{`x=$(y=$(integer n); print $n)`, false},
		{`x=$(integer n; print $n)`, true},
		{`x=$(integer n; y=$(print $n))`, true},
		{`integer n; x=$(print $n)`, true},
		{`integer n; x=$(unset n); print $n`, true},
		{`x=$(integer n; unset n; print $n)`, false},
		{`f() { x=$(integer n); print $n; }`, false},
		{`integer n; x=$(:); print $n`, true},
		{`x=$(:); integer n; print $n`, true},
	}
	for _, test := range tests {
		t.Run(test.src, func(t *testing.T) {
			file, err := parse.Parse(strings.NewReader(test.src), "test.zsh")
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			index := scope.NewMap()
			index.Index(file.AST())
			var last *syntax.ParamExp
			syntax.Walk(file.AST(), func(node syntax.Node) bool {
				if param, ok := node.(*syntax.ParamExp); ok {
					last = param
				}
				return true
			})
			if last == nil {
				t.Fatal("missing test expansion")
			}
			if got := index.IsDeclaredNumeric(last.Param.Value, last.Pos()); got != test.want {
				t.Errorf("IsDeclaredNumeric(%q) = %v, want %v", last.Param.Value, got, test.want)
			}
		})
	}
}
