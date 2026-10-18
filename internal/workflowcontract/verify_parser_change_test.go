package workflowcontract

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// verify-parser-change.sh must refuse to measure against a stale or foreign
// base, the failure #545 was filed for: a measurement once ran 37 commits
// behind origin/main. Runs the real script's --check-base step, which stops
// before building anything, in throwaway repositories.
func TestVerifyParserChangeRefusesAStaleBase(t *testing.T) {
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash is required for the script behavior test")
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is required for the script behavior test")
	}
	script := repositoryFilePath(t, ".github", "scripts", "verify-parser-change.sh")

	dir := t.TempDir()
	env := append(os.Environ(),
		"GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1",
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.invalid",
		"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.invalid")
	git := func(dir string, args ...string) string {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = env
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
		}
		return strings.TrimSpace(string(out))
	}
	commit := func(dir, message string) string {
		t.Helper()
		git(dir, "commit", "-q", "--allow-empty", "-m", message)
		return git(dir, "rev-parse", "HEAD")
	}

	// upstream plays origin; work is the contributor's clone.
	upstream := filepath.Join(dir, "upstream")
	git(dir, "init", "-q", "-b", "main", upstream)
	first := commit(upstream, "first")
	work := filepath.Join(dir, "work")
	git(dir, "clone", "-q", upstream, work)
	git(work, "switch", "-q", "-c", "feature")
	feature := commit(work, "feature")

	checkIn := func(dir string, args ...string) (int, string) {
		t.Helper()
		cmd := exec.Command(bash, append([]string{"--noprofile", "--norc", script, "--check-base"}, args...)...)
		cmd.Dir = dir
		cmd.Env = env
		out, err := cmd.CombinedOutput()
		status := 0
		if exit, ok := err.(*exec.ExitError); ok {
			status = exit.ExitCode()
		} else if err != nil {
			t.Fatalf("run verify-parser-change.sh: %v", err)
		}
		return status, string(out)
	}
	check := func(args ...string) (int, string) { t.Helper(); return checkIn(work, args...) }

	status, out := check()
	if status != 0 || !strings.Contains(out, "verify: base      "+first+" (origin/main)") ||
		!strings.Contains(out, "verify: candidate "+feature+" (HEAD)") {
		t.Fatalf("current base: status %d, output:\n%s", status, out)
	}

	// origin moves on and the clone has not fetched: refuse.
	moved := commit(upstream, "moved")
	status, out = check()
	if status != 2 || !strings.Contains(out, "origin/main is "+first[:12]+" but origin's main is "+moved[:12]) {
		t.Fatalf("stale origin/main: status %d, output:\n%s", status, out)
	}

	// Fetched but not rebased: HEAD lacks the base.
	git(work, "fetch", "-q", "origin")
	status, out = check()
	if status != 2 || !strings.Contains(out, "HEAD "+feature[:12]+" does not contain the base "+moved[:12]) {
		t.Fatalf("unrebased HEAD: status %d, output:\n%s", status, out)
	}

	// A named base skips the remote check but not containment.
	status, out = check(first)
	if status != 0 || !strings.Contains(out, "verify: base      "+first+" ("+first+")") {
		t.Fatalf("named base: status %d, output:\n%s", status, out)
	}
	status, out = check(moved)
	if status != 2 || !strings.Contains(out, "does not contain the base") {
		t.Fatalf("named base HEAD lacks: status %d, output:\n%s", status, out)
	}
	status, out = check("no-such-revision")
	if status != 2 || !strings.Contains(out, "unknown base revision: no-such-revision") {
		t.Fatalf("unknown base: status %d, output:\n%s", status, out)
	}

	// A named candidate is checked the same way, and uncommitted work is
	// reported as part of the candidate.
	status, out = check("--candidate", moved, first)
	if status != 0 || !strings.Contains(out, "verify: candidate "+moved+" (candidate "+moved+")") {
		t.Fatalf("named candidate: status %d, output:\n%s", status, out)
	}
	status, out = check("--candidate", first, moved)
	if status != 2 || !strings.Contains(out, "the candidate "+first[:12]+" does not contain the base") {
		t.Fatalf("candidate without the base: status %d, output:\n%s", status, out)
	}
	if err := os.WriteFile(filepath.Join(work, "tracked"), []byte("a\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	git(work, "add", "tracked")
	status, out = check(first)
	if status != 0 || !strings.Contains(out, "(HEAD plus uncommitted changes)") {
		t.Fatalf("uncommitted candidate: status %d, output:\n%s", status, out)
	}

	// A path argument is relative to the caller's directory, not the
	// repository root the script moves to.
	sub := filepath.Join(work, "sub")
	if err := os.MkdirAll(sub, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, "rows.txt"), []byte("print a\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, option := range []string{"--rows", "--list"} {
		status, out = checkIn(sub, option, "rows.txt", first)
		if status != 0 {
			t.Fatalf("relative %s from a subdirectory: status %d, output:\n%s", option, status, out)
		}
	}
	status, out = checkIn(work, "--rows", "rows.txt", first)
	if status != 2 || !strings.Contains(out, "cannot read rows.txt") {
		t.Fatalf("missing --rows: status %d, output:\n%s", status, out)
	}
	status, out = checkIn(work, "--root", "missing", first)
	if status != 2 || !strings.Contains(out, "not a directory: missing") {
		t.Fatalf("missing --root: status %d, output:\n%s", status, out)
	}
	status, out = checkIn(work, "--rows")
	if status != 2 || !strings.Contains(out, "--rows needs an argument") {
		t.Fatalf("--rows without a file: status %d, output:\n%s", status, out)
	}
}

// Exercise orchestration with cheap stand-ins for builds and mutation, keeping
// the real Git snapshots, option handling, evidence and exit classifications.
func TestVerifyParserChangeMutationPolicy(t *testing.T) {
	for _, tool := range []string{"bash", "git", "zsh"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("%s is required for the script behavior test", tool)
		}
	}
	script := repositoryFilePath(t, ".github", "scripts", "verify-parser-change.sh")
	dir := t.TempDir()
	repo, bin := filepath.Join(dir, "repo"), filepath.Join(dir, "bin")
	write := func(name, contents string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(name), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(name, []byte(contents), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join(bin, "go"), `#!/usr/bin/env bash
set -eu
printf '%s\n' "$*" >> "$PILOT_GO_CALLS"
if [[ $* == 'test -tags=integration ./internal/workflowcontract/' && ${PILOT_INTEGRATION_FAIL:-0} == 1 ]]; then
  exit 1
fi
case $1 in
version) echo 'go version go1.27.0 test'; exit 0 ;;
list) echo '1.27.0'; exit 0 ;;
esac
while (($# > 0)); do
  if [[ $1 == -o ]]; then
    printf '#!/usr/bin/env bash\nexit 0\n' > "$2"
    chmod +x "$2"
    exit 0
  fi
  shift
done
`)
	write(filepath.Join(bin, "golangci-lint"), "#!/usr/bin/env bash\nexit 0\n")
	write(filepath.Join(repo, ".github", "scripts", "mutation.sh"), `#!/usr/bin/env bash
printf '%s\n' "$@" > "$PILOT_MUTATION_ARGS"
if [[ ${PILOT_MUTATION_LOG:-} == unstructured ]]; then
  echo 'mktemp: cannot create temporary file' >&2
  exit "$PILOT_MUTATION_EXIT"
fi
echo 'KILLED 2'
if [[ $PILOT_MUTATION_EXIT == 1 ]]; then echo 'LIVED 1'; else echo 'LIVED 0'; fi
echo 'TIMED OUT 0'
if [[ ${PILOT_CHANGE_SOURCE:-} == 1 ]]; then
  echo changed > new-source.go
fi
exit "$PILOT_MUTATION_EXIT"
`)
	write(filepath.Join(repo, "internal", "survey", "testdata", "corpus", "ok-sample.zsh"), "print sample\n")
	if err := os.MkdirAll(filepath.Join(repo, "third_party", "mvdan-sh"), 0o700); err != nil {
		t.Fatal(err)
	}
	env := append(os.Environ(), "PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"),
		"GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1",
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.invalid",
		"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.invalid")
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir, cmd.Env = repo, env
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	git("init", "-q", "-b", "main")
	git("add", ".")
	git("commit", "-q", "-m", "baseline")
	for _, tc := range []struct {
		name       string
		options    []string
		mutation   string
		wantStatus int
		change     bool
	}{
		{"sampled-survivors", nil, "1", 3, false},
		{"sampled-incomplete", nil, "3", 3, false},
		{"setup-failure", nil, "2", 1, false},
		{"integration-failure", nil, "0", 1, false},
		{"unstructured-setup-failure", nil, "2", 1, false},
		{"full-survivors", []string{"--full-mutation"}, "1", 1, false},
		{"full-clean", []string{"--full-mutation"}, "0", 0, false},
		{"skip", []string{"--skip-mutation"}, "1", 0, false},
		{"conflicting-options", []string{"--skip-mutation", "--full-mutation"}, "0", 2, false},
		{"existing-output", nil, "0", 2, false},
		{"output-in-checkout", nil, "0", 2, false},
		{"relative-output", nil, "0", 0, false},
		{"changed-source", nil, "0", 1, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			output, argsFile := filepath.Join(dir, tc.name), filepath.Join(dir, tc.name+".args")
			if tc.name == "existing-output" {
				write(filepath.Join(output, "preserve"), "existing evidence")
			}
			if tc.name == "output-in-checkout" {
				output = filepath.Join(repo, "new-output")
			}
			caller, outputArg := repo, output
			if tc.name == "relative-output" {
				// Resolve output from the caller, not the repository root.
				caller = filepath.Join(repo, "internal", "workflowcontract")
				output = filepath.Join(dir, "evidence", tc.name)
				for _, parent := range []string{caller, filepath.Dir(output)} {
					if err := os.MkdirAll(parent, 0o700); err != nil {
						t.Fatal(err)
					}
				}
				var err error
				outputArg, err = filepath.Rel(caller, output)
				if err != nil {
					t.Fatal(err)
				}
			}
			args := append([]string{script, "--output", outputArg}, tc.options...)
			cmd := exec.Command("bash", append(args, "HEAD")...)
			cmd.Dir = caller
			cmd.Env = append(env, "PILOT_MUTATION_ARGS="+argsFile, "PILOT_MUTATION_EXIT="+tc.mutation)
			goCalls := filepath.Join(dir, tc.name+".go-calls")
			cmd.Env = append(cmd.Env, "PILOT_GO_CALLS="+goCalls)
			if tc.name == "integration-failure" {
				cmd.Env = append(cmd.Env, "PILOT_INTEGRATION_FAIL=1")
			}
			if tc.name == "unstructured-setup-failure" {
				cmd.Env = append(cmd.Env, "PILOT_MUTATION_LOG=unstructured")
			}
			if tc.change {
				cmd.Env = append(cmd.Env, "PILOT_CHANGE_SOURCE=1")
			}
			out, err := cmd.CombinedOutput()
			status := 0
			if exit, ok := err.(*exec.ExitError); ok {
				status = exit.ExitCode()
			} else if err != nil {
				t.Fatal(err)
			}
			if status != tc.wantStatus {
				t.Fatalf("status %d, want %d:\n%s", status, tc.wantStatus, out)
			}
			if status == 2 {
				if tc.name == "existing-output" {
					got, err := os.ReadFile(filepath.Join(output, "preserve"))
					if err != nil || string(got) != "existing evidence" {
						t.Fatalf("existing evidence changed: %v, %q", err, got)
					}
				}
				if tc.name == "output-in-checkout" {
					if _, err := os.Stat(output); !os.IsNotExist(err) {
						t.Fatalf("created output in checkout: %v", err)
					}
				}
				return
			}
			summary, err := os.ReadFile(filepath.Join(output, "summary.md"))
			if err != nil {
				t.Fatal(err)
			}
			calls, err := os.ReadFile(goCalls)
			if err != nil || !strings.Contains(string(calls), "test -tags=integration ./internal/workflowcontract/\n") {
				t.Fatalf("verification must run the integration lane: %v\n%s", err, calls)
			}
			if !strings.Contains(string(calls), "vet -tags=integration ./...\n") {
				t.Fatalf("verification must vet the integration tests:\n%s", calls)
			}
			if tc.name == "integration-failure" {
				if _, err := os.Stat(argsFile); !os.IsNotExist(err) ||
					!strings.Contains(string(summary), "| integration-test | FAIL |") ||
					!strings.Contains(string(summary), "| mutation | skipped |") {
					t.Fatalf("integration failure must fail verification and skip mutation: %v\n%s", err, summary)
				}
				return
			}
			if tc.name == "unstructured-setup-failure" {
				mutationLog, err := os.ReadFile(filepath.Join(output, "mutation.log"))
				if err != nil || !strings.Contains(string(mutationLog), "mktemp: cannot create temporary file") ||
					!strings.Contains(string(summary), "| mutation | FAIL |") ||
					strings.Contains(string(out), "cannot write verification summary") {
					t.Fatalf("setup failure evidence lost: %v\n%s\n%s", err, mutationLog, summary)
				}
			}
			for _, text := range []string{"verification source:", "| Check | Result | Seconds |", "zsh"} {
				if !strings.Contains(string(summary), text) {
					t.Errorf("summary lacks %q", text)
				}
			}
			mutationArgs, err := os.ReadFile(argsFile)
			if tc.name == "skip" {
				if !os.IsNotExist(err) || !strings.Contains(string(summary), "| mutation | skipped |") {
					t.Fatalf("skipped mutation ran: %v\n%s", err, summary)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			sampled := strings.HasPrefix(string(mutationArgs), "--sample\n30\n--seed\n1\n")
			full := strings.HasPrefix(tc.name, "full-")
			if sampled == full {
				t.Fatalf("wrong mutation arguments: %s", mutationArgs)
			}
			if tc.change && !strings.Contains(string(summary), "| source | FAIL |") {
				t.Fatalf("changed source accepted:\n%s", summary)
			}
		})
	}
}
