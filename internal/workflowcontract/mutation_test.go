//go:build integration

package workflowcontract

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// Exercise the contributor command against a separate fork module and its
// root consumer, rather than assuming a coverage block exists for the line.
func TestMutationRunsForkAndRootConsumers(t *testing.T) {
	dir, run, write := mutationFixture(t)
	write("third_party/mvdan-sh/syntax/guard.go", "package syntax\nfunc Accept(n int) bool { return n <= 2 }\n")
	// Only the root consumer detects the mutant. Passing fork tests alone
	// must never be enough to mark this mutation as survived.
	write("third_party/mvdan-sh/syntax/guard_test.go", "package syntax\nimport \"testing\"\nfunc TestAccept(t *testing.T) {}\n")
	before, err := os.ReadFile(filepath.Join(dir, "third_party/mvdan-sh/syntax/guard.go"))
	if err != nil {
		t.Fatal(err)
	}
	status, out := run()
	if status != 0 && status != 1 {
		t.Fatalf("mutation run: status %d\n%s", status, out)
	}
	if !strings.Contains(out, "KILLED third_party/mvdan-sh/syntax/guard.go:2:") {
		t.Fatalf("fork change must produce a real mutant verdict:\n%s", out)
	}
	if !strings.Contains(out, "./syntax/") || !strings.Contains(out, "./internal/parse/ ./internal/survey/") {
		t.Fatalf("fork mutants must exercise fork tests and root consumers:\n%s", out)
	}
	after, err := os.ReadFile(filepath.Join(dir, "third_party/mvdan-sh/syntax/guard.go"))
	if err != nil || string(before) != string(after) {
		t.Fatalf("mutation changed the contributor's source: %v", err)
	}
}

