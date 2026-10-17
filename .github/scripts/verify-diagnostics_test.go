// Behavioral tests for diagnostic evidence, using explicit synthetic fixtures.
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func runFixtureCLI(behavior string) {
	if len(os.Args) < 2 {
		os.Exit(2)
	}
	file := os.Args[len(os.Args)-1]
	if behavior == "stderr" {
		fmt.Fprintln(os.Stderr, "fixture error")
	}
	if behavior == "drift" {
		_ = os.WriteFile(filepath.Join(filepath.Dir(file), "zsh-lint.json"), []byte("changed"), 0644)
	}
	if behavior == "unwritable-report" {
		_ = os.Mkdir(filepath.Join(filepath.Dir(file), "evidence", "diagnostics.json"), 0755)
	}
	if behavior == "unwritable-summary" {
		_ = os.Mkdir(filepath.Join(filepath.Dir(file), "evidence", "summary.md"), 0755)
	}
	if behavior == "unwritable-lane" {
		_ = os.Mkdir(filepath.Join(filepath.Dir(file), "evidence", "1-base.json"), 0755)
	}
	findings := []Finding{}
	marker := strings.TrimSuffix(file, filepath.Ext(file)) + ".marker"
	if behavior == "other-rule" {
		if _, err := os.Stat(marker); err == nil {
			findings = append(findings, Finding{
				"file":     file,
				"rule":     "other/rule",
				"severity": "hint",
				"message":  "fixture",
			})
		}
	}
	_ = os.WriteFile(marker, []byte(""), 0644)
	summary := map[string]int{
		"files":       1,
		"diagnostics": len(findings),
		"errors":      0,
		"warnings":    0,
		"infos":       0,
		"hints":       len(findings),
	}
	if behavior == "malformed" {
		fmt.Println("not JSON")
	} else {
		out := map[string]any{
			"version":     1,
			"diagnostics": findings,
			"summary":     summary,
		}
		b, _ := json.Marshal(out)
		fmt.Println(string(b))
	}
	if behavior == "bad-exit" {
		os.Exit(1)
	} else {
		os.Exit(0)
	}
}

func TestMain(m *testing.M) {
	if behavior := os.Getenv("TEST_FIXTURE_CLI_BEHAVIOR"); behavior != "" {
		runFixtureCLI(behavior)
		return
	}
	os.Exit(m.Run())
}

func getRepoRoot(t *testing.T) string {
	t.Helper()
	root, err := repoRoot()
	if err != nil {
		t.Fatalf("failed to determine repository root: %v", err)
	}
	return root
}

func repoRoot() (string, error) {
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	root := strings.TrimSpace(string(out))
	return filepath.EvalSymlinks(root)
}

func TestShellSetupReusesToolsAndRejectsCorruptArchives(t *testing.T) {
	repo := getRepoRoot(t)
	setup := filepath.Join(repo, ".github", "scripts", "agent-setup.sh")
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash is required for setup test")
	}

	cases := []struct {
		name    string
		broken  string
		archive string
	}{
		{"clean", "", ""},
		{"dash-broken", "dash", "dash-0.5.12.tar.gz"},
		{"mksh-broken", "mksh", "mksh-R59c.tgz"},
		{"bash-broken", "bash", "bash-5.3.tar.gz"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			tools := filepath.Join(dir, "tools")
			if err := os.Mkdir(tools, 0755); err != nil {
				t.Fatal(err)
			}

			scripts := map[string]string{
				"bash": "echo 'GNU bash, version 5.3.0'",
				"dash": "echo 'Bad substitution' >&2; exit 2",
				"mksh": "echo 'fixture R59 2020/10/31'",
				"curl": "echo unexpected-download >&2; exit 99",
				"go":   "echo unexpected-go >&2; exit 99",
				"tar":  "echo unexpected-extraction >&2; exit 99",
			}
			if tc.broken != "" {
				scripts[tc.broken] = "echo incompatible; exit 1"
			}

			for name, body := range scripts {
				p := filepath.Join(tools, name)
				if err := os.WriteFile(p, []byte("#!/bin/sh\n"+body+"\n"), 0755); err != nil {
					t.Fatal(err)
				}
			}

			prefix := filepath.Join(dir, "prefix")
			if tc.archive != "" {
				if err := os.MkdirAll(filepath.Join(prefix, "downloads"), 0755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(prefix, "downloads", tc.archive), []byte("corrupt fixture archive"), 0644); err != nil {
					t.Fatal(err)
				}
			}

			cmd := exec.Command(bash, setup, "--shells-only")
			cmd.Dir = repo
			cmd.Env = append(os.Environ(),
				"PATH="+tools+string(os.PathListSeparator)+os.Getenv("PATH"),
				"ZSH_LINT_SHELL_PREFIX="+prefix,
			)

			out, err := cmd.CombinedOutput()
			outStr := string(out)

			if tc.broken != "" {
				if err == nil {
					t.Fatalf("expected error for broken %s, got success:\n%s", tc.broken, outStr)
				}
				if !strings.Contains(outStr, "checksum mismatch") {
					t.Fatalf("expected checksum mismatch in output:\n%s", outStr)
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected failure: %v\n%s", err, outStr)
				}
				if !strings.Contains(outStr, "fork shells ready") {
					t.Fatalf("expected 'fork shells ready' in output:\n%s", outStr)
				}
			}

			if strings.Contains(outStr, "unexpected-") {
				t.Fatalf("unexpected download/extraction tool called:\n%s", outStr)
			}
		})
	}
}

