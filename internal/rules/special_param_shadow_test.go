package rules

import (
	"fmt"
	"strings"
	"testing"

	"github.com/z-shell/zsh-lint/internal/analyzer"
	"github.com/z-shell/zsh-lint/internal/diag"
	"github.com/z-shell/zsh-lint/internal/parse"
	"mvdan.cc/sh/v3/syntax"
)

func analyzeSpecialParamShadow(t *testing.T, src string) diag.Diagnostics {
	t.Helper()

	file, err := parse.Parse(strings.NewReader(src), "test.zsh")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	diags := analyzer.New(SpecialParamShadow{}).Analyze(file, "test.zsh")
	for i, diagnostic := range diags {
		if diagnostic.RuleID != "compat/special-param-shadow" {
			t.Fatalf("diagnostic %d rule ID = %q", i, diagnostic.RuleID)
		}
	}
	return diags
}

func requireSpecialParamNames(t *testing.T, got diag.Diagnostics, wantNames []string) {
	t.Helper()

	if len(got) != len(wantNames) {
		t.Fatalf("diagnostics = %v, want names %v", got, wantNames)
	}
	for i, name := range wantNames {
		if !strings.Contains(got[i].Message, name) {
			t.Errorf("diagnostic %d = %q, want it to mention %q", i, got[i].Message, name)
		}
	}
}

func TestSpecialParamShadowDeclarations(t *testing.T) {
	tests := []struct {
		name      string
		src       string
		wantNames []string
	}{
		{name: "valueless local pipestatus", src: "local pipestatus\n", wantNames: []string{"pipestatus"}},
		{name: "array local of curated param", src: "local -a pipestatus\n", wantNames: []string{"pipestatus"}},
		{name: "multiple curated names in one decl", src: "typeset ZSH_VERSION=1 OSTYPE=x\n", wantNames: []string{"ZSH_VERSION", "OSTYPE"}},
		{name: "flags only curated among mixed names", src: "typeset foo=1 ZSH_VERSION=2\n", wantNames: []string{"ZSH_VERSION"}},
		{name: "-h creates local status and removes shell-special behavior", src: "local -h status=7\n", wantNames: []string{"status"}},
		{name: "-h creates local pipestatus and removes shell-special behavior", src: "local -h pipestatus=(7 8)\n", wantNames: []string{"pipestatus"}},
		{name: "global -g excluded", src: "typeset -g ZSH_VERSION=1\n"},
		{name: "plus g remains local", src: "typeset +g ZSH_VERSION=1\n", wantNames: []string{"ZSH_VERSION"}},
		{name: "grouped -ga excluded", src: "typeset -ga pipestatus\n"},
		{name: "readonly -g excluded", src: "readonly -g ZSH_VERSION=1\n"},
		{name: "export excluded", src: "export ZSH_VERSION=1\n"},
		{name: "ordinary local silent", src: "local target_version=$1\n"},
		{name: "read not flagged", src: "print $ZSH_VERSION\n"},
		{name: "empty file", src: ""},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := analyzeSpecialParamShadow(t, test.src)
			requireSpecialParamNames(t, got, test.wantNames)
		})
	}
}

func TestSpecialParamShadowCuratedNamesAcrossVariants(t *testing.T) {
	curatedNames := []string{
		"ZSH_VERSION",
		"ZSH_PATCHLEVEL",
		"ZSH_NAME",
		"ZSH_ARGZERO",
		"OSTYPE",
		"MACHTYPE",
		"VENDOR",
		"pipestatus",
		"status",
	}
	variants := []string{"local", "typeset", "declare", "readonly"}

	for _, variant := range variants {
		for _, name := range curatedNames {
			t.Run(variant+"/"+name, func(t *testing.T) {
				src := fmt.Sprintf("%s %s=value\n", variant, name)
				requireSpecialParamNames(t, analyzeSpecialParamShadow(t, src), []string{name})
			})
		}
	}
}

