package rules

import (
	"fmt"
	"strings"
	"testing"

	"github.com/z-shell/zsh-lint/internal/analyzer"
	"github.com/z-shell/zsh-lint/internal/diag"
	"github.com/z-shell/zsh-lint/internal/parse"
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