func TestManifestRegenerationPreservesMembershipAndOrder(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "one.zsh"), []byte("print one\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "two"), []byte("print two\n"), 0644); err != nil {
		t.Fatal(err)
	}
	listing := filepath.Join(dir, "files.txt")
	if err := os.WriteFile(listing, []byte("two\x00one.zsh\x00"), 0644); err != nil {
		t.Fatal(err)
	}

	manifestPath := filepath.Join(dir, "manifest.json")
	args := []string{
		"--list", listing,
		"--root", "r=" + dir,
		"--relative-to", "r",
		"--mode", "no-config",
		"--output", manifestPath,
	}

	if status := writeManifest(args); status != 0 {
		t.Fatalf("writeManifest failed with code %d", status)
	}

	data, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}

	var m struct {
		Files []string `json:"files"`
	}
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatal(err)
	}

	expected := []string{"r:two", "r:one.zsh"}
	if !reflect.DeepEqual(m.Files, expected) {
		t.Fatalf("expected files %v, got %v", expected, m.Files)
	}

	if status := writeManifest(args); status != 2 {
		t.Fatalf("expected overwrite refusal exit 2, got %d", status)
	}
}

func TestRelocationAndDuplicateFindingsAreNotNetCounts(t *testing.T) {
	old := Finding{
		"rule":     "plugin/target",
		"severity": "hint",
		"file":     "org:a.zsh",
		"message":  "scope",
		"range":    map[string]any{"start": map[string]any{"line": 1}},
	}
	moved := Finding{
		"rule":     "plugin/target",
		"severity": "hint",
		"file":     "org:a.zsh",
		"message":  "scope",
		"range":    map[string]any{"start": map[string]any{"line": 2}},
	}
	other := Finding{
		"rule":     "style/other",
		"severity": "hint",
		"file":     "org:a.zsh",
		"message":  "scope",
		"range":    map[string]any{"start": map[string]any{"line": 1}},
	}

	result := compare([]Finding{old, old, other}, []Finding{moved, other}, "plugin/target")
	if len(result.PerRule["plugin/target"].Removed) != 2 {
		t.Fatalf("expected 2 removed findings, got %d", len(result.PerRule["plugin/target"].Removed))
	}
	if len(result.PerRule["plugin/target"].Added) != 1 {
		t.Fatalf("expected 1 added finding, got %d", len(result.PerRule["plugin/target"].Added))
	}
	if result.NonTargetIdentical == nil || !*result.NonTargetIdentical {
		t.Fatalf("expected non_target_identical to be true")
	}

	result2 := compare([]Finding{old, other}, []Finding{old}, "plugin/target")
	if result2.NonTargetIdentical == nil || *result2.NonTargetIdentical {
		t.Fatalf("expected non_target_identical to be false")
	}
}

func TestManifestAndOutputFailClosed(t *testing.T) {
	dir := t.TempDir()
	manifest := filepath.Join(dir, "population.json")

	_ = os.WriteFile(manifest, []byte(`{"version": 1, "mode": "no-config", "files": ["r:missing"]}`), 0644)
	if _, _, _, err := loadPopulation(manifest, map[string]string{"r": dir}); err == nil {
		t.Fatal("expected error on missing file")
	}

	_ = os.WriteFile(manifest, []byte(`{"version": 1, "mode": "no-config", "files": ["r:../escape"]}`), 0644)
	if _, _, _, err := loadPopulation(manifest, map[string]string{"r": dir}); err == nil {
		t.Fatal("expected error on escaping path")
	}

	validJSON := `{
		"version": 1,
		"diagnostics": [],
		"summary": {
			"files": 1,
			"diagnostics": 0,
			"errors": 0,
			"warnings": 0,
			"infos": 0,
			"hints": 0
		}
	}`
	out, err := validateOutput([]byte(validJSON), map[string]bool{"f": true})
	if err != nil {
		t.Fatalf("expected valid output to succeed: %v", err)
	}
	if out.Summary.Files != 1 {
		t.Fatalf("expected 1 file in summary, got %d", out.Summary.Files)
	}
	normalized, err := json.Marshal(out)
	if err != nil || !strings.Contains(string(normalized), `"diagnostics":[]`) {
		t.Fatalf("empty diagnostic output must retain its JSON array: %s (%v)", normalized, err)
	}
	for _, trailing := range []string{"garbage", validJSON} {
		if _, err := validateOutput([]byte(validJSON+trailing), map[string]bool{"f": true}); err == nil {
			t.Errorf("expected rejection of trailing diagnostic output: %q", trailing)
		}
	}

	invalidSummaryJSON := `{
		"version": 1,
		"diagnostics": [],
		"summary": {
			"files": 0,
			"diagnostics": 0,
			"errors": 0,
			"warnings": 0,
			"infos": 0,
			"hints": 0
		}
	}`
	if _, err := validateOutput([]byte(invalidSummaryJSON), map[string]bool{"f": true}); err == nil {
		t.Fatal("expected error on mismatched summary")
	}
}

