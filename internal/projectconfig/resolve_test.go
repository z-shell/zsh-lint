package projectconfig

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveUsesMostSpecificSourceRoot(t *testing.T) {
	root := t.TempDir()
	filename := filepath.Join(root, "zsh-lint.json")
	if err := os.WriteFile(filename, []byte(validConfig), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	config, err := Load(filename)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	tests := []struct {
		path        string
		wantRoot    string
		wantProfile Profile
		wantRole    SourceRole
	}{
		{path: "plugin.zsh", wantRoot: ".", wantProfile: ProfileSourcedLibrary},
		{path: "functions/example-run", wantRoot: "functions", wantProfile: ProfileAutoloadFunction},
		{path: "completions/_example", wantRoot: "completions", wantProfile: ProfileAutoloadFunction, wantRole: RoleCompletion},
		{path: "tests/fixture.zsh", wantRoot: "tests", wantProfile: ProfileTestFixture},
	}

	for _, test := range tests {
		t.Run(test.path, func(t *testing.T) {
			context, err := config.Resolve(filepath.Join(root, filepath.FromSlash(test.path)))
			if err != nil {
				t.Fatalf("Resolve() error = %v", err)
			}
			if context.SourceRoot != test.wantRoot || context.Profile != test.wantProfile || context.Role != test.wantRole {
				t.Errorf("Resolve() = root %q profile %q role %q, want %q/%q/%q",
					context.SourceRoot, context.Profile, context.Role,
					test.wantRoot, test.wantProfile, test.wantRole)
			}
			if context.ConfigVersion != CurrentVersion || context.ProjectKind != KindPlugin ||
				context.MinimumZsh != "5.8" || !context.Configured() {
				t.Errorf("project context is incomplete: %+v", context)
			}
			context.FunctionNamespaces[0] = "mutated"
		})
	}

	context, err := config.Resolve(filepath.Join(root, "plugin.zsh"))
	if err != nil {
		t.Fatalf("Resolve() after mutation error = %v", err)
	}
	if context.FunctionNamespaces[0] != "example" {
		t.Errorf("resolved namespace was mutated across calls: %q", context.FunctionNamespaces)
	}
}

func TestResolveRejectsOutsideAndUnmatchedInputs(t *testing.T) {
	root := t.TempDir()
	data := strings.Replace(validConfig,
		`{"root": ".", "profile": "sourced-library"},`, "", 1)
	filename := filepath.Join(root, "zsh-lint.json")
	if err := os.WriteFile(filename, []byte(data), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	config, err := Load(filename)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	outside := filepath.Join(filepath.Dir(root), filepath.Base(root)+"-sibling", "script.zsh")
	if _, err := config.Resolve(outside); err == nil || !strings.Contains(err.Error(), "outside configuration root") {
		t.Fatalf("Resolve(outside) error = %v, want containment error", err)
	}
	if _, err := config.Resolve(filepath.Join(root, "script.zsh")); err == nil || !strings.Contains(err.Error(), "matches no configured") {
		t.Fatalf("Resolve(unmatched) error = %v, want unmatched error", err)
	}
}