func TestSpecialParamShadowNonDeclarationModes(t *testing.T) {
	tests := []struct {
		name string
		src  string
	}{
		{name: "typeset minus p", src: "typeset -p ZSH_VERSION\n"},
		{name: "typeset plus p", src: "typeset +p ZSH_VERSION\n"},
		{name: "typeset minus m", src: "typeset -m ZSH_VERSION\n"},
		{name: "typeset plus m", src: "typeset +m ZSH_VERSION\n"},
		{name: "typeset minus f", src: "typeset -f ZSH_VERSION\n"},
		{name: "typeset plus f", src: "typeset +f ZSH_VERSION\n"},
		{name: "readonly minus p", src: "readonly -p ZSH_VERSION\n"},
		{name: "grouped minus p", src: "typeset -ap ZSH_VERSION\n"},
		{name: "grouped plus m and p", src: "typeset +mp ZSH_VERSION\n"},
		{name: "grouped readonly p", src: "readonly -rp ZSH_VERSION\n"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			requireSpecialParamNames(t, analyzeSpecialParamShadow(t, test.src), nil)
		})
	}
}

func TestSpecialParamShadowStaticQuotedModes(t *testing.T) {
	tests := []struct {
		name      string
		src       string
		wantNames []string
	}{
		{name: "single quoted global", src: "typeset '-g' ZSH_VERSION=1\n"},
		{name: "single quoted query", src: "typeset '+p' ZSH_VERSION\n"},
		{name: "double quoted global", src: "typeset \"-g\" ZSH_VERSION=1\n"},
		{name: "double quoted query", src: "typeset \"+p\" ZSH_VERSION\n"},
		{name: "concatenated global", src: "typeset \"-\"g ZSH_VERSION=1\n"},
		{name: "concatenated query", src: "typeset \"+\"p ZSH_VERSION\n"},
		{name: "double quoted grouped global", src: "typeset \"-ag\" ZSH_VERSION=1\n"},
		{name: "single quoted grouped query", src: "typeset '+mp' ZSH_VERSION\n"},
		{name: "ANSI-C escaped global", src: "typeset $'\\x2dg' ZSH_VERSION=1\n"},
		{name: "ANSI-C escaped query", src: "typeset $'\\x2bp' ZSH_VERSION\n"},
		{name: "unquoted escaped global", src: "typeset \\-g ZSH_VERSION=1\n"},
		{name: "unquoted escaped query", src: "typeset \\-p ZSH_VERSION\n"},
		{name: "double quotes preserve backslash before ordinary character", src: `typeset "-\g" ZSH_VERSION=1`},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			requireSpecialParamNames(t, analyzeSpecialParamShadow(t, test.src), test.wantNames)
		})
	}
}

func TestStaticDeclarationWordQuoteRemoval(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want string
	}{
		{name: "unquoted backslash escapes option sign", src: "typeset \\-g ZSH_VERSION=1\n", want: "-g"},
		{name: "ordinary single quotes preserve backslash", src: "typeset '\\-g' ZSH_VERSION=1\n", want: "\\-g"},
		{name: "double quotes preserve backslash before ordinary character", src: `typeset "-\g" ZSH_VERSION=1`, want: "-\\g"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			file, err := parse.Parse(strings.NewReader(test.src), "test.zsh")
			if err != nil {
				t.Fatalf("parse: %v", err)
			}

			var decl *syntax.DeclClause
			syntax.Walk(file.AST(), func(node syntax.Node) bool {
				if candidate, ok := node.(*syntax.DeclClause); ok {
					decl = candidate
					return false
				}
				return true
			})
			if decl == nil || len(decl.Args) == 0 {
				t.Fatalf("declaration args not found in %q", test.src)
			}

			got, ok := staticDeclarationWord(decl.Args[0].Value)
			if !ok || got != test.want {
				t.Fatalf("staticDeclarationWord() = %q, %v; want %q, true", got, ok, test.want)
			}
		})
	}
}

