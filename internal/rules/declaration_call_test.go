package rules

import (
	"strings"
	"testing"

	"github.com/z-shell/zsh-lint/internal/analyzer"
	"github.com/z-shell/zsh-lint/internal/diag"
	"github.com/z-shell/zsh-lint/internal/parse"
	"github.com/z-shell/zsh-lint/internal/projectconfig"
)

func TestDeclarationCallOperandBoundaries(t *testing.T) {
	tests := []struct {
		name string
		rule analyzer.Rule
		src  string
		want int
	}{
		{"quoted dynamic shadow", SpecialParamShadow{}, "example_f() { builtin local \"ZSH_VERSION=$1\"; }\n", 1},
		{"unquoted escaped shadow", SpecialParamShadow{}, "example_f() { builtin local ZSH_\\VERSION=1; }\n", 1},
		{"quoted backslash name is invalid", SpecialParamShadow{}, "example_f() { builtin local \"ZSH_\\VERSION=1\"; }\n", 0},
		{"quoted dynamic zero", ZeroHandling{}, "builtin typeset \"0=$value\"\n", 1},
		{"invalid parameter", PersistentParameterNamespace{}, "builtin typeset -g invalid-name=1\n", 0},
		{"ordinary append rejected", SpecialParamShadow{}, "example_f() { builtin local ZSH_VERSION+=1; }\n", 0},
		{"tied separator", PersistentParameterNamespace{}, "builtin typeset -T _example_scalar _example_array :\n", 0},
		{"tied name separator", PersistentParameterNamespace{}, "builtin typeset -T _example_scalar _example_array foreign_separator\n", 0},
		{"tied registry separator", SharedPluginsRegistry{}, "builtin typeset -T _example_scalar _example_array Plugins\n", 0},
		{"uncertain registry mode", SharedPluginsRegistry{}, "example_f() { builtin local $flags Plugins; Plugins=changed; }\n", 1},
		{"vanishing zero does not initialize", ZeroHandling{}, "builtin typeset 0=$empty\nprint -r -- $0\n", 2},
		{"possibly absent registry local", SharedPluginsRegistry{}, "example_f() { builtin local Plugins=$empty; Plugins=changed; }\n", 1},
		{"globbed registry local", SharedPluginsRegistry{}, "example_f() { builtin local Plugins=*(N); Plugins=changed; }\n", 1},
		{"possibly absent fpath local", FpathHygiene{}, "example_f() { builtin local FPATH=$empty; FPATH=/opt/functions; }\n", 1},
		{"quoted tilde remains literal", FpathHygiene{}, "builtin typeset \"FPATH=~/functions\"\n", 1},
		{"unrelated scalar declaration", FpathHygiene{}, "builtin typeset other=/opt/functions\n", 0},
		{"operand-free declaration", SpecialParamShadow{}, "example_f() { builtin local; }\n", 0},
		{"ordinary valueless zero", ZeroHandling{}, "builtin typeset 0\nprint -r -- $0\n", 1},
		{"query assignment is still read-only", ZeroHandling{}, "builtin typeset -p 0=ignored\n", 0},
		{"scalar path hazards", FpathHygiene{}, "builtin typeset FPATH=/opt/bin:/opt/test:/opt/functions\n", 3},
		{"dynamic zero option", ZeroHandling{}, "builtin typeset $flags 0=changed\nprint -r -- $0\n", 1},
		{"valueless zero declaration", ZeroHandling{}, "typeset 0\nprint -r -- $0\n", 1},
		{"static declaration stops cascading", ZeroHandling{}, "builtin typeset 0=changed\nprint -r -- $0\n", 1},
		{"tied second parameter is declared", PersistentParameterNamespace{}, "builtin typeset -T _example_scalar foreign_array :\n", 1},
		{"quoted dynamic name stays unknown", PersistentParameterNamespace{}, "builtin typeset -g \"$name\"=changed\n", 0},
		{"glued quoted dynamic name stays unknown", SpecialParamShadow{}, "example_f() { builtin local ZSH_\"$name\"=1; }\n", 0},
		{"bare test parameter is not initialization", SpecialParamShadow{}, "example_f() { builtin local OSTYPE; }\n", 1},
		{"letter bounds prove local", SharedPluginsRegistry{}, "example_f() { builtin local Plugins=aAzZ09; Plugins=changed; }\n", 0},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			file, err := parse.Parse(strings.NewReader(test.src), "example.zsh")
			if err != nil {
				t.Fatal(err)
			}
			got := analyzer.New(test.rule).AnalyzeSource(file, "example.zsh", sourceContext(projectconfig.KindPlugin, projectconfig.ProfileSourcedLibrary, "example"))
			if _, fpath := test.rule.(FpathHygiene); fpath {
				got = analyzer.New(test.rule).Analyze(file, "example.zsh")
			}
			if len(got) != test.want {
				t.Fatalf("diagnostics = %+v, want %d", got, test.want)
			}
		})
	}
}

