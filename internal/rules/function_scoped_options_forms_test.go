package rules

import (
	"strings"
	"testing"

	"github.com/z-shell/zsh-lint/internal/analyzer"
	"github.com/z-shell/zsh-lint/internal/parse"
	"github.com/z-shell/zsh-lint/internal/projectconfig"
)

func TestFunctionScopedOptionsAutoloadForms(t *testing.T) {
	const trace = "${=${options[xtrace]:#off}:+-o xtrace}"
	for _, test := range []struct {
		name, path, src string
		want            int
	}{
		{"xtrace bare body needs known IFS", "functions/handler", "builtin emulate -L zsh " + trace + "\nprint ready\n", 1},
		{"xtrace reset needs known IFS", "functions/handler", "builtin emulate -LR zsh " + trace + "\nprint ready\n", 1},
		{"same name wrapper", "functions/handler", "handler() {\n builtin emulate -L zsh\n print ready\n}\n", 0},
		{"wrapper xtrace needs known IFS", "functions/handler", "handler() {\n builtin emulate -L zsh " + trace + "\n print ready\n}\n", 1},
		{"windows wrapper path", `C:\plugin\functions\handler`, "handler() { setopt local_options; print ready; }\n", 0},
		{"wrapper leading guard", "functions/handler", "handler() { [[ -n $ready ]] || return; emulate -L zsh; print ready; }\n", 0},
		{"unscoped wrapper", "functions/handler", "handler() { print ready; }\n", 1},
		{"unscoped bare body", "functions/handler", "print ready\n", 1},
		{"late wrapper scoping", "functions/handler", "handler() { print ready; emulate -L zsh; }\n", 1},
		{"mismatched wrapper", "functions/handler", "other() { emulate -L zsh; print ready; }\n", 1},
		{"wrapper initialization", "functions/handler", "handler() { emulate -L zsh; }\nprint initializing\n", 1},
		{"wrapper redirection", "functions/handler", "handler() { emulate -L zsh; } >$target\n", 1},
		{"completion callback", "functions/_handler", "#compdef handler\n_arguments '*:file:_files'\n", 0},
		{"underscore alone", "functions/_handler", "print ready\n", 1},
		{"non completion name", "functions/handler", "#compdef handler\nprint ready\n", 1},
		{"later completion tag", "functions/_handler", "# comment\n#compdef handler\nprint ready\n", 1},
		{"autoload tag alone", "functions/_handler", "#autoload\nprint ready\n", 1},
		{"wrong emulation", "functions/handler", "emulate -L sh " + trace + "\nprint ready\n", 1},
		{"no local flag", "functions/handler", "emulate zsh " + trace + "\nprint ready\n", 1},
		{"arbitrary dynamic arguments", "functions/handler", "emulate -L zsh ${=dynamic}\nprint ready\n", 1},
		{"unsafe alternative", "functions/handler", "emulate -L zsh ${=${options[xtrace]:#off}:+-c true}\nprint ready\n", 1},
		{"unrecognized option expansion", "functions/handler", "emulate -L zsh ${=${options[xtrace]:#off}:+-o no_local_options}\nprint ready\n", 1},
		{"concatenated expansion", "functions/handler", "emulate -L zsh " + trace + "suffix\nprint ready\n", 1},
		{"expansion as option value", "functions/handler", "emulate -L zsh -o " + trace + "\nprint ready\n", 1},
		{"command mode after expansion", "functions/handler", "emulate -L zsh " + trace + " -c true\nprint ready\n", 1},
		{"trailing option after trace", "functions/handler", "emulate -L zsh " + trace + " +o local_options\nprint ready\n", 1},
		{"trailing inverse option after trace", "functions/handler", "emulate -L zsh " + trace + " -o no_local_options\nprint ready\n", 1},
		{"trailing long option after trace", "functions/handler", "emulate -L zsh " + trace + " --no-localoptions\nprint ready\n", 1},
		{"listing mode", "functions/handler", "emulate -lL zsh " + trace + "\nprint ready\n", 1},
		{"local flag overrides literal disabling", "functions/handler", "emulate -L zsh +o local_options\nprint ready\n", 0},
		{"local flag overrides inverse option", "functions/handler", "emulate -L zsh -o no_local_options\nprint ready\n", 0},
		{"static options before trace need known IFS", "functions/handler", "emulate -L zsh -o extended_glob " + trace + "\nprint ready\n", 1},
		{"only recognize trailing trace idiom", "functions/handler", "emulate -L zsh " + trace + " -o extended_glob\nprint ready\n", 1},
		{"suppressed unscoped wrapper", "functions/handler", "handler() {\n# zsh-lint disable=plugin/function-scoped-options -- caller state is intentional\nprint ready\n}\n", 0},
		{"suppressed wrapper declaration", "functions/handler", "# zsh-lint disable=plugin/function-scoped-options -- caller state is intentional\nhandler() {\n print ready\n}\n", 0},
		{"suppressed wrapper after blank and comment", "functions/handler", "# zsh-lint disable=plugin/function-scoped-options -- caller state is intentional\n\n# wrapper\nhandler() { print ready; }\n", 0},
		{"suppressed wrapper trailing directive", "functions/handler", "handler() { # zsh-lint disable=plugin/function-scoped-options -- caller state is intentional\n print ready\n}\n", 0},
		{"background wrapper", "functions/handler", "handler() { emulate -L zsh; print ready; } &\n", 1},
		{"multiple wrapper names", "functions/handler", "function handler other { emulate -L zsh; print ready; }\n", 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			got := analyzeFunctionScopedOptions(t, test.path, test.src)
			if len(got) != test.want {
				t.Fatalf("lines = %v, want %d findings", got, test.want)
			}
		})
	}
}

