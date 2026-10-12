package expand

import (
	"strings"
	"testing"

	"mvdan.cc/sh/v3/syntax"
)

// The Bash evaluator must not silently use substring replacement for Zsh's
// whole-string operator, including when a length prefix bypasses replacement.
func TestZshFullStringReplacementUnsupported(t *testing.T) {
	for _, source := range []string{`${x:/a/b}`, `${#x:/a/b}`, `${x://a/b}`} {
		f, err := syntax.NewParser(syntax.Variant(syntax.LangZsh)).Parse(strings.NewReader("print "+source+"\n"), "")
		if err != nil {
			t.Fatal(err)
		}
		w := f.Stmts[0].Cmd.(*syntax.CallExpr).Args[1]
		if _, err := Literal(&Config{}, w); err == nil || err.Error() != "unsupported" {
			t.Fatalf("Zsh replacement evaluation error = %v", err)
		}
	}
}
