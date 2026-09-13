package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const cliConfig = `{
  "version": 2,
  "project": {
    "kind": "plugin",
    "minimum_zsh": "5.8",
    "identifier": "example"
  },
  "sources": [
    {"root": ".", "profile": "sourced-library"},
    {"root": "functions", "profile": "autoload-function"}
  ]
}`

func TestRunUsageErrors(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "no inputs", want: usage},
		{name: "unknown flag", args: []string{"--unknown"}, want: "flag provided but not defined"},
		{name: "unknown format", args: []string{"--format=text", "test.zsh"}, want: "unsupported output format"},
		{name: "empty config", args: []string{"--config=", "test.zsh"}, want: "requires a non-empty path"},
		{name: "duplicate config", args: []string{"--config=one", "--config=two", "test.zsh"}, want: "may be specified only once"},
		{name: "config with no config", args: []string{"--config=one", "--no-config", "test.zsh"}, want: "mutually exclusive"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			if exit := run(test.args, &stdout, &stderr); exit != 2 {
				t.Errorf("run() exit = %d, want 2", exit)
			}
			if stdout.Len() != 0 {
				t.Errorf("stdout = %q, want empty", stdout.String())
			}
			if !strings.Contains(stderr.String(), test.want) {
				t.Errorf("stderr = %q, want substring %q", stderr.String(), test.want)
			}
		})
	}
}