func TestCommandFailureAndTimeoutRetainEvidence(t *testing.T) {
	dir := t.TempDir()
	commands := newCommands(dir, 5*time.Second)

	// Failure command: generous 5s deadline, fails with exit code 2.
	_, _, err := commands.run([]string{"/bin/sh", "-c", "exit 2"}, dir, []int{0})
	if err == nil {
		t.Fatal("expected error on nonzero returncode")
	}

	// Timeout command: short 50ms deadline on intentional long sleep.
	commands.timeout = 50 * time.Millisecond
	_, _, err = commands.run([]string{"/bin/sh", "-c", "sleep 10"}, dir, []int{0})
	if err == nil {
		t.Fatal("expected error on timeout")
	}

	cmdLog, err := os.ReadFile(filepath.Join(dir, "commands.json"))
	if err != nil {
		t.Fatal(err)
	}

	var rows []CommandRow
	if err := json.Unmarshal(cmdLog, &rows); err != nil {
		t.Fatal(err)
	}

	if len(rows) != 2 {
		t.Fatalf("expected 2 command rows, got %d", len(rows))
	}
	if rows[0].Returncode != 2 {
		t.Fatalf("expected row 0 returncode 2, got %d", rows[0].Returncode)
	}
	if !rows[1].TimedOut {
		t.Fatal("expected row 1 timed_out to be true")
	}
}

func TestCommandEvidenceWritesFailClosed(t *testing.T) {
	for _, name := range []string{"command-1.stdout", "command-1.stderr", "commands.json"} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.Mkdir(filepath.Join(dir, name), 0755); err != nil {
				t.Fatal(err)
			}
			commands := newCommands(dir, 5*time.Second)
			if _, _, err := commands.run([]string{"/bin/sh", "-c", "exit 0"}, dir, []int{0}); err == nil {
				t.Fatal("expected error when command evidence cannot be written")
			}
		})
	}
}

func TestCommandLaunchFailureRetainsEvidence(t *testing.T) {
	dir := t.TempDir()
	commands := newCommands(dir, 5*time.Second)
	if _, _, err := commands.run([]string{filepath.Join(dir, "missing-executable")}, dir, []int{0, 1}); err == nil {
		t.Fatal("expected launch failure")
	}
	if len(commands.rows) != 1 {
		t.Fatal("expected launch failure to be recorded")
	}
	for _, name := range []string{"command-1.stdout", "command-1.stderr", "commands.json"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Errorf("missing failure evidence %s: %v", name, err)
		}
	}
}

func TestRealEntryPointRejectsMissingPopulationInput(t *testing.T) {
	repo := getRepoRoot(t)
	dir := t.TempDir()
	manifest := filepath.Join(dir, "population.json")
	if err := os.WriteFile(manifest, []byte(`{"version": 1, "mode": "no-config", "files": ["r:missing"]}`), 0644); err != nil {
		t.Fatal(err)
	}

	script := filepath.Join(repo, ".github", "scripts", "verify-parser-change.sh")
	evidenceDir := filepath.Join(dir, "evidence")
	cmd := exec.Command("bash", script,
		"--diagnostics", "HEAD",
		"--root", "r="+dir,
		"--population", "test="+manifest,
		"--output", evidenceDir,
	)
	cmd.Dir = repo
	cmd.Env = os.Environ()

	out, err := cmd.CombinedOutput()
	var exitCode int
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			exitCode = exitErr.ExitCode()
		}
	}

	if exitCode != 2 {
		t.Fatalf("expected exit code 2, got %d\noutput:\n%s", exitCode, out)
	}

	reportBytes, err := os.ReadFile(filepath.Join(evidenceDir, "diagnostics.json"))
	if err != nil {
		t.Fatalf("failed to read diagnostics.json: %v", err)
	}

	var report map[string]any
	if err := json.Unmarshal(reportBytes, &report); err != nil {
		t.Fatal(err)
	}

	if report["status"] != "error" {
		t.Fatalf("expected status 'error', got %v", report["status"])
	}
	pops, ok := report["populations"].(map[string]any)
	if !ok || len(pops) != 0 {
		t.Fatalf("expected empty populations map, got %v", report["populations"])
	}
}