func TestMutationFailfastCanBeDisabled(t *testing.T) {
	goBinary, err := exec.LookPath("go")
	if err != nil {
		t.Fatal(err)
	}
	bin := t.TempDir()
	log := filepath.Join(bin, "calls")
	quotedGo := "'" + strings.ReplaceAll(goBinary, "'", "'\"'\"'") + "'"
	body := fmt.Sprintf("#!/bin/sh\nif [ \"$1\" = build ]; then exec %s \"$@\"; fi\nprintf '%%s\\n' \"$*\" >> \"$MUTATION_CALLS\"\nexit 0\n", quotedGo)
	if err := os.WriteFile(filepath.Join(bin, "go"), []byte(body), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("MUTATION_CALLS", log)
	_, run, write := mutationFixture(t)
	write("third_party/mvdan-sh/syntax/guard.go", "package syntax\nfunc Accept(n int) bool { return true }\n")
	status, out := run("--failfast")
	if status != 1 {
		t.Fatalf("failfast run: status %d\n%s", status, out)
	}
	calls, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	for _, call := range strings.Split(strings.TrimSpace(string(calls)), "\n") {
		if !strings.Contains(" "+call+" ", " -failfast ") {
			t.Fatalf("baseline and mutant commands must use failfast: %s", call)
		}
	}
	if err := os.WriteFile(log, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	status, out = run("--failfast=false")
	if status != 1 {
		t.Fatalf("disabled failfast run: status %d\n%s", status, out)
	}
	calls, err = os.ReadFile(log)
	if err != nil || strings.Contains(string(calls), "-failfast") {
		t.Fatalf("disabled failfast must restore the full-test commands: %v\n%s", err, calls)
	}
}

func TestMutationRunsCaseGuardWithoutCoverage(t *testing.T) {
	_, run, write := mutationFixture(t)
	write("internal/parse/guard.go", "package parse\nfunc Accept(n int) bool { switch { case n <= 2: return true; default: return false } }\n")
	status, out := run()
	if (status != 0 && status != 1) || !strings.Contains(out, "KILLED internal/parse/guard.go:2:") {
		t.Fatalf("case guard must produce a real mutant verdict: status %d\n%s", status, out)
	}
}

func TestMutationIncludesUntrackedSource(t *testing.T) {
	_, run, write := mutationFixture(t)
	write("internal/parse/new.go", "package parse\nfunc Extra(n int) bool { return n <= 2 }\n")
	status, out := run()
	if status != 1 || !strings.Contains(out, "LIVED internal/parse/new.go:") {
		t.Fatalf("new source must not read as clean before staging: status %d\n%s", status, out)
	}
}

func TestMutationNegatesBooleanCaseGuard(t *testing.T) {
	_, run, write := mutationFixture(t)
	write("internal/parse/guard.go", "package parse\nfunc Accept(n int) bool {\nmatched := n <= 2\nswitch {\ncase matched: return true\ndefault: return false\n}\n}\n")
	_, out := run()
	if !strings.Contains(out, `"matched" -> "!(matched)"`) {
		t.Fatalf("boolean case guard was not negated:\n%s", out)
	}
}

func TestMutationNegatesBooleanLoopGuard(t *testing.T) {
	_, run, write := mutationFixture(t)
	write("internal/parse/guard.go", "package parse\nfunc Accept(n int) bool {\nmatched := n <= 2\nfor matched { return true }\nreturn false\n}\n")
	_, out := run()
	if !strings.Contains(out, `"matched" -> "!(matched)"`) {
		t.Fatalf("boolean loop guard was not negated:\n%s", out)
	}
}

func TestMutationExtensionSpecsAreValidated(t *testing.T) {
	_, run, write := mutationFixture(t)
	write("internal/parse/guard.go", "package parse\nfunc Accept(n int) bool { return n <= 2 }\n")
	write("mutants.json", `[{"file":"internal/parse/guard.go","line":2,"old":"n <= 2","new":"false"}]`)
	status, out := run("--spec", "mutants.json")
	if (status != 0 && status != 1) || !strings.Contains(out, `"n <= 2" -> "false"`) {
		t.Fatalf("extension mutant not exercised: status %d\n%s", status, out)
	}
	write("mutants.json", `[{"file":"../outside.go","line":2,"old":"n <= 2","new":"false"}]`)
	status, out = run("--spec", "mutants.json")
	if status != 2 || !strings.Contains(out, "changed source line") {
		t.Fatalf("spec must stay on changed repository source: status %d\n%s", status, out)
	}
}

func TestMutationSpecOnlyDoesNotRepeatGeneratedCampaign(t *testing.T) {
	_, run, write := mutationFixture(t)
	write("internal/parse/guard.go", "package parse\nfunc Accept(n int) bool { return n <= 2 }\n")
	write("internal/parse/extra.go", "package parse\nfunc Extra(n int) bool { return Accept(n) }\n")
	write("mutants.json", `[{"file":"internal/parse/guard.go","line":2,"old":"n <= 2","new":"false"}]`)
	status, out := run("--spec-only", "--spec", "mutants.json")
	if status != 0 || !strings.Contains(out, "1 killed, 0 lived, 0 timed out, 0 invalid") || strings.Contains(out, "LIVED ") || strings.Contains(out, "UNSUPPORTED ") {
		t.Fatalf("explicit-only run must neither repeat defaults nor assess unselected files: status %d\n%s", status, out)
	}
	if !strings.Contains(out, "spec-only") {
		t.Fatalf("explicit-only scope must remain visible:\n%s", out)
	}
	status, out = run("--spec-only")
	if status != 2 {
		t.Fatalf("spec-only without a spec must refuse: status %d\n%s", status, out)
	}
	write("mutants.json", "[]")
	status, out = run("--spec-only", "--spec", "mutants.json")
	if status != 2 {
		t.Fatalf("empty explicit-only run must not pass vacuously: status %d\n%s", status, out)
	}
}

func TestMutationSpecCoversOtherwiseUnsupportedChange(t *testing.T) {
	_, run, write := mutationFixture(t)
	write("internal/parse/guard.go", "package parse\nimport \"example.invalid/fork/syntax\"\nfunc Accept(n int) bool { return syntax.Accept(n) /* changed */ }\n")
	write("mutants.json", `[{"file":"internal/parse/guard.go","line":3,"old":"syntax.Accept(n)","new":"syntax.Accept(n) && false"}]`)
	status, out := run("--spec", "mutants.json")
	if status != 0 || !strings.Contains(out, "KILLED ") || strings.Contains(out, "UNSUPPORTED ") {
		t.Fatalf("extension spec must cover the unsupported change: status %d\n%s", status, out)
	}
}

func TestMutationInvalidAndBaselineFailuresDoNotPass(t *testing.T) {
	_, run, write := mutationFixture(t)
	write("internal/parse/guard.go", "package parse\nfunc Accept(n int) bool { return true }\n")
	write("mutants.json", `[{"file":"internal/parse/guard.go","line":2,"old":"true","new":"unknownIdentifier"}]`)
	status, out := run("--spec", "mutants.json")
	if status != 3 || !strings.Contains(out, "INVALID ") {
		t.Fatalf("non-building mutant must be inconclusive: status %d\n%s", status, out)
	}
	write("internal/parse/guard.go", "package parse\nfunc Accept(n int) bool { return false }\n")
	status, out = run()
	if status != 2 || !strings.Contains(out, "baseline did not pass") {
		t.Fatalf("failing baseline must refuse mutation: status %d\n%s", status, out)
	}
}

func TestMutationTimeoutAfterKilledIsInconclusive(t *testing.T) {
	t.Setenv("MUTATION_TIMEOUT", "3")
	_, run, write := mutationFixture(t)
	write("internal/parse/guard.go", "package parse\nfunc Accept(n int) bool { return true }\n")
	write("mutants.json", `[{"file":"internal/parse/guard.go","line":2,"old":"true","new":"func() bool { for {} }()"}]`)
	status, out := run("--spec", "mutants.json")
	if status != 3 || !strings.Contains(out, "1 killed, 0 lived, 1 timed out, 0 invalid") || !strings.Contains(out, "inconclusive") {
		t.Fatalf("one timeout must prevent a clean result even after a kill: status %d\n%s", status, out)
	}
	write("internal/parse/guard_test.go", "package parse\nimport \"testing\"\nfunc TestAccept(t *testing.T) { Accept(1) }\n")
	status, out = run("--spec", "mutants.json")
	if status != 1 || !strings.Contains(out, "0 killed, 1 lived, 1 timed out, 0 invalid") {
		t.Fatalf("a survivor takes precedence without hiding the timeout: status %d\n%s", status, out)
	}
}

func TestMutationVetAndCompileContract(t *testing.T) {
	for _, file := range []string{"internal/parse/guard.go", "third_party/mvdan-sh/syntax/guard.go"} {
		t.Run(file, func(t *testing.T) {
			_, run, write := mutationFixture(t)
			pkg := "parse"
			if strings.HasPrefix(file, "third_party/") {
				pkg = "syntax"
			}
			write(file, "package "+pkg+"\nimport \"fmt\"\nfunc Accept(n int) bool { fmt.Printf(\"%s\", \"value\"); return true }\n")
			write("mutants.json", fmt.Sprintf(`[{"file":%q,"line":3,"old":"\"%%s\"","new":"\"%%d\""}]`, file))
			status, out := run("--spec", "mutants.json")
			if status != 1 || !strings.Contains(out, "LIVED "+file+":3:") || !strings.Contains(out, "1 killed, 1 lived, 0 timed out, 0 invalid") {
				t.Fatalf("vet diagnostics must not decide a compiling mutant: status %d\n%s", status, out)
			}
			write("mutants.json", fmt.Sprintf(`[{"file":%q,"line":3,"old":"true","new":"unknownIdentifier"}]`, file))
			status, out = run("--spec", "mutants.json")
			if status != 3 || !strings.Contains(out, "INVALID "+file+":3:") || !strings.Contains(out, "1 killed, 0 lived, 0 timed out, 1 invalid") {
				t.Fatalf("compile failures must remain invalid with vet disabled: status %d\n%s", status, out)
			}
			write(file, "package "+pkg+"\nimport \"fmt\"\nfunc Accept(n int) bool { fmt.Printf(\"%d\", \"value\"); return true }\n")
			status, out = run()
			if status != 2 || !strings.Contains(out, "baseline did not pass") || strings.Contains(out, "KILLED ") {
				t.Fatalf("vet failure on the unmutated baseline must refuse mutation: status %d\n%s", status, out)
			}
		})
	}
}

func TestMutationRunnerBuildFailureIsSetupFailure(t *testing.T) {
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "go"), []byte("#!/bin/sh\nexit 1\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	_, run, _ := mutationFixture(t)
	status, out := run()
	if status != 2 {
		t.Fatalf("runner build failure must be setup failure: status %d\n%s", status, out)
	}
}

func TestMutationCandidatePreservesExecutableFiles(t *testing.T) {
	dir, run, write := mutationFixture(t)
	write("internal/parse/guard.go", "package parse\nfunc Accept(n int) bool { return n == 1 }\n")
	write("internal/parse/guard_test.go", "package parse\nimport (\"testing\"; \"os/exec\")\nfunc TestAccept(t *testing.T) { if err := exec.Command(\"./oracle.sh\").Run(); err != nil { t.Fatal(err) }; if !Accept(1) { t.Fatal(\"rejected one\") } }\n")
	write("internal/parse/oracle.sh", "#!/bin/sh\nexit 0\n")
	if err := os.Chmod(filepath.Join(dir, "internal/parse/oracle.sh"), 0o700); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"add", "."}, {"commit", "-q", "-m", "candidate"}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1", "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.invalid", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.invalid")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %s: %v\n%s", args, err, out)
		}
	}
	status, out := run("--candidate", "HEAD", "--workers", "3")
	if status != 0 || !strings.Contains(out, "KILLED ") {
		t.Fatalf("candidate replay must preserve its executable oracle: status %d\n%s", status, out)
	}
}