func TestFunctionScopedOptionsConfiguredForms(t *testing.T) {
	for _, test := range []struct {
		name, src string
		kind      projectconfig.ProjectKind
		profile   projectconfig.Profile
		want      int
	}{
		{"configured plugin wrapper", "handler() { emulate -L zsh; print ready; }", projectconfig.KindPlugin, projectconfig.ProfileAutoloadFunction, 0},
		{"configured suppressed wrapper", "# zsh-lint disable=plugin/function-scoped-options -- caller state is intentional\nhandler() { print ready; }", projectconfig.KindPlugin, projectconfig.ProfileAutoloadFunction, 0},
		{"configured annex xtrace needs known IFS", "builtin emulate -L zsh ${=${options[xtrace]:#off}:+-o xtrace}\nprint ready", projectconfig.KindZiAnnex, projectconfig.ProfileAutoloadFunction, 1},
		{"configured unscoped body", "print ready", projectconfig.KindPlugin, projectconfig.ProfileAutoloadFunction, 1},
		{"sourced profile", "print ready", projectconfig.KindPlugin, projectconfig.ProfileSourcedLibrary, 0},
		{"tool profile", "print ready", projectconfig.KindTool, projectconfig.ProfileAutoloadFunction, 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			file, err := parse.Parse(strings.NewReader(test.src), "autoload/handler")
			if err != nil {
				t.Fatal(err)
			}
			ds := analyzer.New(FunctionScopedOptions{}).AnalyzeSource(file, "autoload/handler", configuredSource(test.kind, test.profile, ""))
			if len(ds) != test.want {
				t.Fatalf("diagnostics = %v, want %d", ds, test.want)
			}
		})
	}
}

func TestFunctionScopedOptionsWrapperSuppressionScope(t *testing.T) {
	for _, test := range []struct {
		name, source, meta string
		line               int
	}{
		{"other rule", "# zsh-lint disable=security/eval\nhandler() {\n print ready\n}\n", "meta/unused-suppression", 3},
		{"malformed", "# zsh-lint enable=plugin/function-scoped-options\nhandler() {\n print ready\n}\n", "meta/malformed-suppression", 3},
		{"later statement", "handler() {\n print first\n# zsh-lint disable=plugin/function-scoped-options -- later statement only\n print second\n}\n", "meta/unused-suppression", 2},
	} {
		t.Run(test.name, func(t *testing.T) {
			file, err := parse.Parse(strings.NewReader(test.source), "functions/handler")
			if err != nil {
				t.Fatal(err)
			}
			ds := analyzer.New(FunctionScopedOptions{}).Analyze(file, "functions/handler")
			if len(ds) != 2 {
				t.Fatalf("diagnostics = %v, want target and %s", ds, test.meta)
			}
			var target, meta bool
			for _, d := range ds {
				switch string(d.RuleID) {
				case "plugin/function-scoped-options":
					target = d.Range.Start.Line == test.line
				case test.meta:
					meta = true
				}
			}
			if !target || !meta {
				t.Fatalf("diagnostics = %v, want target on line %d and %s", ds, test.line, test.meta)
			}
		})
	}
}
