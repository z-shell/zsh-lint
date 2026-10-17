package analyzer_test

import (
	"strings"
	"testing"

	"github.com/z-shell/zsh-lint/internal/parse"
)

// Issue #649: declaration arguments inside alternate brace bodies must remain
// visible to rules, with suppressions applying at the original source line.
func TestAnalyzerDeclarationAlternateBraceBody(t *testing.T) {
	for _, header := range []string{"if [[ -n x ]]", "for i in 1;"} {
		t.Run(header, func(t *testing.T) {
			body := header + " { local u=$(eval $x) }"
			code := body + " # zsh-lint disable=security/eval -- static table\n" + body + "\n"
			file, err := parse.Parse(strings.NewReader(code), "issue-649.zsh")
			if err != nil {
				t.Fatal(err)
			}
			diags := analyzeConfigured(file, "issue-649.zsh")
			if got := findByID(diags, "security/eval"); len(got) != 1 || got[0].Range.Start.Line != 2 {
				t.Errorf("security/eval findings = %+v, want only line 2", got)
			}
			if got := findByID(diags, "quoting/unquoted-var"); len(got) != 2 {
				t.Errorf("unquoted findings = %+v, want both declarations' expansions", got)
			}
			if got := findByID(diags, "meta/unused-suppression"); len(got) != 0 {
				t.Errorf("used suppression reported stale: %+v", got)
			}
		})
	}
}