func TestMutationWorkersPreserveVerdictsAndSource(t *testing.T) {
	dir, run, write := mutationFixture(t)
	source := "package syntax\nfunc Accept(n int) bool { return n <= 2 }\n"
	write("third_party/mvdan-sh/syntax/guard.go", source)
	// Includes a fork kill, a root-only kill, and survivors. If workers
	// share a source file, the always-false and always-true mutants race.
	write("mutants.json", `[{"file":"third_party/mvdan-sh/syntax/guard.go","line":2,"old":"n <= 2","new":"false"},{"file":"third_party/mvdan-sh/syntax/guard.go","line":2,"old":"n <= 2","new":"true"}]`)
	write("internal/parse/guard_test.go", "package parse\nimport \"testing\"\nfunc TestAccept(t *testing.T) { if !Accept(1) || Accept(3) { t.Fatal(\"wrong boundary\") } }\n")
	verdicts := func(out string) string {
		var rows []string
		for _, line := range strings.Split(out, "\n") {
			for _, prefix := range []string{"KILLED ", "LIVED ", "TIMED OUT ", "INVALID "} {
				if strings.HasPrefix(line, prefix) {
					rows = append(rows, line)
				}
			}
		}
		return strings.Join(rows, "\n")
	}
	status, out := run("--spec", "mutants.json", "--workers", "1")
	if status != 1 || !strings.Contains(out, "KILLED ") || !strings.Contains(out, "LIVED ") {
		t.Fatalf("sequential control must contain both kills and survivors: status %d\n%s", status, out)
	}
	control := verdicts(out)
	for _, workers := range []string{"2", "3"} {
		status, out = run("--spec", "mutants.json", "--workers", workers)
		if status != 1 || verdicts(out) != control {
			t.Fatalf("workers=%s changed ordered verdicts: status %d\n%s\ncontrol:\n%s", workers, status, out, control)
		}
		body, err := os.ReadFile(filepath.Join(dir, "third_party/mvdan-sh/syntax/guard.go"))
		if err != nil || string(body) != source {
			t.Fatalf("workers changed contributor source: %v", err)
		}
	}
	for _, workers := range []string{"0", "4"} {
		status, out = run("--workers", workers)
		if status != 2 {
			t.Fatalf("unsupported worker count %s must refuse: status %d\n%s", workers, status, out)
		}
	}
}

