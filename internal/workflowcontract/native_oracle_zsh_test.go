package workflowcontract

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// nativeOracleZshVersion is the Zsh every native-oracle job judges with. It is
// the baseline the parser-gap workflow and the manual citations name, and the
// version the regression corpus record was measured against (#492). Before
// z-shell/.github#666 the shared action ignored its version input and
// installed Ubuntu's zsh 5.9, whose `-n` evaluates `$(< file)` in command
// arguments and so rejected five Completion files that 5.9.2 accepts.
const nativeOracleZshVersion = "5.9.2"

// Every setup-zsh step must request the pinned version explicitly: the
// action's default is the runner's package, which changes with the image.
func TestSetupZshStepsRequestTheNativeOracleVersion(t *testing.T) {
	dir := repositoryFilePath(t, ".github", "workflows")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read %s: %v", dir, err)
	}

	const uses = "uses: z-shell/.github/actions/setup-zsh@"
	steps := 0
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || (!strings.HasSuffix(name, ".yml") && !strings.HasSuffix(name, ".yaml")) {
			continue
		}
		contents, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		lines := strings.Split(string(contents), "\n")
		for i, line := range lines {
			at := strings.Index(line, uses)
			if at < 0 {
				continue
			}
			steps++
			indent := strings.Repeat(" ", at)
			want := []string{
				indent + "with:",
				indent + `  version: "` + nativeOracleZshVersion + `"`,
			}
			for j, expected := range want {
				if i+1+j >= len(lines) || lines[i+1+j] != expected {
					t.Errorf("%s:%d: setup-zsh must be followed by %q", name, i+1, strings.Join(want, "\n"))
					break
				}
			}
		}
	}

	// go-ci, zsh-n, discovery-survey and the two corpus-gate jobs. A lower
	// count means a step was renamed past the match and the check went vacuous.
	if steps < 5 {
		t.Fatalf("found %d setup-zsh step(s) under .github/workflows; want at least 5", steps)
	}
}