func TestSpecialParamShadowDynamicModeIsSilent(t *testing.T) {
	requireSpecialParamNames(t, analyzeSpecialParamShadow(t, "typeset \"$mode\" ZSH_VERSION=1\n"), nil)
}

func TestSpecialParamShadowOrderedGlobalAndExportModes(t *testing.T) {
	tests := []struct {
		name      string
		src       string
		wantNames []string
	}{
		{name: "final plus g is local", src: "typeset -g +g ZSH_VERSION=1\n", wantNames: []string{"ZSH_VERSION"}},
		{name: "ANSI-C escaped plus g is local", src: "typeset $'\\x2bg' ZSH_VERSION=1\n", wantNames: []string{"ZSH_VERSION"}},
		{name: "final minus g is global", src: "typeset +g -g ZSH_VERSION=1\n"},
		{name: "local export is local", src: "local -x ZSH_VERSION=1\n", wantNames: []string{"ZSH_VERSION"}},
		{name: "typeset export depends on ambient global export", src: "typeset -x ZSH_VERSION=1\n"},
		{name: "declare export depends on ambient global export", src: "declare -x ZSH_VERSION=1\n"},
		{name: "readonly export depends on ambient global export", src: "readonly -x ZSH_VERSION=1\n"},
		{name: "export then explicit local", src: "typeset -x +g ZSH_VERSION=1\n", wantNames: []string{"ZSH_VERSION"}},
		{name: "explicit local then export", src: "typeset +g -x ZSH_VERSION=1\n", wantNames: []string{"ZSH_VERSION"}},
		{name: "typeset export remains unknown after plus x", src: "typeset -x +x ZSH_VERSION=1\n"},
		{name: "declare export remains unknown after plus x", src: "declare -x +x ZSH_VERSION=1\n"},
		{name: "readonly export remains unknown after plus x", src: "readonly -x +x ZSH_VERSION=1\n"},
		{name: "final minus x depends on ambient global export", src: "typeset +x -x ZSH_VERSION=1\n"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			requireSpecialParamNames(t, analyzeSpecialParamShadow(t, test.src), test.wantNames)
		})
	}
}

func TestSpecialParamShadowPatternDeclarations(t *testing.T) {
	tests := []struct {
		name      string
		src       string
		wantNames []string
	}{
		{name: "plus g before minus m creates locals", src: "typeset +g -m ZSH_VERSION\n", wantNames: []string{"ZSH_VERSION"}},
		{name: "plus g after minus m creates locals", src: "typeset -m +g ZSH_VERSION\n", wantNames: []string{"ZSH_VERSION"}},
		{name: "final minus g keeps matches non-local", src: "typeset +g -m -g ZSH_VERSION\n"},
		{name: "final plus g restores local creation", src: "typeset -m -g +g ZSH_VERSION\n", wantNames: []string{"ZSH_VERSION"}},
		{name: "plus m remains display mode", src: "typeset +g +m ZSH_VERSION\n"},
		{name: "assignment to exact curated name creates local", src: "typeset +g -m ZSH_VERSION=shadow\n", wantNames: []string{"ZSH_VERSION"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			requireSpecialParamNames(t, analyzeSpecialParamShadow(t, test.src), test.wantNames)
		})
	}
}

