package rules

import (
	"os/exec"
	"strings"
	"testing"

	"github.com/z-shell/zsh-lint/internal/analyzer"
	"github.com/z-shell/zsh-lint/internal/parse"
)

func TestUnquotedVar(t *testing.T) {
	src := `
echo $foo
echo "$bar"
A=$BAZ
echo ${#foo}
echo ${+bar}
echo $?
echo $$
echo $#
echo ${(f)lines}
echo ${(@)array}
`
	f, err := parse.Parse(strings.NewReader(src), "test.zsh")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	rule := UnquotedVar{}
	analyzerInst := analyzer.New(rule)
	diags := analyzerInst.Analyze(f, "test.zsh")

	// Only the unquoted command argument `$foo` (line 2) is flagged. The quoted
	// `"$bar"`, assignment RHS `A=$BAZ`, numeric expansions (`${#foo}`, `${+bar}`,
	// `$?`, `$$`, `$#`), and flag-guided expansions (`${(f)lines}`, `${(@)array}`)
	// are not reported.
	if len(diags) != 1 {
		t.Fatalf("expected 1 diagnostic, got %d: %v", len(diags), diags)
	}
	if diags[0].RuleID != rule.ID() {
		t.Errorf("expected rule ID %q, got %q", rule.ID(), diags[0].RuleID)
	}
	if diags[0].Range.Start.Line != 2 {
		t.Errorf("expected diagnostic on line 2 (echo $foo), got line %d", diags[0].Range.Start.Line)
	}
}

func TestUnquotedVarExplicitSplitting(t *testing.T) {
	tests := []struct {
		name      string
		src       string
		wantDiags int
	}{
		{
			name:      "nested split toggle",
			src:       "builtin emulate -L zsh ${=${options[xtrace]:#off}:+-o xtrace}\n",
			wantDiags: 0,
		},
		{
			name:      "simple split toggle",
			src:       "print -r -- ${=value}\n",
			wantDiags: 0,
		},
		{
			name:      "split toggle disabled",
			src:       "print -r -- ${==value}\n",
			wantDiags: 1,
		},
		{
			name:      "ordinary assignment expansion",
			src:       "print -r -- ${value=default}\n",
			wantDiags: 1,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			file, err := parse.Parse(strings.NewReader(test.src), test.name+".zsh")
			if err != nil {
				t.Fatalf("parse failed: %v", err)
			}

			diags := analyzer.New(UnquotedVar{}).Analyze(file, test.name+".zsh")
			if len(diags) != test.wantDiags {
				t.Fatalf("diagnostics = %d, want %d: %v", len(diags), test.wantDiags, diags)
			}
		})
	}
}

func TestExplicitSplitRequiresUnquotedEmptyElision(t *testing.T) {
	zsh, err := exec.LookPath("zsh")
	if err != nil {
		t.Skip("zsh is required for the native expansion control")
	}

	const script = `
unsetopt xtrace
set -- ${=${options[xtrace]:#off}:+-o xtrace}
print -r -- "unquoted:$#"
set -- "${=${options[xtrace]:#off}:+-o xtrace}"
print -r -- "quoted:$#"
`
	output, err := exec.Command(zsh, "-f", "-c", script).CombinedOutput()
	if err != nil {
		t.Fatalf("native Zsh control failed: %v\n%s", err, output)
	}
	if got, want := string(output), "unquoted:0\nquoted:1\n"; got != want {
		t.Fatalf("native Zsh argument counts = %q, want %q", got, want)
	}
}
