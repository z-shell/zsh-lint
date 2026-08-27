package projectconfig

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const validConfig = `{
  "version": 1,
  "project": {
    "kind": "plugin",
    "minimum_zsh": "5.8",
    "function_namespaces": ["example"]
  },
  "sources": [
    {"root": ".", "profile": "sourced-library"},
    {"root": "functions", "profile": "autoload-function"},
    {"root": "completions", "profile": "autoload-function", "role": "completion"},
    {"root": "tests", "profile": "test-fixture"}
  ]
}`

func TestLoadValidConfiguration(t *testing.T) {
	filename := writeConfig(t, []byte(validConfig))
	config, err := Load(filename)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if config.Version != CurrentVersion {
		t.Errorf("Version = %d, want %d", config.Version, CurrentVersion)
	}
	if config.Project.Kind != KindPlugin {
		t.Errorf("Project.Kind = %q, want %q", config.Project.Kind, KindPlugin)
	}
	if config.Project.MinimumZsh != "5.8" {
		t.Errorf("Project.MinimumZsh = %q, want 5.8", config.Project.MinimumZsh)
	}
	if got := config.Project.FunctionNamespaces; len(got) != 1 || got[0] != "example" {
		t.Errorf("Project.FunctionNamespaces = %q, want [example]", got)
	}
	if len(config.Sources) != 4 {
		t.Fatalf("Sources count = %d, want 4", len(config.Sources))
	}
	if config.Sources[2].Role != RoleCompletion {
		t.Errorf("completion role = %q, want %q", config.Sources[2].Role, RoleCompletion)
	}
}

func TestLoadRejectsInvalidConfiguration(t *testing.T) {
	tests := []struct {
		name string
		data string
		want string
	}{
		{name: "unknown top-level field", data: strings.Replace(validConfig, `"version": 1,`, `"version": 1, "extra": true,`, 1), want: `unknown field "extra"`},
		{name: "case-mismatched field", data: strings.Replace(validConfig, `"version"`, `"Version"`, 1), want: `unknown field "Version"`},
		{name: "duplicate nested field", data: strings.Replace(validConfig, `"kind": "plugin",`, `"kind": "plugin", "kind": "theme",`, 1), want: `duplicate field "kind"`},
		{name: "trailing document", data: validConfig + ` {}`, want: "unexpected value after top-level document"},
		{name: "unsupported version", data: strings.Replace(validConfig, `"version": 1`, `"version": 2`, 1), want: "unsupported schema version 2"},
		{name: "unknown kind", data: strings.Replace(validConfig, `"kind": "plugin"`, `"kind": "bundle"`, 1), want: `unknown project kind "bundle"`},
		{name: "invalid zsh version", data: strings.Replace(validConfig, `"minimum_zsh": "5.8"`, `"minimum_zsh": "05.8"`, 1), want: "leading zero"},
		{name: "null namespaces", data: strings.Replace(validConfig, `["example"]`, `null`, 1), want: "function_namespaces: must be an array"},
		{name: "duplicate namespace", data: strings.Replace(validConfig, `["example"]`, `["example", "example"]`, 1), want: "duplicate namespace"},
		{name: "whitespace namespace", data: strings.Replace(validConfig, `["example"]`, `[" example"]`, 1), want: "leading or trailing whitespace"},
		{name: "empty sources", data: replaceSources(validConfig, `[]`), want: "must contain at least one source root"},
		{name: "absolute source root", data: strings.Replace(validConfig, `"root": "."`, `"root": "/tmp"`, 1), want: "must be relative"},
		{name: "drive-prefixed source root", data: strings.Replace(validConfig, `"root": "."`, `"root": "C:/tmp"`, 1), want: "Windows drive prefix"},
		{name: "escaping source root", data: strings.Replace(validConfig, `"root": "."`, `"root": "../outside"`, 1), want: "must not escape"},
		{name: "dirty source root", data: strings.Replace(validConfig, `"root": "functions"`, `"root": "src/../functions"`, 1), want: "must be clean"},
		{name: "backslash source root", data: strings.Replace(validConfig, `"root": "functions"`, `"root": "functions\\\\nested"`, 1), want: "must use forward slashes"},
		{name: "duplicate source root", data: strings.Replace(validConfig, `"root": "functions"`, `"root": "."`, 1), want: "duplicate source root"},
		{name: "unknown profile", data: strings.Replace(validConfig, `"profile": "sourced-library"`, `"profile": "library"`, 1), want: `unknown execution profile "library"`},
		{name: "unknown role", data: strings.Replace(validConfig, `"role": "completion"`, `"role": "generated"`, 1), want: `unknown source role "generated"`},
		{name: "completion role mismatch", data: strings.Replace(validConfig, `"profile": "autoload-function", "role": "completion"`, `"profile": "sourced-library", "role": "completion"`, 1), want: "completion requires profile"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := Load(writeConfig(t, []byte(test.data)))
			if err == nil {
				t.Fatal("Load() succeeded, want error")
			}
			if !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Load() error = %q, want substring %q", err, test.want)
			}
		})
	}
}

func TestLoadRejectsInvalidUTF8(t *testing.T) {
	data := append([]byte(validConfig), 0xff)
	_, err := Load(writeConfig(t, data))
	if err == nil || !strings.Contains(err.Error(), "not valid UTF-8") {
		t.Fatalf("Load() error = %v, want invalid UTF-8 error", err)
	}
}

func TestLoadRejectsOversizedConfiguration(t *testing.T) {
	_, err := Load(writeConfig(t, make([]byte, maxConfigBytes+1)))
	if err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("Load() error = %v, want size limit error", err)
	}
}

func writeConfig(t *testing.T, data []byte) string {
	t.Helper()
	filename := filepath.Join(t.TempDir(), "zsh-lint.json")
	if err := os.WriteFile(filename, data, 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return filename
}

func replaceSources(config, replacement string) string {
	start := strings.Index(config, `"sources": [`)
	if start < 0 {
		return config
	}
	return config[:start] + `"sources": ` + replacement + "\n}"
}
