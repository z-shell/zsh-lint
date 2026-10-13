package expand

import (
	"strings"
	"testing"

	"mvdan.cc/sh/v3/syntax"
)

// The Bash evaluator does not evaluate native Zsh arithmetic nodes.
func TestZshArithmeticUnsupported(t *testing.T) {
	for _, source := range []string{"[#16] 255", "#key", "##a", "++#key", "#key++", "#key = 2", "key [#16] = 2", "1 $op 2"} {
		expr, err := syntax.NewParser(syntax.Variant(syntax.LangZsh)).Arithmetic(strings.NewReader(source))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := Arithm(&Config{}, expr); err == nil || err.Error() != "unsupported" {
			t.Fatalf("Zsh arithmetic evaluation error = %v", err)
		}
	}
}
