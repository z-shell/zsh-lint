package rules

import (
	"strings"
	"testing"

	"github.com/z-shell/zsh-lint/internal/analyzer"
	"github.com/z-shell/zsh-lint/internal/diag"
	"github.com/z-shell/zsh-lint/internal/parse"
	"github.com/z-shell/zsh-lint/internal/projectconfig"
)

func TestFunctionNamespaceDeclarations(t *testing.T) {
	tests := []struct {
		name       string
		source     string
		identifier string
		want       int
	}{
		{name: "public underscore", source: "example_refresh() { :; }\n", want: 0},
		{name: "portable private", source: "_example_precmd() { :; }\n", want: 0},
		{name: "hyphenated identifier", source: "_zsh_fancy_completions_state() { :; }\n", identifier: "zsh-fancy-completions", want: 0},
		{name: "legacy private role", source: "function .example_private { :; }\n", want: 1},
		{name: "legacy hook role", source: "function →example_hook { :; }\n", want: 1},
		{name: "legacy output role", source: "function +example_output { :; }\n", want: 1},
		{name: "legacy debug role", source: "function /example_debug { :; }\n", want: 1},
		{name: "legacy api role", source: "function @example_api { :; }\n", want: 1},
		{name: "missing namespace", source: "refresh() { :; }\n", want: 1},
		{name: "prefix without boundary", source: "exampled_refresh() { :; }\n", want: 1},
		{name: "role without namespace", source: "function .refresh { :; }\n", want: 1},
		{name: "multiple declaration names", source: "function example_one foreign { :; }\n", want: 1},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			identifier := test.identifier
			if identifier == "" {
				identifier = "example"
			}
			diagnostics := analyzeFunctionNamespace(t, test.source, "plugin.zsh", sourcedPlugin(identifier))
			got := diagnosticsByID(diagnostics, "plugin/function-namespace")
			if len(got) != test.want {
				t.Fatalf("finding count = %d, want %d: %+v", len(got), test.want, diagnostics)
			}
		})
	}
}

func TestFunctionNamespaceDeclarationRange(t *testing.T) {
	diagnostics := analyzeFunctionNamespace(t, "print ok\nfunction refresh { :; }\n", "plugin.zsh", sourcedPlugin("example"))
	got := diagnosticsByID(diagnostics, "plugin/function-namespace")
	if len(got) != 1 {
		t.Fatalf("finding count = %d, want 1: %+v", len(got), diagnostics)
	}
	if got[0].Range.Start.Line != 2 || got[0].Range.Start.Column != 10 || got[0].Range.End.Column != 17 {
		t.Errorf("range = %+v, want line 2 columns 10..17", got[0].Range)
	}
	if !strings.Contains(got[0].Message, `Function name "refresh"`) {
		t.Errorf("message = %q, want function name", got[0].Message)
	}
}

func TestFunctionNamespaceZiAnnexDeclaration(t *testing.T) {
	context := sourceContext(projectconfig.KindZiAnnex, projectconfig.ProfileSourcedLibrary, "za-example")
	diagnostics := analyzeFunctionNamespace(t, ".foreign_handler() { :; }\n", "z-a-example.plugin.zsh", context)
	got := diagnosticsByID(diagnostics, "plugin/function-namespace")
	if len(got) != 1 {
		t.Fatalf("finding count = %d, want 1: %+v", len(got), diagnostics)
	}
}

func TestFunctionNamespaceNoOpProfiles(t *testing.T) {
	tests := []struct {
		name    string
		context projectconfig.SourceContext
	}{
		{name: "unconfigured", context: projectconfig.SourceContext{}},
		{name: "standalone", context: sourceContext(projectconfig.KindPlugin, projectconfig.ProfileStandaloneExecutable, "example")},
		{name: "startup", context: sourceContext(projectconfig.KindPlugin, projectconfig.ProfileStartupFile, "example")},
		{name: "test fixture", context: sourceContext(projectconfig.KindPlugin, projectconfig.ProfileTestFixture, "example")},
		{name: "library project", context: sourceContext(projectconfig.KindLibrary, projectconfig.ProfileSourcedLibrary, "example")},
		{name: "empty identifier", context: sourceContext(projectconfig.KindPlugin, projectconfig.ProfileSourcedLibrary, "")},
		{name: "unknown config version", context: sourcedPlugin("example")},
	}
	tests[len(tests)-1].context.ConfigVersion = projectconfig.CurrentVersion + 1

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			diagnostics := analyzeFunctionNamespace(t, "refresh() { :; }\n", "script.zsh", test.context)
			if got := diagnosticsByID(diagnostics, "plugin/function-namespace"); len(got) != 0 {
				t.Errorf("findings = %+v, want none", got)
			}
		})
	}
}

