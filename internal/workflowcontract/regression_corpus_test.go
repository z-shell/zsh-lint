package workflowcontract

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// The regression corpus holds sources with open parser gaps (#470): they
// cannot join the strict corpus, but a pull request must not make a file in
// them regress. Each manifest line is `<path> <repository> <sha> <root>...`.
func TestRegressionCorpusManifestPinsEverySource(t *testing.T) {
	manifest := readRepositoryFile(t, "docs", "project", "regression-corpus.txt")
	lines := strings.Split(strings.TrimSuffix(manifest, "\n"), "\n")
	want := []string{"F-Sy-H", "zsh"}
	if len(lines) != len(want) {
		t.Fatalf("regression-corpus.txt must list exactly %q; got %d lines", want, len(lines))
	}
	workflow := readRepositoryFile(t, ".github", "workflows", "corpus-gate.yml")
	for index, line := range lines {
		fields := strings.Fields(line)
		if len(fields) < 4 {
			t.Fatalf("regression-corpus.txt line %d must be `<path> <repository> <sha> <root>...`: %q", index+1, line)
		}
		path, repository, revision := fields[0], fields[1], fields[2]
		if path != want[index] {
			t.Errorf("regression-corpus.txt line %d is %q, want %q", index+1, path, want[index])
		}
		if !fullCommitSHA.MatchString(revision) {
			t.Errorf("regression-corpus.txt pins %s to %q, want a full 40-character commit SHA", path, revision)
		}
		for _, root := range fields[3:] {
			if filepath.IsAbs(root) || filepath.Clean(root) != root || strings.HasPrefix(root, "..") {
				t.Errorf("regression corpus root must be a contained clean relative path: %s/%s", path, root)
			}
		}
		checkout := "repository: " + repository + "\n" +
			"          ref: ${{ steps.revisions.outputs." + path + " }}\n" +
			"          path: corpus/" + path + "\n"
		if !strings.Contains(workflow, checkout) {
			t.Errorf("regression corpus job must check %s out at its pinned revision into corpus/%s", repository, path)
		}
	}
}

func TestRegressionCorpusJobComparesAgainstTheBase(t *testing.T) {
	workflow := readRepositoryFile(t, ".github", "workflows", "corpus-gate.yml")
	job := workflowBlock(t, workflow, "regression-corpus:", 2)
	for _, required := range []string{
		"if: github.event_name == 'pull_request'",
		"ref: ${{ github.event.pull_request.base.sha }}",
		`go build -o "$RUNNER_TEMP/survey-head" ./cmd/zsh-lint-survey`,
		`go build -o "$RUNNER_TEMP/survey-base" ./cmd/zsh-lint-survey`,
		"uses: z-shell/.github/actions/setup-zsh@",
		"run: bash zsh-lint/.github/scripts/regression-corpus.sh",
	} {
		if !strings.Contains(job, required) {
			t.Errorf("regression corpus job is missing %q", required)
		}
	}
	if strings.Contains(job, "continue-on-error") {
		t.Error("regression corpus job gates the pull request; it must not continue on error")
	}
}

