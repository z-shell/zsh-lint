package workflowcontract

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
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
