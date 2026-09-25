package workflowcontract

import (
	"encoding/json"
	"path/filepath"
	"regexp"
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

func TestCorpusGateUsesReadOnlyPinnedCheckouts(t *testing.T) {
	workflow := readRepositoryFile(t, ".github", "workflows", "corpus-gate.yml")
	// The corpus gate holds no secrets, so the pin is checked for shape and
	// consistency rather than one literal commit; a Renovate bump then moves
	// every checkout together without touching this test (#91, #420).
	checkouts := pinnedCheckout.FindAllStringSubmatch(workflow, -1)
	if len(checkouts) != 14 {
		t.Fatalf("two corpus jobs must use fourteen checkout steps pinned to a full commit SHA with a version comment; got %d", len(checkouts))
	}
	for _, checkout := range checkouts[1:] {
		if checkout[1] != checkouts[0][1] {
			t.Fatalf("every corpus checkout must use the same pin; got %q and %q", checkouts[0][1], checkout[1])
		}
	}
	if got := strings.Count(workflow, "uses: actions/checkout@"); got != len(checkouts) {
		t.Fatalf("every corpus checkout must be pinned to a full commit SHA with a version comment; %d of %d are", len(checkouts), got)
	}
	if got := strings.Count(workflow, "persist-credentials: false"); got != 14 {
		t.Fatalf("every corpus checkout must disable persisted credentials; got %d", got)
	}
	if got := strings.Count(workflow, "- name: Resolve pinned corpus revisions\n        id: revisions"); got != 2 {
		t.Fatalf("both corpus jobs must resolve docs/project/corpus-revisions.txt before checking consumers out; got %d", got)
	}
	if got := strings.Count(workflow, `done < zsh-lint/docs/project/corpus-revisions.txt`); got != 2 {
		t.Fatalf("both corpus jobs must read the revision pins from the manifest; got %d", got)
	}

	for _, repository := range corpusRepositories {
		want := "repository: z-shell/" + repository + "\n" +
			"          ref: ${{ (github.event_name == 'schedule' || github.event_name == 'workflow_dispatch') && 'main' || steps.revisions.outputs." + repository + " }}\n" +
			"          path: corpus/" + repository + "\n" +
			"          persist-credentials: false"
		if got := strings.Count(workflow, want); got != 2 {
			t.Errorf("both corpus jobs must check out %s at its pinned revision (main only on schedule and dispatch) in its isolated path; got %d", repository, got)
		}
	}
}

var corpusRepositories = []string{"src", "zd", "zunit", "z-a-meta-plugins", "zsh-fancy-completions", "zsh-eza"}

var fullCommitSHA = regexp.MustCompile(`^[0-9a-f]{40}$`)

// pinnedCheckout matches a checkout step pinned to a full commit SHA with a
// version comment and captures the pin.
var pinnedCheckout = regexp.MustCompile(`uses: (actions/checkout@[0-9a-f]{40} # v[0-9][0-9.]*)\n`)

func TestCorpusRevisionsPinEveryRepository(t *testing.T) {
	manifest := readRepositoryFile(t, "docs", "project", "corpus-revisions.txt")
	lines := strings.Split(strings.TrimSuffix(manifest, "\n"), "\n")
	if len(lines) != len(corpusRepositories) {
		t.Fatalf("corpus-revisions.txt must pin exactly the %d corpus repositories; got %d lines", len(corpusRepositories), len(lines))
	}
	for index, line := range lines {
		fields := strings.Fields(line)
		if len(fields) != 2 {
			t.Fatalf("corpus-revisions.txt line %d must be `<repository> <sha>`: %q", index+1, line)
		}
		if fields[0] != corpusRepositories[index] {
			t.Errorf("corpus-revisions.txt line %d pins %q, want %q (same order as the corpus jobs)", index+1, fields[0], corpusRepositories[index])
		}
		if !fullCommitSHA.MatchString(fields[1]) {
			t.Errorf("corpus-revisions.txt pins %s to %q, want a full 40-character commit SHA", fields[0], fields[1])
		}
	}
}

func TestCorpusGateRunsTheFullReadinessContract(t *testing.T) {
	workflow := readRepositoryFile(t, ".github", "workflows", "corpus-gate.yml")
	if strings.Contains(workflow, `grep -n -F 'zsh-lint disable='`) {
		t.Fatal("corpus gate must not reject suppressions before the owning analysis profile evaluates them")
	}
	for _, required := range []string{
		"permissions:\n  contents: read",
		`mapfile -t roots < ../zsh-lint/docs/project/corpus-paths.txt`,
		`mapfile -d '' files < <(find "${roots[@]}" -type f -print0 | sort -z)`,
		`EXPECTED_CORPUS_FILES: "18"`,
		`zsh -f -n -- "$file"`,
		`"$RUNNER_TEMP/zsh-lint-survey" "${files[@]}"`,
		`"$RUNNER_TEMP/zsh-lint" --format=json --no-config "${files[@]}"`,
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

	for _, repository := range corpusRepositories {
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
	if expected == nil {
		t.Fatal("configured corpus classifications must be a JSON array; [] records a clean corpus")
	}
	for index, finding := range expected {
		if finding.Repository == "" || finding.Path == "" || finding.Rule == "" || finding.Severity == "" || finding.Classification == "" {
			t.Errorf("configured corpus classification %d has an empty required field: %+v", index, finding)
		}
	}
}