func TestSpecialParamShadowDeclarationCalls(t *testing.T) {
	for _, prefix := range []string{"builtin ", "noglob ", "- ", "nocorrect "} {
		t.Run(prefix, func(t *testing.T) {
			source := "example_f() { " + prefix + "local ZSH_VERSION=1; }\n"
			file, err := parse.Parse(strings.NewReader(source), "example.zsh")
			if err != nil {
				t.Fatal(err)
			}
			got := analyzer.New(SpecialParamShadow{}).Analyze(file, "example.zsh")
			requireSpecialParamNames(t, got, []string{"ZSH_VERSION"})
		})
	}
}

func TestZeroDeclarationCalls(t *testing.T) {
	for _, prefix := range []string{"", "builtin ", "noglob ", "- ", "nocorrect "} {
		for _, test := range []struct {
			name string
			src  string
			want int
		}{
			{"assignment", prefix + "typeset 0=changed\n", 1},
			{"dynamic value", prefix + "typeset 0=$value\n", 1},
			{"read", prefix + "typeset -p 0\n", 0},
			{"no assignment", prefix + "typeset zero=changed\n", 0},
			{"function", "example_f() { " + prefix + "typeset 0=changed; }\n", 0},
		} {
			t.Run(prefix+test.name, func(t *testing.T) {
				file, err := parse.Parse(strings.NewReader(test.src), "example.zsh")
				if err != nil {
					t.Fatal(err)
				}
				got := analyzer.New(ZeroHandling{}).AnalyzeSource(file, "example.zsh", sourceContext(projectconfig.KindPlugin, projectconfig.ProfileSourcedLibrary, "example"))
				if len(got) != test.want {
					t.Fatalf("diagnostics = %+v, want %d", got, test.want)
				}
			})
		}
	}
}

func TestFpathDeclarationCalls(t *testing.T) {
	for _, prefix := range []string{"", "builtin ", "noglob ", "- ", "nocorrect "} {
		for _, test := range []struct {
			name string
			src  string
			want int
		}{
			{"overwrite", prefix + "typeset FPATH=/opt/functions\n", 1},
			{"localization", "example_f() { " + prefix + "local -a fpath; fpath=( /opt/functions ); }\n", 0},
			{"global", "example_f() { " + prefix + "typeset -g FPATH=/opt/functions; }\n", 1},
			{"print", prefix + "typeset -p FPATH\n", 0},
		} {
			t.Run(prefix+test.name, func(t *testing.T) {
				file, err := parse.Parse(strings.NewReader(test.src), "example.zsh")
				if err != nil {
					t.Fatal(err)
				}
				got := analyzer.New(FpathHygiene{}).Analyze(file, "example.zsh")
				if len(got) != test.want {
					t.Fatalf("diagnostics = %+v, want %d", got, test.want)
				}
			})
		}
	}
}

func TestPersistentDeclarationCalls(t *testing.T) {
	for _, prefix := range []string{"", "builtin ", "noglob ", "- ", "nocorrect "} {
		for _, test := range []struct {
			name string
			rule analyzer.Rule
			src  string
			want int
		}{
			{"global", PersistentParameterNamespace{}, "example_f() { " + prefix + "typeset -g foreign_state=1; }\n", 1},
			{"local", PersistentParameterNamespace{}, "example_f() { " + prefix + "local foreign_state=1; }\n", 0},
			{"registry", SharedPluginsRegistry{}, prefix + "typeset -gA Plugins\n", 1},
			{"local registry", SharedPluginsRegistry{}, "example_f() { " + prefix + "local -A Plugins; Plugins[item]=value; }\n", 0},
			{"registry query", SharedPluginsRegistry{}, prefix + "typeset -p Plugins\n", 0},
		} {
			t.Run(prefix+test.name, func(t *testing.T) {
				file, err := parse.Parse(strings.NewReader(test.src), "example.zsh")
				if err != nil {
					t.Fatal(err)
				}
				got := analyzer.New(test.rule).AnalyzeSource(file, "example.zsh", sourceContext(projectconfig.KindPlugin, projectconfig.ProfileSourcedLibrary, "example"))
				if len(got) != test.want {
					t.Fatalf("diagnostics = %+v, want %d", got, test.want)
				}
				for _, diagnostic := range got {
					if diagnostic.Severity != diag.Warning {
						t.Fatalf("severity = %v, want warning", diagnostic.Severity)
					}
				}
			})
		}
	}
}