func TestMutationSamplingIsSeededAndIncomplete(t *testing.T) {
	_, run, write := mutationFixture(t)
	write("internal/parse/guard.go", "package parse\nfunc Accept(n int) bool { return n <= 2 }\n")
	rows := func(out string) string {
		var result []string
		for _, line := range strings.Split(out, "\n") {
			if strings.HasPrefix(line, "KILLED ") || strings.HasPrefix(line, "LIVED ") {
				result = append(result, line)
			}
		}
		return strings.Join(result, "\n")
	}
	status, out := run("--sample", "1", "--seed", "17")
	if (status != 1 && status != 3) || !strings.Contains(out, "sampling: testing 1 of 4 generated mutants; seed 17") || strings.Count(rows(out), "\n") != 0 || rows(out) == "" {
		t.Fatalf("sample must run one real mutant and retain incomplete scope: status %d\n%s", status, out)
	}
	first := rows(out)
	status, out = run("--sample", "1", "--seed", "17")
	if (status != 1 && status != 3) || rows(out) != first {
		t.Fatalf("same seed and snapshot must reproduce sample: status %d\n%s", status, out)
	}
	status, out = run("--sample", "1", "--seed", "99")
	if (status != 1 && status != 3) || rows(out) == first {
		t.Fatalf("a different pinned seed must select a different mutant: status %d\n%s", status, out)
	}
	write("mutants.json", `[{"file":"internal/parse/guard.go","line":2,"old":"n <= 2","new":"false"}]`)
	status, out = run("--sample", "1", "--seed", "17", "--spec", "mutants.json")
	if (status != 1 && status != 3) || !strings.Contains(out, `"n <= 2" -> "false"`) || strings.Count(rows(out), "\n") != 1 {
		t.Fatalf("sampling must not drop explicit supplemental mutants: status %d\n%s", status, out)
	}
	for _, options := range [][]string{{"--sample", "-1"}, {"--sample", "1", "--max-mutants", "1"}, {"--sample", "1", "--spec-only", "--spec", "mutants.json"}} {
		status, out = run(options...)
		if status != 2 {
			t.Fatalf("invalid sample options must refuse: %v status %d\n%s", options, status, out)
		}
	}
	write("internal/parse/guard.go", "package parse\nfunc Accept(n int) bool { return n != 0 }\n")
	status, out = run("--sample", "1", "--seed", "17")
	if status != 3 || !strings.Contains(out, "1 killed, 0 lived, 0 timed out, 0 invalid") {
		t.Fatalf("all-killed partial sample must not return clean: status %d\n%s", status, out)
	}
}

