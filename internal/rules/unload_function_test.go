package rules

import (
	"strings"
	"testing"

	"github.com/z-shell/zsh-lint/internal/analyzer"
	"github.com/z-shell/zsh-lint/internal/diag"
	"github.com/z-shell/zsh-lint/internal/parse"
	"github.com/z-shell/zsh-lint/internal/projectconfig"
)

func TestUnloadFunction(t *testing.T) {
	tests := []struct {
		name     string
		src      string
		path     string
		want     int
		severity diag.Severity
	}{
		{name: "hook registered without unload function", src: "autoload -Uz add-zsh-hook\nadd-zsh-hook precmd _my_precmd\n", path: "my-plugin.plugin.zsh", want: 1, severity: diag.Hint},
		{name: "unload function missing self-unfunction", src: "my_plugin_unload() {\n  add-zsh-hook -d precmd _my_precmd\n  unfunction _my_precmd\n}\n", path: "my-plugin.plugin.zsh", want: 1, severity: diag.Hint},
		{name: "unload function with indiscriminate function wipe", src: "my_plugin_unload() {\n  unfunction ${(k)functions}\n  unfunction my_plugin_unload\n}\n", path: "my-plugin.plugin.zsh", want: 1, severity: diag.Warning},
		{name: "compliant hook and unload function", src: "add-zsh-hook precmd _my_precmd\nmy_plugin_unload() {\n  add-zsh-hook -d precmd _my_precmd\n  unfunction _my_precmd my_plugin_unload\n}\n", path: "my-plugin.plugin.zsh"},
		{name: "file inside functions directory", src: "add-zsh-hook precmd _my_precmd\n", path: "functions/.handler"},
		{name: "ordinary source is not an entrypoint", src: "add-zsh-hook precmd _my_precmd\n", path: "lib/widgets.zsh"},
		{name: "multiple registrations report once", src: "add-zsh-hook precmd _my_precmd\nzle -N example-widget\n", path: "my-plugin.plugin.zsh", want: 1, severity: diag.Hint},
		{name: "hook array append is a registration", src: "precmd_functions+=(_my_precmd)\n", path: "my-plugin.plugin.zsh", want: 1, severity: diag.Hint},
		{name: "scalar hook append is a registration", src: "precmd_functions+=_my_precmd\n", path: "my-plugin.plugin.zsh", want: 1, severity: diag.Hint},
		{name: "theme entrypoint", src: "zle -N example-widget\n", path: "themes/example.zsh-theme", want: 1, severity: diag.Hint},
		{name: "init entrypoint", src: "add-zsh-hook precmd _my_precmd\n", path: "modules/example/init.zsh", want: 1, severity: diag.Hint},
		{name: "windows init entrypoint", src: "add-zsh-hook precmd _my_precmd\n", path: `modules\example\init.zsh`, want: 1, severity: diag.Hint},
		{name: "near miss init basename", src: "add-zsh-hook precmd _my_precmd\n", path: "modules/my-init.zsh"},
		{name: "entrypoint suffix is exact", src: "add-zsh-hook precmd _my_precmd\n", path: "example.plugin.zsh.backup"},
		{name: "hook array declaration", src: "typeset -ga precmd_functions+=(_my_precmd)\n", path: "my-plugin.plugin.zsh", want: 1, severity: diag.Hint},
		{name: "mixed registrations report once", src: "precmd_functions+=(_my_precmd)\nadd-zsh-hook preexec _my_preexec\n", path: "my-plugin.plugin.zsh", want: 1, severity: diag.Hint},
		{name: "hook array with unload", src: "precmd_functions+=(_my_precmd)\nmy_plugin_unload() { unfunction my_plugin_unload }\n", path: "my-plugin.plugin.zsh"},
		{name: "empty hook array append", src: "precmd_functions+=()\n", path: "my-plugin.plugin.zsh"},
		{name: "empty scalar hook append", src: "precmd_functions+=\n", path: "my-plugin.plugin.zsh"},
		{name: "hook array reset", src: "precmd_functions=()\n", path: "my-plugin.plugin.zsh"},
		{name: "hook array replacement is not an append", src: "precmd_functions=(_my_precmd)\n", path: "my-plugin.plugin.zsh"},
		{name: "other array append", src: "handlers+=(_my_precmd)\n", path: "my-plugin.plugin.zsh"},
		{name: "hook name substring", src: "precmd_functions_extra+=(_my_precmd)\n", path: "my-plugin.plugin.zsh"},
		{name: "hook element append", src: "precmd_functions[1]+=suffix\n", path: "my-plugin.plugin.zsh"},
		{name: "suppressed hook array finding", src: "# zsh-lint disable=plugin/unload-function -- static plugin\nprecmd_functions+=(_my_precmd)\n", path: "my-plugin.plugin.zsh"},
		// The count of a repeat loop is a word, not a command (#281): a count
		// spelled like a hook command is not a registration, and a
		// registration in the body still is.
		{name: "repeat count spelled add-zsh-hook", src: "repeat add-zsh-hook print hi\n", path: "my-plugin.plugin.zsh"},
		{name: "repeat count spelled add-zle-hook-widget", src: "repeat add-zle-hook-widget print hi\n", path: "my-plugin.plugin.zsh"},
		{name: "hook registered in a repeat body", src: "repeat 2 add-zsh-hook precmd _my_precmd\n", path: "my-plugin.plugin.zsh", want: 1, severity: diag.Hint},
		{name: "suppressed hook finding", src: "# zsh-lint disable=plugin/unload-function -- static plugin\nadd-zsh-hook precmd _my_precmd\n", path: "my-plugin.plugin.zsh"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			file, err := parse.Parse(strings.NewReader(test.src), test.path)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			diagnostics := analyzer.New(UnloadFunction{}).Analyze(file, test.path)
			if len(diagnostics) != test.want {
				t.Fatalf("diagnostics = %+v, want %d", diagnostics, test.want)
			}
			if test.want > 0 && diagnostics[0].Severity != test.severity {
				t.Fatalf("severity = %v, want %v", diagnostics[0].Severity, test.severity)
			}
		})
	}
}