// Runs the real script against fake survey binaries that print a fixed
// -compare report and exit with the given status.
func TestRegressionCorpusScriptFailsOnlyOnARegression(t *testing.T) {
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash is required for the workflow behavior test")
	}
	script := repositoryFilePath(t, ".github", "scripts", "regression-corpus.sh")

	tests := []struct {
		name       string
		report     string
		status     int
		manifest   string
		wantStatus int
		wantOutput string
	}{
		{name: "unchanged", report: "2 file(s) compared, 2 unchanged; known: 1 gap(s), 0 false accept(s)", status: 0, wantStatus: 0},
		{name: "fixed", report: "FIXED corpus/a/x/b.zsh\n\n2 file(s) compared, 1 unchanged, 1 fixed", status: 0, wantStatus: 0},
		{name: "regressed", report: "REGRESSED corpus/a/x/b.zsh\ncorpus/a/x/b.zsh:1:1: x\n\n2 file(s) compared, 1 unchanged, 1 regressed", status: 1, wantStatus: 1, wantOutput: "::error title=Regression corpus::a file regressed"},
		{name: "comparison failed", report: "compare: base survey: boom", status: 2, wantStatus: 2, wantOutput: "::error title=Regression corpus::the comparison could not run (status 2)"},
		{name: "missing root", manifest: "a o/a 0123456789012345678901234567890123456789 missing\n", wantStatus: 2, wantOutput: "root does not exist: corpus/a/missing"},
		{name: "malformed line", manifest: "a o/a\n", wantStatus: 2, wantOutput: "malformed manifest line for a"},
		{name: "empty root", manifest: "a o/a 0123456789012345678901234567890123456789 empty\n", wantStatus: 2, wantOutput: "::error title=Regression corpus::no files to compare"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			manifest := tt.manifest
			if manifest == "" {
				manifest = "a o/a 0123456789012345678901234567890123456789 x\n"
			}
			for name, contents := range map[string]string{
				"zsh-lint/docs/project/regression-corpus.txt": manifest,
				"corpus/a/x/a.zsh":                            "true\n",
				"corpus/a/x/b.zsh":                            "true\n",
				"corpus/a/empty/":                             "",
			} {
				if strings.HasSuffix(name, "/") {
					if err := os.MkdirAll(filepath.Join(dir, name), 0o700); err != nil {
						t.Fatal(err)
					}
					continue
				}
				path := filepath.Join(dir, name)
				if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			tmp := filepath.Join(dir, "tmp")
			if err := os.Mkdir(tmp, 0o700); err != nil {
				t.Fatal(err)
			}
			args := filepath.Join(tmp, "args")
			head := "#!/usr/bin/env bash\n" +
				"printf '%s\\n' \"$@\" > " + args + "\n" +
				"cat <<'EOF'\n" + tt.report + "\nEOF\n" +
				"exit " + strconv.Itoa(tt.status) + "\n"
			if err := os.WriteFile(filepath.Join(tmp, "survey-head"), []byte(head), 0o700); err != nil {
				t.Fatal(err)
			}
			summary := filepath.Join(dir, "summary.md")

			command := exec.Command(bash, "--noprofile", "--norc", script)
			command.Dir = dir
			command.Env = append(os.Environ(), "RUNNER_TEMP="+tmp, "GITHUB_STEP_SUMMARY="+summary)
			output, err := command.CombinedOutput()
			status := 0
			if exit, ok := err.(*exec.ExitError); ok {
				status = exit.ExitCode()
			} else if err != nil {
				t.Fatalf("run regression-corpus.sh: %v", err)
			}
			if status != tt.wantStatus {
				t.Fatalf("status = %d, want %d; output:\n%s", status, tt.wantStatus, output)
			}
			if tt.wantOutput != "" && !strings.Contains(string(output), tt.wantOutput) {
				t.Fatalf("output lacks %q:\n%s", tt.wantOutput, output)
			}
			if tt.manifest != "" {
				return
			}
			gotArgs, err := os.ReadFile(args)
			if err != nil {
				t.Fatalf("survey-head was not run: %v", err)
			}
			wantArgs := "-compare\n" + tmp + "/survey-base\n-native\ncorpus/a/x/a.zsh\ncorpus/a/x/b.zsh\n"
			if string(gotArgs) != wantArgs {
				t.Fatalf("survey-head arguments:\n%s\nwant:\n%s", gotArgs, wantArgs)
			}
			report, err := os.ReadFile(summary)
			if err != nil {
				t.Fatalf("read summary: %v", err)
			}
			if !strings.Contains(string(report), "2 file(s) compared against the base build") {
				t.Fatalf("summary lacks the file count:\n%s", report)
			}
		})
	}
}