func TestRunPreservesUnconfiguredBehavior(t *testing.T) {
	root := t.TempDir()
	filename := filepath.Join(root, "script.zsh")
	writeFile(t, filename, "eval $value\n")

	var stdout, stderr bytes.Buffer
	if exit := run([]string{filename}, &stdout, &stderr); exit != 1 {
		t.Fatalf("run() exit = %d, want 1", exit)
	}
	if !strings.Contains(stdout.String(), "[security/eval]") {
		t.Errorf("stdout = %q, want security/eval diagnostic", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Errorf("stderr = %q, want empty", stderr.String())
	}
}

func TestRunNoConfigDisablesDiscovery(t *testing.T) {
	root := t.TempDir()
	script := filepath.Join(root, "functions", "refresh")
	writeFile(t, filepath.Join(root, "zsh-lint.json"), cliConfig)
	writeFile(t, script, "builtin emulate -L zsh\n")

	var stdout, stderr bytes.Buffer
	if exit := run([]string{"--no-config", script}, &stdout, &stderr); exit != 0 {
		t.Fatalf("run() exit = %d, want 0; stderr = %q", exit, stderr.String())
	}
	if strings.Contains(stdout.String(), "[plugin/function-namespace]") {
		t.Errorf("stdout = %q, want discovery disabled", stdout.String())
	}
}

func TestRunWithConfiguration(t *testing.T) {
	root := t.TempDir()
	config := filepath.Join(root, "zsh-lint.json")
	script := filepath.Join(root, "functions", "example_run")
	writeFile(t, config, cliConfig)
	writeFile(t, script, "builtin emulate -L zsh\nprint ok\n")

	var stdout, stderr bytes.Buffer
	if exit := run([]string{"--config", config, script}, &stdout, &stderr); exit != 0 {
		t.Fatalf("run() exit = %d, want 0; stderr = %q", exit, stderr.String())
	}
	if stdout.Len() != 0 || stderr.Len() != 0 {
		t.Errorf("stdout = %q, stderr = %q, want both empty", stdout.String(), stderr.String())
	}
}

func TestRunAutoDiscoversConfiguration(t *testing.T) {
	root := t.TempDir()
	config := filepath.Join(root, "zsh-lint.json")
	script := filepath.Join(root, "functions", "refresh")
	writeFile(t, config, cliConfig)
	writeFile(t, script, "builtin emulate -L zsh\n")

	var stdout, stderr bytes.Buffer
	if exit := run([]string{script}, &stdout, &stderr); exit != 0 {
		t.Fatalf("run() exit = %d, want 0 for hint-only finding; stderr = %q", exit, stderr.String())
	}
	if !strings.Contains(stdout.String(), "[plugin/function-namespace]") {
		t.Errorf("stdout = %q, want auto-discovered project-profile diagnostic", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Errorf("stderr = %q, want empty", stderr.String())
	}
}

func TestRunAutoDiscoversPerProject(t *testing.T) {
	pluginRoot := filepath.Join(t.TempDir(), "plugin")
	toolRoot := filepath.Join(t.TempDir(), "tool")
	pluginScript := filepath.Join(pluginRoot, "functions", "refresh")
	toolScript := filepath.Join(toolRoot, "functions", "handler")
	writeFile(t, filepath.Join(pluginRoot, "zsh-lint.json"), cliConfig)
	writeFile(t, filepath.Join(toolRoot, "zsh-lint.json"), `{
  "version": 2,
  "project": {
    "kind": "tool",
    "minimum_zsh": "5.8",
    "identifier": "zunit"
  },
  "sources": [
    {"root": ".", "profile": "sourced-library"}
  ]
}`)
	writeFile(t, pluginScript, "builtin emulate -L zsh\n")
	writeFile(t, toolScript, "builtin emulate -L zsh\n")

	var stdout, stderr bytes.Buffer
	if exit := run([]string{pluginScript, toolScript}, &stdout, &stderr); exit != 0 {
		t.Fatalf("run() exit = %d, want 0 for hint-only findings; stderr = %q", exit, stderr.String())
	}
	if !strings.Contains(stdout.String(), pluginScript+": [plugin/function-namespace]") {
		t.Errorf("stdout = %q, want plugin project diagnostic", stdout.String())
	}
	if strings.Contains(stdout.String(), toolScript+": [plugin/function-namespace]") {
		t.Errorf("stdout = %q, want no plugin diagnostic for tool project", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Errorf("stderr = %q, want empty", stderr.String())
	}
}

func TestRunAutoDiscoveryKeepsProjectRulesWithinEachProject(t *testing.T) {
	completeRoot := filepath.Join(t.TempDir(), "complete")
	incompleteRoot := filepath.Join(t.TempDir(), "incomplete")
	completeEntry := filepath.Join(completeRoot, "complete.plugin.zsh")
	completeUnload := filepath.Join(completeRoot, "lib", "state.zsh")
	incompleteEntry := filepath.Join(incompleteRoot, "incomplete.plugin.zsh")
	for root, identifier := range map[string]string{completeRoot: "complete", incompleteRoot: "incomplete"} {
		writeFile(t, filepath.Join(root, "zsh-lint.json"), strings.ReplaceAll(cliConfig, "example", identifier))
	}
	writeFile(t, completeEntry, "add-zsh-hook precmd _complete_tick\n")
	writeFile(t, completeUnload, "complete_plugin_unload() { unfunction complete_plugin_unload }\n")
	writeFile(t, incompleteEntry, "add-zsh-hook precmd _incomplete_tick\n")

	var stdout, stderr bytes.Buffer
	if exit := run([]string{completeEntry, completeUnload, incompleteEntry}, &stdout, &stderr); exit != 0 {
		t.Fatalf("run() exit = %d, want 0 for hint-only findings; stderr = %q", exit, stderr.String())
	}
	if strings.Contains(stdout.String(), completeEntry+":1:1: [plugin/project-unload-lifecycle]") {
		t.Errorf("stdout = %q, complete project must see its unload source", stdout.String())
	}
	if !strings.Contains(stdout.String(), incompleteEntry+":1:1: [plugin/project-unload-lifecycle]") {
		t.Errorf("stdout = %q, incomplete project must remain isolated", stdout.String())
	}
}

func TestRunExplicitConfigurationOverridesDiscovery(t *testing.T) {
	parent := t.TempDir()
	child := filepath.Join(parent, "plugin")
	script := filepath.Join(child, "functions", "refresh")
	writeFile(t, filepath.Join(parent, "zsh-lint.parent.json"), `{
  "version": 2,
  "project": {
    "kind": "tool",
    "minimum_zsh": "5.8",
    "identifier": "zunit"
  },
  "sources": [
    {"root": ".", "profile": "sourced-library"}
  ]
}`)
	writeFile(t, filepath.Join(child, "zsh-lint.json"), cliConfig)
	writeFile(t, script, "builtin emulate -L zsh\n")

	var stdout, stderr bytes.Buffer
	if exit := run([]string{"--config", filepath.Join(parent, "zsh-lint.parent.json"), script}, &stdout, &stderr); exit != 0 {
		t.Fatalf("run() exit = %d, want 0; stdout = %q, stderr = %q", exit, stdout.String(), stderr.String())
	}
	if strings.Contains(stdout.String(), "[plugin/function-namespace]") {
		t.Errorf("stdout = %q, want explicit config to override discovered config", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Errorf("stderr = %q, want empty", stderr.String())
	}
}

func TestRunDiscoveryConfigurationErrorDoesNotBlockUnrelatedInputs(t *testing.T) {
	badRoot := t.TempDir()
	badScript := filepath.Join(badRoot, "script.zsh")
	writeFile(t, filepath.Join(badRoot, "zsh-lint.json"), `{}`)
	writeFile(t, badScript, "print ok\n")

	goodScript := filepath.Join(t.TempDir(), "standalone.zsh")
	writeFile(t, goodScript, "eval $value\n")

	var stdout, stderr bytes.Buffer
	if exit := run([]string{badScript, goodScript}, &stdout, &stderr); exit != 1 {
		t.Fatalf("run() exit = %d, want 1", exit)
	}
	if !strings.Contains(stdout.String(), goodScript+":1:1: [security/eval]") {
		t.Errorf("stdout = %q, want diagnostics for unaffected input", stdout.String())
	}
	if !strings.Contains(stderr.String(), `missing required field "version"`) {
		t.Errorf("stderr = %q, want discovered configuration error", stderr.String())
	}
}

func TestRunDiscoveryFallsBackWhenNoSourceMatches(t *testing.T) {
	root := t.TempDir()
	script := filepath.Join(root, "functions", "handler")
	writeFile(t, filepath.Join(root, "zsh-lint.json"), `{
  "version": 2,
  "project": {
    "kind": "tool",
    "minimum_zsh": "5.8",
    "identifier": "zunit"
  },
  "sources": [
    {"root": "tests", "profile": "test-fixture"}
  ]
}`)
	writeFile(t, script, "rehash\n")

	var stdout, stderr bytes.Buffer
	if exit := run([]string{script}, &stdout, &stderr); exit != 0 {
		t.Fatalf("run() exit = %d, want 0; stdout = %q, stderr = %q", exit, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), "[plugin/function-scoped-options]") {
		t.Errorf("stdout = %q, want fallback legacy-path diagnostic", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Errorf("stderr = %q, want empty", stderr.String())
	}
}

func TestRunJSONDoesNotCountConfigurationFailuresAsInspected(t *testing.T) {
	badRoot := t.TempDir()
	badScript := filepath.Join(badRoot, "bad.zsh")
	writeFile(t, filepath.Join(badRoot, "zsh-lint.json"), `{}`)
	writeFile(t, badScript, "print bad\n")
	goodScript := filepath.Join(t.TempDir(), "good.zsh")
	writeFile(t, goodScript, "print good\n")

	var stdout, stderr bytes.Buffer
	if exit := run([]string{"--format=json", badScript, goodScript}, &stdout, &stderr); exit != 1 {
		t.Fatalf("run() exit = %d, want 1", exit)
	}
	if !strings.Contains(stdout.String(), `"files":1`) {
		t.Errorf("stdout = %q, want one inspected file", stdout.String())
	}
}

func TestRunDirectoryInputDoesNotDiscoverConfiguration(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "zsh-lint.json"), `{}`)

	var stdout, stderr bytes.Buffer
	if exit := run([]string{root}, &stdout, &stderr); exit != 1 {
		t.Fatalf("run() exit = %d, want unreadable-input status 1", exit)
	}
	if strings.Contains(stderr.String(), "configuration:") {
		t.Errorf("stderr = %q, directory input must not discover configuration", stderr.String())
	}
}

func TestRunRejectsConfigurationBeforeAnalysis(t *testing.T) {
	root := t.TempDir()
	config := filepath.Join(root, "zsh-lint.json")
	outside := filepath.Join(t.TempDir(), "outside.zsh")
	writeFile(t, config, cliConfig)
	writeFile(t, outside, "eval $value\n")

	var stdout, stderr bytes.Buffer
	if exit := run([]string{"--format=json", "--config", config, outside}, &stdout, &stderr); exit != 2 {
		t.Fatalf("run() exit = %d, want 2", exit)
	}
	if stdout.Len() != 0 {
		t.Errorf("stdout = %q, want no diagnostics", stdout.String())
	}
	if !strings.Contains(stderr.String(), "outside configuration root") {
		t.Errorf("stderr = %q, want containment error", stderr.String())
	}
}

func TestRunRejectsMissingOrInvalidConfiguration(t *testing.T) {
	root := t.TempDir()
	script := filepath.Join(root, "script.zsh")
	invalid := filepath.Join(root, "invalid.json")
	writeFile(t, script, "eval $value\n")
	writeFile(t, invalid, `{}`)

	tests := []struct {
		name   string
		config string
		want   string
	}{
		{name: "missing", config: filepath.Join(root, "missing.json"), want: "configuration: open"},
		{name: "invalid", config: invalid, want: "missing required field"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			if exit := run([]string{"--format=json", "--config", test.config, script}, &stdout, &stderr); exit != 2 {
				t.Fatalf("run() exit = %d, want 2", exit)
			}
			if stdout.Len() != 0 {
				t.Errorf("stdout = %q, want no diagnostics", stdout.String())
			}
			if !strings.Contains(stderr.String(), test.want) {
				t.Errorf("stderr = %q, want substring %q", stderr.String(), test.want)
			}
		})
	}
}

func TestRunJSONWithConfiguration(t *testing.T) {
	root := t.TempDir()
	config := filepath.Join(root, "zsh-lint.json")
	script := filepath.Join(root, "script.zsh")
	writeFile(t, config, cliConfig)
	writeFile(t, script, "print ok\n")

	var stdout, stderr bytes.Buffer
	if exit := run([]string{"--format=json", "--config", config, script}, &stdout, &stderr); exit != 0 {
		t.Fatalf("run() exit = %d, want 0; stderr = %q", exit, stderr.String())
	}
	if !strings.Contains(stdout.String(), `"files":1`) {
		t.Errorf("stdout = %q, want JSON file count", stdout.String())
	}
}

func TestRunConfigurationActivatesProjectProfile(t *testing.T) {
	root := t.TempDir()
	config := filepath.Join(root, "zsh-lint.json")
	script := filepath.Join(root, "functions", "refresh")
	writeFile(t, config, cliConfig)
	writeFile(t, script, "builtin emulate -L zsh\n")

	var stdout, stderr bytes.Buffer
	if exit := run([]string{"--config", config, script}, &stdout, &stderr); exit != 0 {
		t.Fatalf("run() exit = %d, want 0 for hint-only finding; stderr = %q", exit, stderr.String())
	}
	if !strings.Contains(stdout.String(), "[plugin/function-namespace]") {
		t.Errorf("stdout = %q, want project-profile diagnostic", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Errorf("stderr = %q, want empty", stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	unconfigured := filepath.Join(t.TempDir(), "functions", "refresh")
	writeFile(t, unconfigured, "builtin emulate -L zsh\n")
	if exit := run([]string{unconfigured}, &stdout, &stderr); exit != 0 {
		t.Fatalf("unconfigured run() exit = %d, want 0; stderr = %q", exit, stderr.String())
	}
	if strings.Contains(stdout.String(), "[plugin/function-namespace]") {
		t.Errorf("unconfigured stdout = %q, want no project-profile diagnostic", stdout.String())
	}
}

func TestRunConfiguredProjectUnloadLifecycle(t *testing.T) {
	root := t.TempDir()
	config := filepath.Join(root, "zsh-lint.json")
	entry := filepath.Join(root, "example.plugin.zsh")
	unload := filepath.Join(root, "lib", "state.zsh")
	writeFile(t, config, cliConfig)
	writeFile(t, entry, "add-zsh-hook precmd _example_tick\n")
	writeFile(t, unload, "example_plugin_unload() { unfunction example_plugin_unload }\n")

	tests := []struct {
		name        string
		inputs      []string
		wantFinding bool
	}{
		{name: "missing unload function", inputs: []string{entry}, wantFinding: true},
		{name: "unload function in another source", inputs: []string{entry, unload}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			args := append([]string{"--config", config}, test.inputs...)
			var stdout, stderr bytes.Buffer
			if exit := run(args, &stdout, &stderr); exit != 0 {
				t.Fatalf("run() exit = %d, want 0 for hint-only project finding; stderr = %q", exit, stderr.String())
			}
			gotFinding := strings.Contains(stdout.String(), "[plugin/project-unload-lifecycle]")
			if gotFinding != test.wantFinding {
				t.Errorf("project unload finding = %v, want %v; stdout = %q", gotFinding, test.wantFinding, stdout.String())
			}
			if stderr.Len() != 0 {
				t.Errorf("stderr = %q, want empty", stderr.String())
			}
		})
	}
}

func TestRunConfiguredMetadataOverridesLegacyPathHeuristics(t *testing.T) {
	const toolConfig = `{
  "version": 2,
  "project": {
    "kind": "tool",
    "minimum_zsh": "5.8",
    "identifier": "zunit"
  },
  "sources": [
    {"root": "functions", "profile": "autoload-function"}
  ]
}`
	root := t.TempDir()
	config := filepath.Join(root, "zsh-lint.json")
	script := filepath.Join(root, "functions", "handler")
	writeFile(t, config, toolConfig)
	writeFile(t, script, "rehash\n")

	var stdout, stderr bytes.Buffer
	if exit := run([]string{"--config", config, script}, &stdout, &stderr); exit != 0 {
		t.Fatalf("configured run() exit = %d, want 0; stdout = %q, stderr = %q", exit, stdout.String(), stderr.String())
	}
	if strings.Contains(stdout.String(), "[plugin/") {
		t.Errorf("configured tool stdout = %q, want no plugin diagnostic", stdout.String())
	}

	stdout.Reset()
	stderr.Reset()
	unconfigured := filepath.Join(t.TempDir(), "functions", "handler")
	writeFile(t, unconfigured, "rehash\n")
	if exit := run([]string{unconfigured}, &stdout, &stderr); exit != 0 {
		t.Fatalf("unconfigured run() exit = %d, want 0; stderr = %q", exit, stderr.String())
	}
	if !strings.Contains(stdout.String(), "[plugin/function-scoped-options]") {
		t.Errorf("unconfigured stdout = %q, want legacy path diagnostic", stdout.String())
	}
}

func writeFile(t *testing.T, filename, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(filename), 0o755); err != nil {
		t.Fatalf("create parent directory: %v", err)
	}
	if err := os.WriteFile(filename, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", filename, err)
	}
}
