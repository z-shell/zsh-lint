package workflowcontract

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestCorpusManifestIsExactAndContained(t *testing.T) {
	want := []string{
		"src/public/zsh/init.zsh",
		"zd/docker/utils.zsh",
		"zd/docker/zshrc",
		"zd/docker/zshenv",
		"zunit/build.zsh",
		"z-a-meta-plugins/z-a-meta-plugins.plugin.zsh",
		"z-a-meta-plugins/functions",
		"zsh-fancy-completions/zsh-fancy-completions.plugin.zsh",
		"zsh-fancy-completions/functions",
		"zsh-fancy-completions/lib",
		"zsh-eza/zsh-eza.plugin.zsh",
		"zsh-eza/functions",
	}

	manifest := readRepositoryFile(t, "docs", "project", "corpus-paths.txt")
	got := strings.Split(strings.TrimSuffix(manifest, "\n"), "\n")
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("corpus manifest differs from the reviewed inventory:\nwant: %q\ngot:  %q", want, got)
	}
	for _, entry := range got {
		if filepath.IsAbs(entry) || filepath.Clean(entry) != entry || strings.HasPrefix(entry, "..") {
			t.Fatalf("corpus manifest entry must be a contained clean relative path: %q", entry)
		}
	}
}

func TestCorpusGateUsesReadOnlyMainCheckouts(t *testing.T) {
	workflow := readRepositoryFile(t, ".github", "workflows", "corpus-gate.yml")
	if got := strings.Count(workflow, "uses: actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1 # v7.0.1"); got != 7 {
		t.Fatalf("corpus gate must use seven pinned checkout steps; got %d", got)
	}
	if got := strings.Count(workflow, "persist-credentials: false"); got != 7 {
		t.Fatalf("every corpus checkout must disable persisted credentials; got %d", got)
	}

	for _, repository := range []string{"src", "zd", "zunit", "z-a-meta-plugins", "zsh-fancy-completions", "zsh-eza"} {
		want := "repository: z-shell/" + repository + "\n" +
			"          ref: main\n" +
			"          path: corpus/" + repository + "\n" +
			"          persist-credentials: false"
		if !strings.Contains(workflow, want) {
			t.Errorf("corpus checkout for %s must use its main branch and isolated path", repository)
		}
	}
}

func TestCorpusGateRunsTheFullReadinessContract(t *testing.T) {
	workflow := readRepositoryFile(t, ".github", "workflows", "corpus-gate.yml")
	for _, required := range []string{
		"permissions:\n  contents: read",
		`mapfile -t roots < ../zsh-lint/docs/project/corpus-paths.txt`,
		`mapfile -d '' files < <(find "${roots[@]}" -type f -print0 | sort -z)`,
		`EXPECTED_CORPUS_FILES: "20"`,
		`grep -n -F 'zsh-lint disable=' "${files[@]}"`,
		`zsh -f -n -- "$file"`,
		`"$RUNNER_TEMP/zsh-lint-survey" "${files[@]}"`,
		`"$RUNNER_TEMP/zsh-lint" --format=json "${files[@]}"`,
		`.summary.errors == 0 and .summary.warnings == 0`,
	} {
		if !strings.Contains(workflow, required) {
			t.Errorf("corpus gate is missing required contract fragment %q", required)
		}
	}
}