func TestMutationReportsSurvivorsAndIncompleteEvidence(t *testing.T) {
	_, run, write := mutationFixture(t)
	write("internal/parse/guard.go", "package parse\nfunc Accept(n int) bool { return n <= 2 }\n")
	status, out := run()
	if status != 1 || !strings.Contains(out, "LIVED internal/parse/guard.go:") {
		t.Fatalf("survivor must fail mutation: status %d\n%s", status, out)
	}
	// The first ordered mutant negates <= to < and lives for n=1. A cap
	// must remain visible even when a survivor already makes the run fail.
	status, out = run("--max-mutants", "1")
	if status != 1 || !strings.Contains(out, "LIVED ") {
		t.Fatalf("capped survivor must still fail: status %d\n%s", status, out)
	}
	write("internal/parse/guard.go", "package parse\nfunc Accept(n int) bool { return n != 0 }\n")
	status, out = run("--max-mutants", "1")
	if status != 3 || !strings.Contains(out, "inconclusive") {
		t.Fatalf("capped killed run must not pass: status %d\n%s", status, out)
	}
	write("internal/parse/guard.go", "package parse\nfunc Accept(n int) bool { return true }\n")
	status, out = run()
	if status != 0 || !strings.Contains(out, "KILLED ") {
		t.Fatalf("all killed must pass: status %d\n%s", status, out)
	}
}

