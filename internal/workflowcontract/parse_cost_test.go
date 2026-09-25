package workflowcontract

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// The Parse Cost workflow is observed, not gated (ADR-0024, #414): the job
// may never fail a run, and a parse-count rise is a notice, not an error.
func TestParseCostIsObservedNotGated(t *testing.T) {
	workflow := readRepositoryFile(t, ".github", "workflows", "parse-cost.yml")
	jobBlock := workflowBlock(t, workflow, "parse-cost:", 2)
	if values := directWorkflowMapping(jobBlock, 4)["continue-on-error"]; len(values) != 1 || values[0] != "true" {
		t.Fatalf("parse-cost job must set continue-on-error: true; got %q", values)
	}
	for _, step := range workflowJobSteps(t, workflow, "parse-cost") {
		uses := directWorkflowMapping(step, 8)["uses"]
		if len(uses) == 1 && strings.HasPrefix(uses[0], "actions/checkout@") {
			withBlock := workflowBlock(t, step, "with:", 8)
			if values := directWorkflowMapping(withBlock, 10)["persist-credentials"]; len(values) != 1 || values[0] != "false" {
				t.Fatalf("parse-cost checkout must disable persisted credentials; got %q", values)
			}
		}
	}
	if !strings.Contains(workflow, "run: bash zsh-lint/.github/scripts/parse-cost.sh") {
		t.Fatal("parse-cost job must run .github/scripts/parse-cost.sh")
	}
}

// Runs the real script against fake survey binaries: a rise above the
// threshold produces a notice naming the file and both counts, an unchanged
// count produces none, and the script exits 0 either way.
func TestParseCostScriptFlagsRisesAndNeverFails(t *testing.T) {
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash is required for the workflow behavior test")
	}
	script := repositoryFilePath(t, ".github", "scripts", "parse-cost.sh")

	fakeSurvey := func(dir, name string, ziParses int) string {
		path := filepath.Join(dir, name)
		body := "#!/usr/bin/env bash\n" +
			"for f in \"$@\"; do\n" +
			"  [[ $f == -* ]] && continue\n" +
			"  n=3; [[ $f == corpus/zi/zi.zsh ]] && n=" + strconv.Itoa(ziParses) + "\n" +
			"  echo \"TRACE $f parses=$n adapter-depth=1\" >&2\n" +
			"done\n" +
			"echo 'TRACE total files=0 parses=0 max-adapter-depth=0' >&2\n" +
			"exit 1\n"
		if err := os.WriteFile(path, []byte(body), 0o700); err != nil {
			t.Fatalf("write fake survey: %v", err)
		}
		return path
	}

	tests := []struct {
		name       string
		baseParses int
		wantNotice bool
	}{
		{name: "rise above threshold", baseParses: 100, wantNotice: true},
		{name: "rise within threshold", baseParses: 115, wantNotice: false},
		{name: "unchanged", baseParses: 120, wantNotice: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			for _, file := range []string{
				"corpus/zi/zi.zsh",
				"corpus/zi/lib/zsh/a.zsh",
				"zsh-lint/internal/survey/testdata/corpus/ok-a.zsh",
			} {
				if err := os.MkdirAll(filepath.Join(dir, filepath.Dir(file)), 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(dir, file), []byte("true\n"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			tmp := filepath.Join(dir, "tmp")
			if err := os.Mkdir(tmp, 0o700); err != nil {
				t.Fatal(err)
			}
			fakeSurvey(tmp, "survey-head", 120)
			fakeSurvey(tmp, "survey-base", tt.baseParses)
			summary := filepath.Join(dir, "summary.md")

			command := exec.Command(bash, "--noprofile", "--norc", script)
			command.Dir = dir
			command.Env = append(os.Environ(), "RUNNER_TEMP="+tmp, "GITHUB_STEP_SUMMARY="+summary, "FLAG_PERCENT=10")
			output, err := command.CombinedOutput()
			if err != nil {
				t.Fatalf("parse-cost.sh failed: %v\n%s", err, output)
			}
			notice := "::notice title=Parse cost::corpus/zi/zi.zsh: " + strconv.Itoa(tt.baseParses) + " -> 120 whole-source parses"
			if got := strings.Contains(string(output), notice); got != tt.wantNotice {
				t.Fatalf("notice present = %v, want %v; output:\n%s", got, tt.wantNotice, output)
			}
			report, err := os.ReadFile(summary)
			if err != nil {
				t.Fatalf("read summary: %v", err)
			}
			if !strings.Contains(string(report), "| `corpus/zi/zi.zsh` | 120 | 1 |") {
				t.Fatalf("summary lacks the head measurement for zi.zsh:\n%s", report)
			}
		})
	}
}