func TestProjectUnloadLifecycleAcrossFiles(t *testing.T) {
	entry, err := parse.Parse(strings.NewReader("add-zsh-hook precmd _tick\n"), "plugin.zsh")
	if err != nil {
		t.Fatalf("parse entry: %v", err)
	}
	unload, err := parse.Parse(strings.NewReader("example_plugin_unload() { unfunction example_plugin_unload }\n"), "lib/state.zsh")
	if err != nil {
		t.Fatalf("parse unload: %v", err)
	}
	source := configuredSource(projectconfig.KindPlugin, projectconfig.ProfileSourcedLibrary, "")
	an := analyzer.New(UnloadFunction{}, ProjectUnloadLifecycle{})
	diagnostics := an.AnalyzeProject([]analyzer.ProjectInput{
		{File: entry, Path: "plugin.zsh", Source: source},
		{File: unload, Path: "lib/state.zsh", Source: source},
	})
	if len(diagnostics) != 0 {
		t.Fatalf("cross-file unload diagnostics = %+v, want none", diagnostics)
	}

	diagnostics = an.AnalyzeProject([]analyzer.ProjectInput{{File: entry, Path: "plugin.zsh", Source: source}})
	if len(diagnostics) != 1 || diagnostics[0].RuleID != "plugin/project-unload-lifecycle" {
		t.Fatalf("missing project unload diagnostics = %+v, want one project finding", diagnostics)
	}
}

func TestUnloadFunctionExactLifecycleMatching(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want int
	}{
		{name: "near-miss unload suffix does not satisfy lifecycle", src: "add-zsh-hook precmd _tick\nexample_unload() { unfunction example_unload }\n", want: 1},
		{name: "substring does not satisfy self removal", src: "example_plugin_unload() { unfunction prefix-example_plugin_unload-suffix }\n", want: 1},
		{name: "exact parameter satisfies self removal", src: "example_plugin_unload() { unfunction \"$0\" }\n"},
		{name: "zle widget registration requires unload", src: "zle -N example-widget example_widget\n", want: 1},
		{name: "zle widget deletion is not registration", src: "zle -D example-widget\n"},
		{name: "hook deletion is not registration", src: "add-zsh-hook -d precmd _tick\n"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			file, err := parse.Parse(strings.NewReader(test.src), "example.plugin.zsh")
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			diagnostics := analyzer.New(UnloadFunction{}).Analyze(file, "example.plugin.zsh")
			if len(diagnostics) != test.want {
				t.Fatalf("diagnostics = %+v, want %d", diagnostics, test.want)
			}
		})
	}
}