func TestFunctionNamespaceAutoloadBasenames(t *testing.T) {
	tests := []struct {
		name       string
		path       string
		role       projectconfig.SourceRole
		identifier string
		want       int
		message    string
	}{
		{name: "public", path: "functions/example_refresh", want: 0},
		{name: "private", path: "functions/_example_refresh", want: 0},
		{name: "legacy private", path: "functions/.example_refresh", want: 1},
		{name: "legacy hook", path: "functions/→example_hook", want: 1},
		{name: "hyphenated identifier", path: "functions/_zsh_fancy_completions_state", identifier: "zsh-fancy-completions", want: 0},
		{name: "missing namespace", path: "functions/.refresh", want: 1, message: `Autoload function name ".refresh"`},
		{name: "completion", path: "completions/_git", role: projectconfig.RoleCompletion, want: 0},
		{name: "completion requires underscore", path: "completions/example_git", role: projectconfig.RoleCompletion, want: 1, message: "must use the _command form"},
		{name: "bare underscore is not completion", path: "completions/_", role: projectconfig.RoleCompletion, want: 1, message: "must use the _command form"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			identifier := test.identifier
			if identifier == "" {
				identifier = "example"
			}
			context := sourceContext(projectconfig.KindPlugin, projectconfig.ProfileAutoloadFunction, identifier)
			context.Role = test.role
			diagnostics := analyzeFunctionNamespace(t, "builtin emulate -L zsh\n", test.path, context)
			got := diagnosticsByID(diagnostics, "plugin/function-namespace")
			if len(got) != test.want {
				t.Fatalf("finding count = %d, want %d: %+v", len(got), test.want, diagnostics)
			}
			if len(got) != 0 {
				if got[0].Range.IsValid() {
					t.Errorf("basename finding range = %+v, want unpositioned", got[0].Range)
				}
				if test.message != "" && !strings.Contains(got[0].Message, test.message) {
					t.Errorf("message = %q, want substring %q", got[0].Message, test.message)
				}
			}
		})
	}
}

func TestFunctionNamespaceSuppression(t *testing.T) {
	tests := []struct {
		name         string
		path         string
		source       string
		wantRule     int
		wantMeta     diag.RuleID
		wantContains string
	}{
		{
			name:     "header suppresses basename",
			path:     "functions/.refresh",
			source:   "# zsh-lint disable=plugin/function-namespace -- intentional external API\nbuiltin emulate -L zsh\n",
			wantRule: 0,
		},
		{
			name:         "header is stale for valid basename",
			path:         "functions/_example_refresh",
			source:       "# zsh-lint disable=plugin/function-namespace -- intentional external API\nbuiltin emulate -L zsh\n",
			wantRule:     0,
			wantMeta:     "meta/unused-suppression",
			wantContains: "matched no finding",
		},
		{
			name:         "malformed header suppresses nothing",
			path:         "functions/.refresh",
			source:       "# zsh-lint enable=plugin/function-namespace\nbuiltin emulate -L zsh\n",
			wantRule:     1,
			wantMeta:     "meta/malformed-suppression",
			wantContains: "unknown verb",
		},
		{
			name:         "unknown header rule is stale",
			path:         "functions/.refresh",
			source:       "# zsh-lint disable=plugin/not-a-rule\nbuiltin emulate -L zsh\n",
			wantRule:     1,
			wantMeta:     "meta/unused-suppression",
			wantContains: "unknown rule ID",
		},
		{
			name:         "directive after code cannot suppress basename",
			path:         "functions/.refresh",
			source:       "builtin emulate -L zsh\n# zsh-lint disable=plugin/function-namespace\n",
			wantRule:     1,
			wantMeta:     "meta/unused-suppression",
			wantContains: "matched no finding",
		},
		{
			name:     "preceding suppresses declaration",
			path:     "plugin.zsh",
			source:   "# zsh-lint disable=plugin/function-namespace -- external API\nrefresh() { :; }\n",
			wantRule: 0,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			context := sourceContext(projectconfig.KindPlugin, projectconfig.ProfileAutoloadFunction, "example")
			if test.path == "plugin.zsh" {
				context.Profile = projectconfig.ProfileSourcedLibrary
			}
			diagnostics := analyzeFunctionNamespace(t, test.source, test.path, context)
			if got := diagnosticsByID(diagnostics, "plugin/function-namespace"); len(got) != test.wantRule {
				t.Errorf("rule findings = %d, want %d: %+v", len(got), test.wantRule, diagnostics)
			}
			if test.wantMeta == "" {
				if got := metaDiagnostics(diagnostics); len(got) != 0 {
					t.Errorf("meta findings = %+v, want none", got)
				}
				return
			}
			got := diagnosticsByID(diagnostics, test.wantMeta)
			if len(got) != 1 || !strings.Contains(got[0].Message, test.wantContains) {
				t.Errorf("meta findings = %+v, want one %s containing %q", got, test.wantMeta, test.wantContains)
			}
		})
	}
}

func analyzeFunctionNamespace(t *testing.T, source, path string, context projectconfig.SourceContext) diag.Diagnostics {
	t.Helper()
	file, err := parse.Parse(strings.NewReader(source), path)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	return analyzer.New(FunctionNamespace{}).AnalyzeSource(file, path, context)
}

func sourcedPlugin(identifier string) projectconfig.SourceContext {
	return sourceContext(projectconfig.KindPlugin, projectconfig.ProfileSourcedLibrary, identifier)
}

func sourceContext(kind projectconfig.ProjectKind, profile projectconfig.Profile, identifier string) projectconfig.SourceContext {
	return projectconfig.SourceContext{
		ConfigVersion:     projectconfig.CurrentVersion,
		ProjectKind:       kind,
		MinimumZsh:        "5.8",
		ProjectIdentifier: identifier,
		Profile:           profile,
		SourceRoot:        ".",
	}
}

func diagnosticsByID(diagnostics diag.Diagnostics, id diag.RuleID) diag.Diagnostics {
	var found diag.Diagnostics
	for _, diagnostic := range diagnostics {
		if diagnostic.RuleID == id {
			found = append(found, diagnostic)
		}
	}
	return found
}

func metaDiagnostics(diagnostics diag.Diagnostics) diag.Diagnostics {
	var found diag.Diagnostics
	for _, diagnostic := range diagnostics {
		if strings.HasPrefix(string(diagnostic.RuleID), "meta/") {
			found = append(found, diagnostic)
		}
	}
	return found
}
