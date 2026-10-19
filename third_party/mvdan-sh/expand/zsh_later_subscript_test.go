package expand

import (
	"strings"
	"testing"

	"github.com/z-shell/zsh-lint/third_party/mvdan-sh/syntax"
)

// The Bash evaluator has no later subscripts, so it must not silently
// evaluate only the first subscript of a Zsh expansion (zsh-lint #252).
func TestZshLaterSubscriptsUnsupported(t *testing.T) {
	for _, source := range []string{`${a[1][2]}`, `${#a[1][2]}`, `${a[1][2]:-x}`} {
		f, err := syntax.NewParser(syntax.Variant(syntax.LangZsh)).Parse(strings.NewReader("print "+source+"\n"), "")
		if err != nil {
			t.Fatal(err)
		}
		w := f.Stmts[0].Cmd.(*syntax.CallExpr).Args[1]
		if _, err := Literal(&Config{}, w); err == nil || err.Error() != "unsupported" {
			t.Fatalf("%s: later subscript evaluation error = %v", source, err)
		}
	}
}