func TestUnloadFunctionConfiguredHookArrays(t *testing.T) {
	for _, test := range []struct {
		src  string
		want int
	}{
		{"precmd_functions+=(_tick)\npreexec_functions+=(_run)\n", 2},
		{"typeset -ga precmd_functions+=(_tick)\n", 1},
		{"precmd_functions+=_tick\n", 1},
		{"precmd_functions=(_tick)\n", 0},
		{"precmd_functions+=()\n", 0},
		{"handlers+=(_tick)\n", 0},
		{"precmd_functions[1]+=suffix\n", 0},
	} {
		src := test.src
		file, err := parse.Parse(strings.NewReader(src), "lib/hooks.zsh")
		if err != nil {
			t.Fatalf("parse: %v", err)
		}
		source := configuredSource(projectconfig.KindPlugin, projectconfig.ProfileSourcedLibrary, "")
		an := analyzer.New(UnloadFunction{}, ProjectUnloadLifecycle{})
		got := an.AnalyzeSource(file, "lib/hooks.zsh", source)
		if len(got) != test.want {
			t.Fatalf("%q: configured diagnostics = %v, want %d", src, got, test.want)
		}
		for _, diagnostic := range got {
			if diagnostic.RuleID != "plugin/project-unload-lifecycle" {
				t.Fatalf("configured diagnostic = %v, want project lifecycle rule", diagnostic)
			}
		}
		unload, err := parse.Parse(strings.NewReader("example_plugin_unload() { unfunction example_plugin_unload }\n"), "lib/unload.zsh")
		if err != nil {
			t.Fatalf("parse unload: %v", err)
		}
		got = an.AnalyzeProject([]analyzer.ProjectInput{
			{File: file, Path: "lib/hooks.zsh", Source: source},
			{File: unload, Path: "lib/unload.zsh", Source: source},
		})
		if len(got) != 0 {
			t.Fatalf("cross-file hook array diagnostics = %v, want none", got)
		}
	}
}

func TestUnloadFunctionCommandForms(t *testing.T) {
	for _, registration := range []struct {
		name string
		args string
	}{
		{"zle", "-N example-widget example_widget"},
		{"add-zsh-hook", "precmd _tick"},
		{"add-zle-hook-widget", "line-init _tick"},
	} {
		for _, command := range commandForms(registration.name) {
			t.Run(command, func(t *testing.T) {
				file, err := parse.Parse(strings.NewReader(command+" "+registration.args+"\n"), "example.plugin.zsh")
				if err != nil {
					t.Fatalf("parse: %v", err)
				}
				if got := analyzer.New(UnloadFunction{}).Analyze(file, "example.plugin.zsh"); len(got) != 1 {
					t.Fatalf("unconfigured diagnostics = %v, want one", got)
				}
				got := analyzer.New(ProjectUnloadLifecycle{}).AnalyzeSource(file, "plugin.zsh",
					configuredSource(projectconfig.KindPlugin, projectconfig.ProfileSourcedLibrary, ""))
				if len(got) != 1 {
					t.Fatalf("project diagnostics = %v, want one", got)
				}
			})
		}
	}
	for _, command := range commandForms("unfunction") {
		t.Run(command, func(t *testing.T) {
			for _, test := range []struct {
				args string
				want int
			}{
				{"example_plugin_unload", 0},
				{`${(k)functions} example_plugin_unload`, 1},
				{"g", 1},
			} {
				file, err := parse.Parse(strings.NewReader("example_plugin_unload() { "+command+" "+test.args+"; }"), "example.plugin.zsh")
				if err != nil {
					t.Fatalf("parse: %v", err)
				}
				if got := analyzer.New(UnloadFunction{}).Analyze(file, "example.plugin.zsh"); len(got) != test.want {
					t.Fatalf("args %s: diagnostics = %v, want %d", test.args, got, test.want)
				}
			}
		})
	}
}

func TestUnloadFunctionPrefixedDeregistration(t *testing.T) {
	for _, test := range []struct {
		name string
		args string
	}{
		{"zle", "-D example-widget"},
		{"add-zsh-hook", "-d precmd _tick"},
		{"add-zle-hook-widget", "-D line-init _tick"},
	} {
		for _, command := range commandForms(test.name) {
			t.Run(command, func(t *testing.T) {
				file, err := parse.Parse(strings.NewReader(command+" "+test.args+"\n"), "example.plugin.zsh")
				if err != nil {
					t.Fatalf("parse: %v", err)
				}
				if got := analyzer.New(UnloadFunction{}).Analyze(file, "example.plugin.zsh"); len(got) != 0 {
					t.Fatalf("diagnostics = %v, want none", got)
				}
			})
		}
	}
}
