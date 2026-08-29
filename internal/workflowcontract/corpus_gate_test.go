package workflowcontract

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/z-shell/zsh-lint/internal/projectconfig"
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
	if got := strings.Count(workflow, "uses: actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1 # v7.0.1"); got != 14 {
		t.Fatalf("two corpus jobs must use fourteen pinned checkout steps; got %d", got)
	}
	if got := strings.Count(workflow, "persist-credentials: false"); got != 14 {
		t.Fatalf("every corpus checkout must disable persisted credentials; got %d", got)
	}

	for _, repository := range []string{"src", "zd", "zunit", "z-a-meta-plugins", "zsh-fancy-completions", "zsh-eza"} {
		want := "repository: z-shell/" + repository + "\n" +
			"          ref: main\n" +
			"          path: corpus/" + repository + "\n" +
			"          persist-credentials: false"
		if got := strings.Count(workflow, want); got != 2 {
			t.Errorf("both corpus jobs must check out %s from main in its isolated path; got %d", repository, got)
		}
	}
}

func TestCorpusGateRunsTheFullReadinessContract(t *testing.T) {
	workflow := readRepositoryFile(t, ".github", "workflows", "corpus-gate.yml")
	for _, required := range []string{
		"permissions:\n  contents: read",
		`mapfile -t roots < ../zsh-lint/docs/project/corpus-paths.txt`,
		`mapfile -d '' files < <(find "${roots[@]}" -type f -print0 | sort -z)`,
		`EXPECTED_CORPUS_FILES: "18"`,
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

func TestConfiguredCorpusContract(t *testing.T) {
	workflow := readRepositoryFile(t, ".github", "workflows", "corpus-gate.yml")
	for _, required := range []string{
		"configured-corpus:",
		"name: Configured corpus",
		`config_source="../zsh-lint/docs/project/corpus-configs/$repository.json"`,
		`"$RUNNER_TEMP/zsh-lint" --format=json --config "$config" "${files[@]}"`,
		`configured-corpus-expected.json`,
		`if [[ $analyzer_status -gt 1 ]]`,
		`.summary.errors == 0`,
		`Every configured corpus finding needs a non-empty classification.`,
		`Configured corpus diagnostics changed; review and classify every difference.`,
	} {
		if !strings.Contains(workflow, required) {
			t.Errorf("configured corpus gate is missing required contract fragment %q", required)
		}
	}

	repositories := []string{"src", "zd", "zunit", "z-a-meta-plugins", "zsh-fancy-completions", "zsh-eza"}
	for _, repository := range repositories {
		path := repositoryFilePath(t, "docs", "project", "corpus-configs", repository+".json")
		config, err := projectconfig.Load(path)
		if err != nil {
			t.Errorf("configured corpus fixture %s is invalid: %v", repository, err)
			continue
		}
		if config.Version != projectconfig.CurrentVersion {
			t.Errorf("configured corpus fixture %s uses schema %d, want %d", repository, config.Version, projectconfig.CurrentVersion)
		}
	}

	var expected []struct {
		Repository     string `json:"repository"`
		Path           string `json:"path"`
		Rule           string `json:"rule"`
		Severity       string `json:"severity"`
		Classification string `json:"classification"`
	}
	if err := json.Unmarshal([]byte(readRepositoryFile(t, "docs", "project", "configured-corpus-expected.json")), &expected); err != nil {
		t.Fatalf("decode configured corpus classifications: %v", err)
	}
	if len(expected) == 0 {
		t.Fatal("configured corpus classifications must not be empty")
	}
	for index, finding := range expected {
		if finding.Repository == "" || finding.Path == "" || finding.Rule == "" || finding.Severity == "" || finding.Classification == "" {
			t.Errorf("configured corpus classification %d has an empty required field: %+v", index, finding)
		}
	}
}
