package rules

import (
	"fmt"
	"os/exec"
	"strings"
	"testing"

	"github.com/z-shell/zsh-lint/internal/analyzer"
	"github.com/z-shell/zsh-lint/internal/diag"
	"github.com/z-shell/zsh-lint/internal/parse"
	"github.com/z-shell/zsh-lint/internal/projectconfig"
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
	diags := analyzeUnquotedVar(f, "test.zsh")

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

			diags := analyzeUnquotedVar(file, test.name+".zsh")
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

	diags := analyzeUnquotedVar(f, "test.zsh")

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

	diags := analyzeUnquotedVar(f, "test.zsh")

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
		{"exec -a name word", `exec -a $x eval "$y"`, 1},
		{"exec -a name with options", `noglob exec -a $x -l print ok`, 1},
		{"exec -a name before colon", `exec -a $x : $y`, 1},
		{"exec -a quoted name", `exec -a "$x" print ok`, 0},
		{"exec -a attached name", `exec -afoo print $y`, 1},
		{"dynamic command after modifier", `noglob $cmd arg`, 0},
		{"dynamic external command", `command -- $cmd arg`, 0},
		{"bare variable", `print $var`, 1},
		{"literal default alone", `print ${x:-y}`, 0},
		{"expansion default alone", `print ${x:-$y}`, 1},
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
			got := analyzeUnquotedVar(file, "test.zsh")
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
			got := analyzeUnquotedVar(file, "test.zsh")
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

func TestUnquotedVarSubscriptAdvice(t *testing.T) {
	const scalar = "Variable expansion should be double-quoted"
	const selection = "Quote this expansion without replacing its subscripts; use (@) to preserve selected array elements"
	tests := []struct {
		name, src, want string
	}{
		{"indexed element", `typeset -a arr=(x y); print $arr[1]`, scalar},
		{"braced element", `typeset -a arr=(x y); print ${arr[1]}`, scalar},
		{"negative element", `typeset -a arr=(x y); print $arr[-1]`, scalar},
		{"dynamic element", `typeset -a arr=(x y); print $arr[$i]`, scalar},
		{"associative element", `typeset -A m; m=(a b); print $m[a]`, scalar},
		{"array range", `typeset -a arr=(x y z); print ${arr[1,2]}`, selection},
		{"reverse selection", `typeset -a arr=(x y); print ${arr[(r)y]}`, selection},
		{"chained subscript", `typeset -a arr=(xx yy); print ${arr[1][2]}`, selection},
		{"all then selection", `typeset -a arr=(x y z); print ${arr[@][1,2]}`, selection},
		{"all elements", `typeset -a arr=(x y); print ${arr[@]}`, `Array expansion should preserve elements with "${arr[@]}"`},
		{"star elements", `typeset -a arr=(x y); print ${arr[*]}`, `Array expansion should preserve elements with "${arr[@]}"`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			file, err := parse.Parse(strings.NewReader(test.src), "test.zsh")
			if err != nil {
				t.Fatal(err)
			}
			got := analyzeUnquotedVar(file, "test.zsh")
			if len(got) != 1 || got[0].Message != test.want || got[0].Severity != diag.Info {
				t.Fatalf("diagnostics = %v, want one Info with %q", got, test.want)
			}
		})
	}
}

// analyzeUnquotedVar runs the rule for a configured standalone script; the rule
// reports nothing without project configuration (#705).
func analyzeUnquotedVar(file *parse.File, path string) diag.Diagnostics {
	return analyzer.New(UnquotedVar{}).AnalyzeSource(file, path,
		configuredSource(projectconfig.KindPlugin, projectconfig.ProfileStandaloneExecutable, ""))
}

func TestUnquotedVarRequiresConfiguration(t *testing.T) {
	const src = "print -r -- $value\n"
	file, err := parse.Parse(strings.NewReader(src), "test.zsh")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if got := analyzer.New(UnquotedVar{}).Analyze(file, "test.zsh"); len(got) != 0 {
		t.Errorf("unconfigured diagnostics = %v, want none", got)
	}
	got := analyzeUnquotedVar(file, "test.zsh")
	if len(got) != 1 || got[0].Severity != diag.Info || got[0].Range.Start.Column != 13 {
		t.Fatalf("configured diagnostics = %v, want one Info at column 13", got)
	}
}

