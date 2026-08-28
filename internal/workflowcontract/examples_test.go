package workflowcontract

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/z-shell/zsh-lint/internal/analyzer"
	"github.com/z-shell/zsh-lint/internal/parse"
	"github.com/z-shell/zsh-lint/internal/projectconfig"
	"github.com/z-shell/zsh-lint/internal/rules"
)

func TestUserExamplesAreValidAndFindingFree(t *testing.T) {
	tests := []struct {
		name    string
		dir     string
		sources map[string]projectconfig.Profile
	}{
		{
			name: "standalone",
			dir:  "standalone",
			sources: map[string]projectconfig.Profile{
				"script.zsh": projectconfig.ProfileStandaloneExecutable,
			},
		},
		{
			name: "plugin",
			dir:  "plugin",
			sources: map[string]projectconfig.Profile{
				"example.plugin.zsh":        projectconfig.ProfileSourcedLibrary,
				"functions/example-run":     projectconfig.ProfileAutoloadFunction,
				"completions/_example":      projectconfig.ProfileAutoloadFunction,
				"tests/example-fixture.zsh": projectconfig.ProfileTestFixture,
			},
		},
	}

	activeRules, err := rules.ForProfile(rules.CurrentProjectProfile)
	if err != nil {
		t.Fatalf("load current project rule profile: %v", err)
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := repositoryFilePath(t, "examples", test.dir)
			config, err := projectconfig.Load(filepath.Join(root, "zsh-lint.json"))
			if err != nil {
				t.Fatalf("load example configuration: %v", err)
			}

			inputs := make([]analyzer.ProjectInput, 0, len(test.sources))
			for relative, wantProfile := range test.sources {
				path := filepath.Join(root, filepath.FromSlash(relative))
				context, err := config.Resolve(path)
				if err != nil {
					t.Fatalf("resolve %s: %v", relative, err)
				}
				if context.Profile != wantProfile {
					t.Errorf("resolve %s profile = %q, want %q", relative, context.Profile, wantProfile)
				}

				fileHandle, err := os.Open(path)
				if err != nil {
					t.Fatalf("open %s: %v", relative, err)
				}
				file, parseErr := parse.Parse(fileHandle, path)
				closeErr := fileHandle.Close()
				if parseErr != nil {
					t.Fatalf("parse %s: %v", relative, parseErr)
				}
				if closeErr != nil {
					t.Fatalf("close %s: %v", relative, closeErr)
				}
				inputs = append(inputs, analyzer.ProjectInput{File: file, Path: path, Source: context})
			}

			diagnostics := analyzer.New(activeRules...).AnalyzeProject(inputs)
			if len(diagnostics) != 0 {
				t.Fatalf("example diagnostics = %+v, want none", diagnostics)
			}
		})
	}
}