func TestRealEntryPointRejectsCheckoutOutputAlias(t *testing.T) {
	repo := getRepoRoot(t)
	dir := t.TempDir()
	alias := filepath.Join(dir, "checkout-alias")
	if err := os.Symlink(repo, alias); err != nil {
		t.Fatal(err)
	}
	name := "diagnostic-output-" + filepath.Base(filepath.Dir(dir))
	target := filepath.Join(repo, name)
	if _, err := os.Lstat(target); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("test output must not exist: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(target) })
	cmd := exec.Command("bash", filepath.Join(repo, ".github", "scripts", "verify-parser-change.sh"),
		"--diagnostics", "HEAD", "--output", filepath.Join(alias, name))
	cmd.Dir = repo
	output, err := cmd.CombinedOutput()
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != 2 || !strings.Contains(string(output), "output must be outside the checkout") {
		t.Fatalf("expected checkout-alias rejection, got %v: %s", err, output)
	}
}

func TestComparisonRunsBothLanesAndRejectsStderrAndDrift(t *testing.T) {
	repo := getRepoRoot(t)
	testExe, err := os.Executable()
	if err != nil {
		t.Fatalf("failed to get test executable: %v", err)
	}

	behaviors := []string{
		"pass",
		"stderr",
		"malformed",
		"drift",
		"other-rule",
		"bad-exit",
		"unwritable-report",
		"unwritable-summary",
		"unwritable-lane",
	}

	for _, behavior := range behaviors {
		t.Run(behavior, func(t *testing.T) {
			dir := t.TempDir()
			source := filepath.Join(dir, "a.zsh")
			if err := os.WriteFile(source, []byte("print ok\n"), 0644); err != nil {
				t.Fatal(err)
			}
			config := filepath.Join(dir, "zsh-lint.json")
			if err := os.WriteFile(config, []byte("{}"), 0644); err != nil {
				t.Fatal(err)
			}
			manifest := filepath.Join(dir, "population.json")
			manifestData := fmt.Sprintf(`{"version": 1, "mode": "configured", "files": ["r:%s"]}`, filepath.Base(source))
			if err := os.WriteFile(manifest, []byte(manifestData), 0644); err != nil {
				t.Fatal(err)
			}

			fixtureScript := filepath.Join(dir, "fixture-cli")
			scriptBody := fmt.Sprintf("#!/bin/sh\nexport TEST_FIXTURE_CLI_BEHAVIOR=%q\nexec %q \"$@\"\n", behavior, testExe)
			if err := os.WriteFile(fixtureScript, []byte(scriptBody), 0755); err != nil {
				t.Fatal(err)
			}

			origBuild := buildCLI
			buildCLI = func(commands *Commands, root, revision, destination string, working bool) (string, error) {
				return fixtureScript, nil
			}
			defer func() { buildCLI = origBuild }()

			evidenceDir := filepath.Join(dir, "evidence")
			args := []string{
				"HEAD",
				"--root", "r=" + dir,
				"--population", "test=" + manifest,
				"--target-rule", "plugin/target",
				"--output", evidenceDir,
			}

			origDir, _ := os.Getwd()
			_ = os.Chdir(repo)
			defer func() { _ = os.Chdir(origDir) }()

			status := runMain(args)

			expected := 2
			if behavior == "pass" {
				expected = 0
			} else if behavior == "other-rule" {
				expected = 1
			}

			if status != expected {
				t.Fatalf("behavior %s: expected exit %d, got %d", behavior, expected, status)
			}
			if behavior == "unwritable-report" {
				return // The obstruction deliberately prevents a report file.
			}

			reportBytes, err := os.ReadFile(filepath.Join(evidenceDir, "diagnostics.json"))
			if err != nil {
				t.Fatalf("failed to read diagnostics.json: %v", err)
			}

			var report map[string]any
			if err := json.Unmarshal(reportBytes, &report); err != nil {
				t.Fatal(err)
			}
			if expected == 2 && report["status"] != "error" {
				t.Fatalf("expected error evidence, got %v", report["status"])
			}

			if behavior == "pass" {
				pops := report["populations"].(map[string]any)
				testPop := pops["test"].(map[string]any)
				if testPop["input_hashes_identical"] != true {
					t.Fatalf("expected input_hashes_identical to be true")
				}
				base := testPop["base"].(map[string]any)
				if base["parse_errors"].(float64) != 0 {
					t.Fatalf("expected parse_errors 0, got %v", base["parse_errors"])
				}
			}
		})
	}
}