func TestMutationWorkersContainTimeouts(t *testing.T) {
	goBinary, err := exec.LookPath("go")
	if err != nil {
		t.Fatal(err)
	}
	bin := t.TempDir()
	pids := filepath.Join(bin, "pids")
	quotedGo := "'" + strings.ReplaceAll(goBinary, "'", "'\"'\"'") + "'"
	body := fmt.Sprintf("#!/bin/sh\nif [ \"$1\" = build ]; then exec %s \"$@\"; fi\ncase \" $* \" in *' -vet=all '*) exit 0 ;; esac\nsleep 300 &\necho $! >> \"$MUTATION_PIDS\"\nwait\n", quotedGo)
	if err := os.WriteFile(filepath.Join(bin, "go"), []byte(body), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("MUTATION_PIDS", pids)
	t.Setenv("MUTATION_TIMEOUT", "0.2")
	_, run, write := mutationFixture(t)
	write("internal/parse/guard.go", "package parse\nfunc Accept(n int) bool { return n != 0 }\n")
	status, out := run("--workers", "3")
	if status != 3 || !strings.Contains(out, "0 killed, 0 lived, 3 timed out, 0 invalid") {
		t.Fatalf("parallel timeouts must all remain unresolved: status %d\n%s", status, out)
	}
	pidText, err := os.ReadFile(pids)
	if err != nil {
		t.Fatal(err)
	}
	if len(strings.Fields(string(pidText))) != 3 {
		t.Fatalf("every worker must have started a descendant: %s", pidText)
	}
	for _, text := range strings.Fields(string(pidText)) {
		pid, err := strconv.Atoi(text)
		if err != nil {
			t.Fatal(err)
		}
		if syscall.Kill(pid, 0) == nil {
			state, _ := exec.Command("ps", "-o", "stat=", "-p", text).Output()
			if !strings.HasPrefix(strings.TrimSpace(string(state)), "Z") {
				_ = syscall.Kill(pid, syscall.SIGKILL)
				t.Fatalf("parallel descendant still running: %d %s", pid, state)
			}
		}
	}
}

func TestMutationWorkersCancelTogether(t *testing.T) {
	goBinary, err := exec.LookPath("go")
	if err != nil {
		t.Fatal(err)
	}
	bin := t.TempDir()
	pids := filepath.Join(bin, "pids")
	quotedGo := "'" + strings.ReplaceAll(goBinary, "'", "'\"'\"'") + "'"
	body := fmt.Sprintf("#!/bin/sh\nif [ \"$1\" = build ]; then exec %s \"$@\"; fi\ncase \" $* \" in *' -vet=all '*) exit 0 ;; esac\nsleep 300 &\nprintf '%%s %%s\\n' \"$PPID\" \"$!\" >> \"$MUTATION_PIDS\"\nwait\n", quotedGo)
	if err := os.WriteFile(filepath.Join(bin, "go"), []byte(body), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("MUTATION_PIDS", pids)
	dir, _, write := mutationFixture(t)
	write("internal/parse/guard.go", "package parse\nfunc Accept(n int) bool { return n != 0 }\n")
	script := repositoryFilePath(t, ".github", "scripts", "mutation.sh")
	cmd := exec.Command("bash", script, "--workers", "3", "HEAD")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOWORK=off", "MUTATION_TIMEOUT=60")
	var output bytes.Buffer
	cmd.Stdout, cmd.Stderr = &output, &output
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	finished := make(chan error, 1)
	go func() { finished <- cmd.Wait() }()
	deadline := time.NewTimer(20 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	var children []int
	runner := 0
	for len(children) < 3 {
		select {
		case err := <-finished:
			t.Fatalf("runner exited before all workers started: %v\n%s", err, output.String())
		case <-deadline.C:
			_ = cmd.Process.Kill()
			t.Fatal("workers did not start before cancellation deadline")
		case <-ticker.C:
			text, _ := os.ReadFile(pids)
			children = nil
			for _, line := range strings.Split(strings.TrimSpace(string(text)), "\n") {
				fields := strings.Fields(line)
				if len(fields) != 2 {
					continue
				}
				runner, _ = strconv.Atoi(fields[0])
				child, _ := strconv.Atoi(fields[1])
				if runner > 0 && child > 0 {
					children = append(children, child)
				}
			}
		}
	}
	if err := syscall.Kill(runner, syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-finished:
		exit, ok := err.(*exec.ExitError)
		if !ok || exit.ExitCode() != 2 || !strings.Contains(output.String(), "context canceled") {
			t.Fatalf("cancellation must be setup/incomplete failure: %v\n%s", err, output.String())
		}
	case <-deadline.C:
		_ = cmd.Process.Kill()
		t.Fatal("runner did not join its canceled workers")
	}
	for _, pid := range children {
		if syscall.Kill(pid, 0) == nil {
			state, _ := exec.Command("ps", "-o", "stat=", "-p", strconv.Itoa(pid)).Output()
			if !strings.HasPrefix(strings.TrimSpace(string(state)), "Z") {
				_ = syscall.Kill(pid, syscall.SIGKILL)
				t.Fatalf("canceled worker left descendant %d: %s", pid, state)
			}
		}
	}
}

func TestMutationCleansDescendantsOnTimeoutAndNormalExit(t *testing.T) {
	goBinary, err := exec.LookPath("go")
	if err != nil {
		t.Skip("go is required")
	}
	for _, finish := range []string{"wait", "exit 0"} {
		t.Run(finish, func(t *testing.T) {
			fakeBin := t.TempDir()
			marker, pids := filepath.Join(fakeBin, "baseline"), filepath.Join(fakeBin, "pids")
			// Go is a process boundary here. Keep the real helper build, then
			// make the test driver leave a descendant holding its output open.
			quotedGo := "'" + strings.ReplaceAll(goBinary, "'", "'\"'\"'") + "'"
			body := fmt.Sprintf("#!/bin/sh\nif [ \"$1\" = build ]; then exec %s \"$@\"; fi\nif [ ! -f \"$MUTATION_MARKER\" ]; then : > \"$MUTATION_MARKER\"; exit 0; fi\nsleep 300 &\necho $! >> \"$MUTATION_PIDS\"\n%s\n", quotedGo, finish)
			if err := os.WriteFile(filepath.Join(fakeBin, "go"), []byte(body), 0o700); err != nil {
				t.Fatal(err)
			}
			t.Setenv("PATH", fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"))
			t.Setenv("MUTATION_MARKER", marker)
			t.Setenv("MUTATION_PIDS", pids)
			t.Setenv("MUTATION_TIMEOUT", "0.2")
			_, run, write := mutationFixture(t)
			write("internal/parse/guard.go", "package parse\nfunc Accept(n int) bool { return true }\n")
			status, out := run()
			want := 3
			verdict := "TIMED OUT"
			if finish == "exit 0" {
				want, verdict = 1, "LIVED"
			}
			if status != want || !strings.Contains(out, verdict) {
				t.Fatalf("descendant run: want %s, got status %d\n%s", verdict, status, out)
			}
			pidText, err := os.ReadFile(pids)
			if err != nil {
				t.Fatal(err)
			}
			for _, text := range strings.Fields(string(pidText)) {
				pid, err := strconv.Atoi(text)
				if err != nil {
					t.Fatal(err)
				}
				if syscall.Kill(pid, 0) == nil {
					state, _ := exec.Command("ps", "-o", "stat=", "-p", text).Output()
					if !strings.HasPrefix(strings.TrimSpace(string(state)), "Z") {
						_ = syscall.Kill(pid, syscall.SIGKILL)
						t.Fatalf("descendant %d still running: %s", pid, state)
					}
				}
			}
		})
	}
}

func mutationFixture(t *testing.T) (string, func(...string) (int, string), func(string, string)) {
	t.Helper()
	for _, command := range []string{"bash", "git", "go"} {
		if _, err := exec.LookPath(command); err != nil {
			t.Skip(command + " is required for the mutation behavior test")
		}
	}
	dir := t.TempDir()
	timeout := os.Getenv("MUTATION_TIMEOUT")
	if timeout == "" {
		timeout = "60"
	}
	env := append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1",
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.invalid",
		"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.invalid",
		"GOWORK=off", "MUTATION_TIMEOUT="+timeout)
	write := func(name, body string) {
		t.Helper()
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	git := func(args ...string) string {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir, cmd.Env = dir, env
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
		}
		return strings.TrimSpace(string(out))
	}
	write("go.mod", "module example.invalid/root\ngo 1.26.0\nrequire example.invalid/fork v0.0.0\nreplace example.invalid/fork => ./third_party/mvdan-sh\n")
	write("third_party/mvdan-sh/go.mod", "module example.invalid/fork\ngo 1.26.0\n")
	write("third_party/mvdan-sh/syntax/guard.go", "package syntax\nfunc Accept(n int) bool { return n < 2 }\n")
	write("third_party/mvdan-sh/syntax/guard_test.go", "package syntax\nimport \"testing\"\nfunc TestAccept(t *testing.T) { if !Accept(1) { t.Fatal(\"rejected one\") } }\n")
	write("internal/parse/guard.go", "package parse\nimport \"example.invalid/fork/syntax\"\nfunc Accept(n int) bool { return syntax.Accept(n) }\n")
	write("internal/parse/guard_test.go", "package parse\nimport \"testing\"\nfunc TestAccept(t *testing.T) { if !Accept(1) { t.Fatal(\"rejected one\") } }\n")
	write("internal/survey/guard_test.go", "package survey\nimport (\"testing\"; \"example.invalid/fork/syntax\")\nfunc TestAccept(t *testing.T) { if !syntax.Accept(1) { t.Fatal(\"rejected one\") } }\n")
	git("init", "-q", "-b", "main")
	git("add", ".")
	git("commit", "-q", "-m", "fixture")
	base := git("rev-parse", "HEAD")
	script := repositoryFilePath(t, ".github", "scripts", "mutation.sh")
	run := func(args ...string) (int, string) {
		t.Helper()
		arguments := append([]string{script}, args...)
		cmd := exec.Command("bash", append(arguments, base)...)
		cmd.Dir, cmd.Env = dir, env
		out, err := cmd.CombinedOutput()
		if exit, ok := err.(*exec.ExitError); ok {
			return exit.ExitCode(), string(out)
		}
		if err != nil {
			t.Fatal(err)
		}
		return 0, string(out)
	}
	return dir, run, write
}
