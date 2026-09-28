package workflowcontract

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// parser-check.sh must refuse to measure against a stale or foreign base, the
// failure #545 was filed for: a measurement once ran 37 commits behind
// origin/main. Runs the real script's --check-base step in throwaway
// repositories; nothing is built.
func TestParserCheckRefusesAStaleBase(t *testing.T) {
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash is required for the script behavior test")
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is required for the script behavior test")
	}
	script := repositoryFilePath(t, ".github", "scripts", "parser-check.sh")

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
			t.Fatalf("run parser-check.sh: %v", err)
		}
		return status, string(out)
	}
	check := func(args ...string) (int, string) { t.Helper(); return checkIn(work, args...) }

	status, out := check()
	if status != 0 || !strings.Contains(out, "base:      "+first+" (origin/main, current with origin)") ||
		!strings.Contains(out, "candidate: "+feature+" (HEAD)") {
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

	// An explicit base skips the remote check but not containment.
	status, out = check("--base", first)
	if status != 0 || !strings.Contains(out, "base:      "+first+" (named with --base "+first+")") {
		t.Fatalf("explicit base: status %d, output:\n%s", status, out)
	}
	status, out = check("--base", moved)
	if status != 2 || !strings.Contains(out, "does not contain the base") {
		t.Fatalf("explicit base HEAD lacks: status %d, output:\n%s", status, out)
	}

	// A named candidate is checked the same way, and uncommitted work is
	// reported as part of the candidate.
	status, out = check("--base", first, "--candidate", moved)
	if status != 0 || !strings.Contains(out, "candidate: "+moved+" (named with --candidate "+moved+")") {
		t.Fatalf("named candidate: status %d, output:\n%s", status, out)
	}
	if err := os.WriteFile(filepath.Join(work, "tracked"), []byte("a\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	git(work, "add", "tracked")
	status, out = check("--base", first)
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
	status, out = checkIn(sub, "--base", first, "--rows", "rows.txt")
	if status != 0 {
		t.Fatalf("relative --rows from a subdirectory: status %d, output:\n%s", status, out)
	}
	status, out = checkIn(work, "--base", first, "--rows", "rows.txt")
	if status != 2 || !strings.Contains(out, "no such file or directory: rows.txt") {
		t.Fatalf("missing --rows: status %d, output:\n%s", status, out)
	}
}

// The script is documented as the parser-gap skill's verification step, so
// its usage must name every option the skill and the workflow page use.
func TestParserCheckDocumentsItsOptions(t *testing.T) {
	script := readRepositoryFile(t, ".github", "scripts", "parser-check.sh")
	for _, option := range []string{"--out", "--base", "--candidate", "--rows", "--bodies", "--root", "--list", "--regression-corpus", "--skip-go-checks", "--check-base"} {
		if !strings.Contains(script, "    "+option+")") {
			t.Errorf("parser-check.sh does not parse %s", option)
		}
	}
	skill := readRepositoryFile(t, ".github", "skills", "parser-gap-fix", "SKILL.md")
	if !strings.Contains(skill, "bash .github/scripts/parser-check.sh") {
		t.Error("the parser-gap-fix skill must run .github/scripts/parser-check.sh")
	}
}