func TestSpecialParamShadowNumericOptionArguments(t *testing.T) {
	tests := []struct {
		name      string
		src       string
		wantNames []string
	}{
		{name: "E width permits later global", src: "typeset -E 5 -g ZSH_VERSION\n"},
		{name: "F quoted width permits later global", src: "typeset -F '5' -g ZSH_VERSION\n"},
		{name: "L width permits later global", src: "typeset -L 5 -g ZSH_VERSION\n"},
		{name: "plus R width permits later global", src: "typeset +R 5 -g ZSH_VERSION\n"},
		{name: "Z width permits later global", src: "typeset -Z 5 -g ZSH_VERSION\n"},
		{name: "i base permits later global", src: "typeset -i 10 -g ZSH_VERSION\n"},
		{name: "later explicit local reports", src: "typeset -L 5 +g ZSH_VERSION\n", wantNames: []string{"ZSH_VERSION"}},
		{name: "nonnumeric candidate starts operands", src: "typeset -L width -g ZSH_VERSION\n", wantNames: []string{"ZSH_VERSION"}},
		{name: "dynamic candidate is unknown", src: "typeset -L \"$width\" -g ZSH_VERSION\n"},
		{name: "later pattern mode is processed", src: "typeset -L 5 -m ZSH_VERSION\n"},
		{name: "later display pattern mode is processed", src: "typeset -L 5 +m ZSH_VERSION\n"},
		{name: "later export mode is processed", src: "typeset -L 5 -x ZSH_VERSION\n"},
		{name: "later explicit local overrides export ambiguity", src: "typeset -L 5 -x +g ZSH_VERSION\n", wantNames: []string{"ZSH_VERSION"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			requireSpecialParamNames(t, analyzeSpecialParamShadow(t, test.src), test.wantNames)
		})
	}
}

func TestSpecialParamShadowStandalonePlusDisplay(t *testing.T) {
	tests := []struct {
		name string
		src  string
	}{
		{name: "standalone plus", src: "typeset + ZSH_VERSION\n"},
		{name: "standalone plus after option", src: "typeset -U + ZSH_VERSION\n"},
		{name: "quoted standalone plus", src: "typeset '+' ZSH_VERSION\n"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			requireSpecialParamNames(t, analyzeSpecialParamShadow(t, test.src), nil)
		})
	}
}

func TestSpecialParamShadowTiedParameterOperands(t *testing.T) {
	tests := []struct {
		name      string
		src       string
		wantNames []string
	}{
		{name: "static third operand is separator", src: "typeset -T tied_scalar tied_array ZSH_VERSION\n"},
		{name: "first operand is scalar declaration", src: "typeset -T ZSH_VERSION tied_array\n", wantNames: []string{"ZSH_VERSION"}},
		{name: "second operand is array declaration", src: "typeset -T tied_scalar ZSH_VERSION\n", wantNames: []string{"ZSH_VERSION"}},
		{name: "quoted third operand is separator", src: "typeset -T tied_scalar tied_array 'ZSH_VERSION'\n"},
		{name: "numeric option after T preserves separator", src: "typeset -T -L 5 tied_scalar tied_array ZSH_VERSION\n"},
		{name: "numeric option before T preserves separator", src: "typeset -L 5 -T tied_scalar tied_array ZSH_VERSION\n"},
		{name: "grouped T and numeric option preserve separator", src: "typeset -TL 5 tied_scalar tied_array ZSH_VERSION\n"},
		{name: "numeric option preserves curated scalar", src: "typeset -T -R 5 ZSH_VERSION tied_array separator\n", wantNames: []string{"ZSH_VERSION"}},
		{name: "numeric option preserves curated array", src: "typeset -Z 5 -T tied_scalar ZSH_VERSION separator\n", wantNames: []string{"ZSH_VERSION"}},
		{name: "ambiguous grouped numeric ordering is silent", src: "typeset -LT 5 ZSH_VERSION tied_array separator\n"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			requireSpecialParamNames(t, analyzeSpecialParamShadow(t, test.src), test.wantNames)
		})
	}
}

func TestSpecialParamShadowOptionProcessingStopsAtFirstOperand(t *testing.T) {
	tests := []struct {
		name      string
		src       string
		wantNames []string
	}{
		{name: "dynamic word before operand is unknown", src: "typeset \"$mode\" ZSH_VERSION=1\n"},
		{name: "dynamic word after assignment does not hide declaration", src: "typeset ZSH_VERSION=1 \"$value\"\n", wantNames: []string{"ZSH_VERSION"}},
		{name: "global-looking word after assignment is an operand", src: "typeset ZSH_VERSION=1 -g\n", wantNames: []string{"ZSH_VERSION"}},
		{name: "query-looking word after assignment is an operand", src: "typeset ZSH_VERSION=1 -p\n", wantNames: []string{"ZSH_VERSION"}},
		{name: "double dash terminates options", src: "typeset -- ZSH_VERSION=1 \"$value\"\n", wantNames: []string{"ZSH_VERSION"}},
		{name: "single dash terminates options", src: "typeset - ZSH_VERSION=1 \"$value\"\n", wantNames: []string{"ZSH_VERSION"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			requireSpecialParamNames(t, analyzeSpecialParamShadow(t, test.src), test.wantNames)
		})
	}
}