func TestUnquotedVarSkipsNeverEmptyExpansions(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want int
	}{
		{"alternate on set or non-empty", "print -r -- ${name:+--named}\n", 0},
		{"alternate on set", "print -r -- ${name+--named}\n", 0},
		{"error on unset or empty", "print -r -- ${name:?required}\n", 0},
		{"literal default", "sleep ${interval:-0.1}\n", 0},
		{"quoted literal default", "sleep ${interval:-\"0.1\"}\n", 0},
		{"literal assigned default", "print -r -- ${mode:=fast}\n", 0},
		{"literal suffix in dynamic default", "print -r -- ${mode:-$other/path}\n", 0},
		{"literal prefix in dynamic default", "print -r -- ${mode:-prefix$other}\n", 0},
		{"literal between expansions", "print -r -- ${mode:-$first/$second}\n", 0},
		{"nested pattern remains unsupported", "print -r -- ${mode:-${other//a/}/file}\n", 1},
		{"literal glob inside double quotes", "print -r -- ${mode:-\"$other/missing*(N)\"}\n", 0},
		{"quoted dynamic default", "print -r -- ${mode:-\"prefix$other\"}\n", 0},
		{"single quoted text remains unsupported", "print -r -- ${mode:-'prefix'$other}\n", 1},
		{"unquoted assigned suffix default", "print -r -- ${mode:=$other/path}\n", 0},
		{"unquoted assigned prefix default", "print -r -- ${mode:=prefix$other}\n", 0},
		{"quoted assigned default", "print -r -- ${mode:=\"$other/path\"}\n", 0},
		{"empty brace expansion", "print -r -- ${mode:-$other{,}}\n", 1},
		{"unquoted outer array empty elements", "mode=('' ''); print -r -- ${mode:-prefix$other}\n", 1},
		{"unquoted outer array assigned default", "mode=('' ''); print -r -- ${mode:=prefix$other}\n", 1},
		{"outer array empty elements", "mode=('' ''); print -r -- ${mode:-\"prefix$other\"}\n", 1},
		{"outer array assigned default", "mode=('' ''); print -r -- ${mode:=\"prefix$other\"}\n", 1},
		{"braced reference remains unsupported", "print -r -- ${mode:-\"prefix${other}\"}\n", 1},
		{"short subscript remains unsupported", "print -r -- ${mode:-\"prefix$other[1]\"}\n", 1},
		{"command substitution remains unsupported", "print -r -- ${mode:-\"prefix$(true)\"}\n", 1},
		{"escaped quoted text remains unsupported", "print -r -- ${mode:-\"\\\n$other\"}\n", 1},
		{"quoted escaped dollar remains unsupported", "print -r -- ${mode:-\"\\$first$other\"}\n", 1},
		{"short splitting reference remains unsupported", "print -r -- ${mode:-prefix$=other}\n", 1},
		{"positional reference remains unsupported", "print -r -- ${mode:-prefix$1}\n", 1},
		{"plain nested arrays remain unsupported", "other=(''); print -r -- ${mode:-prefix$other}\n", 1},
		{"rc option with unknown reference", "setopt rc_expand_param; print -r -- ${mode:-prefix$other}\n", 1},
		{"glob option with unknown reference", "setopt glob_subst; print -r -- ${mode:-prefix$other}\n", 1},
		{"quoted option after expansion", "print -r -- ${mode:-prefix$other}; builtin setopt 'RCEXPANDPARAM'\n", 1},
		{"glob option in other function", "f() { setopt GLOB_SUBST; }; print -r -- ${mode:-prefix$other}\n", 1},
		{"dynamic setopt argument", "setopt $options; print -r -- ${mode:-prefix$other}\n", 2},
		{"short setopt bundle", "setopt -P; print -r -- ${mode:-prefix$other}\n", 1},
		{"setopt pattern", "setopt -m 'rc*'; print -r -- ${mode:-prefix$other}\n", 1},
		{"setopt after unrelated command in function", "f() { true; setopt rc_expand_param; }; print -r -- ${mode:-prefix$other}\n", 1},
		{"setopt inside command argument", "print \"$(setopt rc_expand_param)\"; print -r -- ${mode:-prefix$other}\n", 1},
		{"unrelated option still vetoes", "setopt no_beep; print -r -- ${mode:-prefix$other}\n", 1},
		{"tilde syntax unsupported", "print -r -- ${mode:-~$other}\n", 1},
		{"equals syntax unsupported", "print -r -- ${mode:-=$other}\n", 1},
		{"hash syntax unsupported", "print -r -- ${mode:-#$other}\n", 1},
		{"brace syntax unsupported", "print -r -- ${mode:-$other{a,b}}\n", 1},
		{"mixed glob with null qualifier", "print -r -- ${mode:-$other/missing*(N)}\n", 1},
		{"assigned mixed glob", "print -r -- ${mode:=$other/missing*(N)}\n", 1},
		{"mixed bracket glob", "print -r -- ${mode:-$other/missing[ab](N)}\n", 1},
		{"mixed question glob", "print -r -- ${mode:-$other/missing?(N)}\n", 1},
		{"mixed extended glob", "setopt ksh_glob; print -r -- ${mode:-prefix$other@(a|b)}\n", 1},
		{"outer unquote dynamic default", "print -r -- ${(Q)mode:-'\"\"'$other}\n", 1},
		{"outer unquote assigned default", "print -r -- ${(Q)mode:='\"\"'$other}\n", 1},
		{"outer reevaluate dynamic default", "print -r -- ${(e)mode:-'$value'$other}\n", 1},
		{"outer truncation dynamic default", "print -r -- ${(l:0:)mode:-prefix$other}\n", 1},

		{"array product can remove literal", "print -r -- ${mode:-prefix${^other}}\n", 1},
		{"declared empty array with rc option", "setopt rc_expand_param; other=(); print -r -- ${mode:-prefix$other}\n", 1},
		{"declared array without rc option", "other=(); print -r -- ${mode:-prefix$other}\n", 1},
		{"explicit rc toggle remains unsupported", "setopt rc_expand_param; other=(); print -r -- ${mode:-prefix${^^other}}\n", 1},
		{"glob prefix can remove literal", "setopt null_glob; print -r -- ${mode:-prefix$~other}\n", 1},
		{"glob suffix can remove literal", "setopt null_glob; print -r -- ${mode:-${~other}suffix}\n", 1},
		{"product before other expansion", "print -r -- ${mode:-${^first}/$second}\n", 1},
		{"product after other expansion", "print -r -- ${mode:-$first/${^second}}\n", 1},
		{"quoted product remains unsupported", "print -r -- ${mode:-\"prefix${^other}\"}\n", 1},
		{"escaped newline is not literal text", "print -r -- ${mode:-\\\n$other}\n", 1},
		{"empty default", "print -r -- ${mode:-}\n", 1},
		{"empty quoted default", "print -r -- ${mode:-\"\"}\n", 1},
		{"expansion default", "print -r -- ${mode:-$other}\n", 1},
		{"quoted expansion default", "print -r -- ${mode:-\"$other\"}\n", 1},
		{"default without colon", "print -r -- ${mode-fast}\n", 1},
		{"error without colon", "print -r -- ${mode?required}\n", 1},
		{"read-only numeric special", "print -r -- $PPID $ZSH_SUBSHELL $TTYIDLE\n", 0},
		{"assignable special", "print -r -- $HOME $PWD $OSTYPE\n", 3},
		{"integer local", "f() { integer count=3; print -r -- $count; }\n", 0},
		{"float declaration", "float ratio=0.5; print -r -- $ratio\n", 0},
		{"typeset integer flag", "f() { local -i n; print -r -- $n; }\n", 0},
		{"typeset float flag", "typeset -F t; print -r -- $t\n", 0},
		{"integer flag removed", "typeset +i n; print -r -- $n\n", 1},
		{"integer then unset", "f() { integer n; unset n; print -r -- $n; }\n", 1},
		{"integer in another function", "f() { integer n; }; g() { print -r -- $n; }\n", 1},
		{"use before integer", "print -r -- $n; integer n\n", 1},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			file, err := parse.Parse(strings.NewReader(test.src), "test.zsh")
			if err != nil {
				t.Fatalf("parse failed: %v", err)
			}
			if got := analyzeUnquotedVar(file, "test.zsh"); len(got) != test.want {
				t.Errorf("diagnostics = %d, want %d: %v", len(got), test.want, got)
			}
		})
	}
}
