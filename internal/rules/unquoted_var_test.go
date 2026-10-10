package rules

import (
	"fmt"
	"os/exec"
	"strings"
	"testing"

	"github.com/z-shell/zsh-lint/internal/analyzer"
	"github.com/z-shell/zsh-lint/internal/diag"
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
		{
			name:      "glob substitution",
			src:       "[[ $str == ${~pattern} ]] && print -r -- ${~pattern}\n",
			wantDiags: 0,
		},
		{
			name:      "glob substitution disabled",
			src:       "print -r -- ${~~pattern}\n",
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

// The count of a repeat loop is an arithmetic expression, not a command
// argument, so `repeat $n` is not flagged; the loop body still is.
func TestUnquotedVarIgnoresRepeatCount(t *testing.T) {
	src := "repeat $n print hi\nrepeat ${count} { print $x }\nrepeat $n; do print $y; done\n"
	f, err := parse.Parse(strings.NewReader(src), "test.zsh")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	diags := analyzer.New(UnquotedVar{}).Analyze(f, "test.zsh")

	want := []string{"2:25", "3:21"}
	var got []string
	for _, d := range diags {
		got = append(got, fmt.Sprintf("%d:%d", d.Range.Start.Line, d.Range.Start.Column))
	}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Fatalf("expected diagnostics at %v, got %v", want, got)
	}
}

// The body of a select loop written in the short form is fed to rules on
// its original bytes, exactly as the `do` form of the same loop is.
func TestUnquotedVarSeesSelectShortFormBody(t *testing.T) {
	src := "select o in a b c; print $o\nselect o in a b c; do print $o; done\nselect o in \"$@\"\n  eval $o && break\n"
	f, err := parse.Parse(strings.NewReader(src), "test.zsh")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	diags := analyzer.New(UnquotedVar{}).Analyze(f, "test.zsh")

	want := []string{"1:26", "2:29", "4:8"}
	var got []string
	for _, d := range diags {
		got = append(got, fmt.Sprintf("%d:%d", d.Range.Start.Line, d.Range.Start.Column))
	}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Fatalf("expected diagnostics at %v, got %v", want, got)
	}
}

func TestUnquotedVarArgumentBoundaries(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want int
	}{
		{"literal prefix", `print a$y`, 0},
		{"literal suffix", `print $y/b`, 0},
		{"other word parts", `print $y"suffix" $y$(print b)`, 0},
		{"colon arguments", `: ${x:=default}`, 0},
		{"builtin colon arguments", `builtin : ${x:=default}`, 0},
		{"command colon arguments", `command -p : $var`, 0},
		{"quoted colon arguments", `":" $var`, 0},
		{"command position", `$cmd`, 0},
		{"prefixed command position", `builtin $cmd`, 0},
		{"command position with argument", `$cmd $var`, 1},
		{"prefixed command with argument", `command -p $cmd $var`, 1},
		{"bare variable", `print $var`, 1},
		{"default alone", `print ${x:-y}`, 1},
		{"quoted variable", `print "$var"`, 0},
		{"assignment", `name=$var`, 0},
		{"issue repro without parser gaps", `f() { local -a opts; print a$y $y/b; : ${x:=default}; print ${opts[@]} $opts; }`, 2},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			file, err := parse.Parse(strings.NewReader(test.src), "test.zsh")
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			got := analyzer.New(UnquotedVar{}).Analyze(file, "test.zsh")
			if len(got) != test.want {
				t.Fatalf("diagnostics = %v, want %d", got, test.want)
			}
			for _, finding := range got {
				if finding.Severity != diag.Info {
					t.Fatalf("severity = %v, want info", finding.Severity)
				}
			}
		})
	}
}

func TestUnquotedVarDeclaredArrays(t *testing.T) {
	tests := []struct {
		name  string
		src   string
		array bool
	}{
		{"local", `f() { local -a opts; print $opts; }`, true},
		{"typeset", `typeset -a opts; print $opts`, true},
		{"declare", `declare -a opts; print $opts`, true},
		{"combined options", `f() { local -ra opts; print $opts; }`, true},
		{"initialized declaration", `f() { local -a opts=(a b); print $opts; }`, true},
		{"array assignment", `opts=(a b); print $opts`, true},
		{"function array assignment", `f() { opts=(a b); print $opts; }`, true},
		{"unquoted all elements", `f() { local -a opts; print ${opts[@]}; }`, true},
		{"scalar", `f() { local opts; print $opts; }`, false},
		{"declared later", `f() { print $opts; local -a opts; }`, false},
		{"sibling function", `f() { local -a opts; }; g() { print $opts; }`, false},
		{"function declaration at file scope", `f() { local -a opts; }; print $opts`, false},
		{"nested function", `f() { local -a opts; g() { print $opts; }; }`, false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			file, err := parse.Parse(strings.NewReader(test.src), "test.zsh")
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			got := analyzer.New(UnquotedVar{}).Analyze(file, "test.zsh")
			if len(got) != 1 {
				t.Fatalf("diagnostics = %v, want one", got)
			}
			want := "Variable expansion should be double-quoted"
			if test.array {
				want = `Array expansion should preserve elements with "${opts[@]}"`
			}
			if got[0].Message != want || got[0].Severity != diag.Info {
				t.Fatalf("diagnostic = %v, want info with %q", got[0], want)
			}
		})
	}
}