func TestSpecialParamShadowStaticDeclarationOperands(t *testing.T) {
	tests := []struct {
		name      string
		src       string
		wantNames []string
	}{
		{name: "double quoted assignment", src: "local \"ZSH_VERSION=shadow\"\n", wantNames: []string{"ZSH_VERSION"}},
		{name: "concatenated quoted assignment", src: "local ZSH\"_\"VERSION=shadow\n", wantNames: []string{"ZSH_VERSION"}},
		{name: "dynamic quoted name is unknown", src: "local \"ZSH_${part}=shadow\"\n"},
		{name: "dynamic concatenated name is unknown", src: "local ZSH_\"$part\"=shadow\n"},
		{name: "quoted name matching is case sensitive", src: "local \"zsh_version=shadow\"\n"},
		{name: "quoted trailing plus is not an append assignment", src: "local \"ZSH_VERSION+\"\n"},
		{name: "ordinary assignment is not duplicated", src: "local ZSH_VERSION=shadow\n", wantNames: []string{"ZSH_VERSION"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			requireSpecialParamNames(t, analyzeSpecialParamShadow(t, test.src), test.wantNames)
		})
	}
}

func TestSpecialParamShadowExclusionsAreExact(t *testing.T) {
	for _, name := range []string{"path", "fpath", "PATH", "REPLY", "match"} {
		t.Run("excluded/"+name, func(t *testing.T) {
			src := fmt.Sprintf("local %s=value\n", name)
			requireSpecialParamNames(t, analyzeSpecialParamShadow(t, src), nil)
		})
	}

	curatedNames := []string{
		"ZSH_VERSION",
		"ZSH_PATCHLEVEL",
		"ZSH_NAME",
		"ZSH_ARGZERO",
		"OSTYPE",
		"MACHTYPE",
		"VENDOR",
		"pipestatus",
		"status",
	}
	for _, name := range curatedNames {
		t.Run("case/"+name, func(t *testing.T) {
			otherCase := strings.ToLower(name)
			if otherCase == name {
				otherCase = strings.ToUpper(name)
			}
			src := fmt.Sprintf("local %s=value\n", otherCase)
			requireSpecialParamNames(t, analyzeSpecialParamShadow(t, src), nil)
		})
	}
}

func TestSpecialParamShadowDiagnostic(t *testing.T) {
	diags := analyzeSpecialParamShadow(t, "local ZSH_VERSION=\"$1\"\n")
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %v, want one", diags)
	}

	wantMessage := "Declaring shell-set parameter 'ZSH_VERSION' can override shell-managed state in this scope and nested code; use a different parameter name"
	if diags[0].Message != wantMessage {
		t.Errorf("message = %q, want %q", diags[0].Message, wantMessage)
	}
	wantRange := diag.Range{
		Start: diag.Position{Line: 1, Column: 7, Offset: 6},
		End:   diag.Position{Line: 1, Column: 18, Offset: 17},
	}
	if diags[0].Range != wantRange {
		t.Errorf("range = %+v, want %+v", diags[0].Range, wantRange)
	}
}

func TestSpecialParamShadowSuppression(t *testing.T) {
	src := "local ZSH_VERSION=\"$1\" # zsh-lint disable=compat/special-param-shadow -- intentional version shim\n"
	file, err := parse.Parse(strings.NewReader(src), "test.zsh")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	diags := analyzer.New(SpecialParamShadow{}).Analyze(file, "test.zsh")
	if len(diags) != 0 {
		t.Fatalf("suppressed diagnostics = %v, want none", diags)
	}
}
